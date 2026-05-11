package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/frederic/tgtldr/app/internal/bot"
	"github.com/frederic/tgtldr/app/internal/clock"
	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/store"
	"github.com/frederic/tgtldr/app/internal/summary"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"
)

type knowledgeExtractor interface {
	RunDailyExtractionsForSummary(ctx context.Context, chat model.Chat, date string) ([]model.KnowledgeRun, error)
	EnsureDailyExtractionsForSummary(ctx context.Context, chat model.Chat, date string) ([]model.KnowledgeRun, error)
}

type aggregator interface {
	RunAggregatedSummary(ctx context.Context, channel model.DeliveryChannel, date string) (summary.AggregatedSummaryResult, error)
}

type Service struct {
	store              *store.Store
	clock              clock.Clock
	summaries          *summary.Service
	botService         *bot.Service
	knowledgeExtractor knowledgeExtractor
	aggregator         aggregator
	mu                 sync.Mutex
	inflight           map[string]struct{}
}

type scheduledAction int

const (
	scheduledActionSkip scheduledAction = iota
	scheduledActionGenerate
	scheduledActionDeliver
)

const channelExtractionParallelism = 2

type extractionRunReport struct {
	ChatTitle string
	SpaceName string
	Run       model.KnowledgeRun
}

func NewService(st *store.Store, c clock.Clock, summaries *summary.Service, botService *bot.Service, extractor knowledgeExtractor, agg aggregator) *Service {
	return &Service{
		store:              st,
		clock:              c,
		summaries:          summaries,
		botService:         botService,
		knowledgeExtractor: extractor,
		aggregator:         agg,
		inflight:           make(map[string]struct{}),
	}
}

func (s *Service) ContextPreview(ctx context.Context, item model.Summary) (model.SummaryContextPreview, error) {
	return s.summaries.BuildContextPreview(ctx, item)
}

func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	if err := s.runOnce(ctx); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.runOnce(ctx); err != nil {
				continue
			}
		}
	}
}

func (s *Service) RunNow(ctx context.Context, chat model.Chat, date string) error {
	key := summaryTaskKey(chat.ID, date)
	if !s.beginTask(key) {
		return nil
	}
	defer s.finishTask(key)
	return s.runNow(ctx, chat, date)
}

func (s *Service) RunNowAsync(ctx context.Context, chat model.Chat, date string) (bool, error) {
	key := summaryTaskKey(chat.ID, date)
	if !s.beginTask(key) {
		return false, nil
	}

	if err := s.store.Summaries.UpsertPending(ctx, chat.ID, date); err != nil {
		s.finishTask(key)
		return false, err
	}
	if err := s.store.Summaries.SetRunning(ctx, chat.ID, date); err != nil {
		s.finishTask(key)
		return false, err
	}

	go func() {
		defer s.finishTask(key)
		runCtx := context.Background()
		if err := s.executeSummary(runCtx, chat, date); err != nil {
			_ = s.store.Summaries.SetFailed(context.Background(), chat.ID, date, err.Error())
		}
	}()
	return true, nil
}

func (s *Service) RunChannelNowAsync(ctx context.Context, channel model.DeliveryChannel, date string) (bool, error) {
	key := channelTaskKey(channel.ID, date)
	if !s.beginTask(key) {
		return false, nil
	}

	started, err := s.store.DeliveryChannelRuns.TrySetRunning(ctx, channel.ID, date)
	if err != nil {
		s.finishTask(key)
		return false, err
	}
	if !started {
		s.finishTask(key)
		return false, nil
	}

	go func() {
		defer s.finishTask(key)
		runCtx := context.Background()
		if err := s.executeChannelSummary(runCtx, channel, date); err != nil {
			_ = s.store.DeliveryChannelRuns.MarkFailed(context.Background(), channel.ID, date, err.Error())
		}
	}()
	return true, nil
}

func (s *Service) RetryDelivery(ctx context.Context, summaryID int64) error {
	item, err := s.store.Summaries.GetByID(ctx, summaryID)
	if err != nil {
		return err
	}
	if item.Status != model.SummaryStatusSucceeded {
		return fmt.Errorf("只有生成成功的摘要才能重试发送")
	}

	chat, err := s.store.Chats.GetByID(ctx, item.ChatID)
	if err != nil {
		return err
	}
	if chat.DeliveryMode != model.DeliveryModeBot {
		return fmt.Errorf("当前群组设置为不发送")
	}

	key := summaryTaskKey(chat.ID, item.SummaryDate)
	if !s.beginTask(key) {
		return nil
	}
	defer s.finishTask(key)

	if err := s.deliverSummary(ctx, chat, item); err != nil {
		_ = s.store.Summaries.MarkDeliveryFailed(ctx, item.ChatID, item.SummaryDate, err.Error())
		return err
	}
	return s.store.Summaries.MarkDelivered(ctx, item.ChatID, item.SummaryDate, s.clock.Now())
}

func (s *Service) RepairEmptySummariesInRange(ctx context.Context, chat model.Chat, fromDate, toDate string) error {
	settings, err := s.store.Settings.Get(ctx)
	if err != nil {
		return err
	}

	timezone := settings.DefaultTimezone
	for _, date := range datesInRange(fromDate, toDate, timezone) {
		item, found, err := s.lookupSummary(ctx, chat.ID, date)
		if err != nil {
			return err
		}
		if !found || !isRepairableEmptySummary(item) {
			continue
		}

		start, end, err := summaryDayRange(date, timezone)
		if err != nil {
			return err
		}
		messageCount, err := s.store.Messages.CountForRange(ctx, chat.ID, start, end)
		if err != nil {
			return err
		}
		if messageCount == 0 {
			continue
		}
		if _, err := s.RunNowAsync(ctx, chat, date); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) runNow(ctx context.Context, chat model.Chat, date string) error {
	if err := s.store.Summaries.UpsertPending(ctx, chat.ID, date); err != nil {
		return err
	}
	if err := s.store.Summaries.SetRunning(ctx, chat.ID, date); err != nil {
		return err
	}
	return s.executeSummary(ctx, chat, date)
}

func (s *Service) executeSummary(ctx context.Context, chat model.Chat, date string) error {
	s.extractKnowledgeForSummary(ctx, chat, date)
	result, err := s.summaries.RunDailySummary(ctx, chat, date)
	if err != nil {
		return err
	}
	if err := s.store.Summaries.SaveResult(ctx, result); err != nil {
		return err
	}
	if result.Status != model.SummaryStatusSucceeded {
		return nil
	}
	s.tryDeliverSummary(ctx, chat, result)
	return nil
}

func (s *Service) extractKnowledgeForSummary(ctx context.Context, chat model.Chat, date string) []model.KnowledgeRun {
	if s.knowledgeExtractor == nil {
		return nil
	}
	runs, _ := s.knowledgeExtractor.EnsureDailyExtractionsForSummary(ctx, chat, date)
	return runs
}

func (s *Service) extractKnowledgeForChannel(ctx context.Context, channel model.DeliveryChannel, date string) []extractionRunReport {
	if s.knowledgeExtractor == nil || s.store == nil || s.store.Chats == nil {
		return nil
	}
	spaceNames := s.knowledgeSpaceNames(ctx)
	reports := make([]extractionRunReport, 0)
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(channelExtractionParallelism)
	for _, chatID := range uniqueChannelSourceChatIDs(channel.SourceChatIDs) {
		chatID := chatID
		group.Go(func() error {
			chat, err := s.store.Chats.GetByID(groupCtx, chatID)
			if err != nil {
				return nil
			}
			chatReports := make([]extractionRunReport, 0)
			for _, run := range s.extractKnowledgeForSummary(groupCtx, chat, date) {
				chatReports = append(chatReports, extractionRunReport{
					ChatTitle: chat.Title,
					SpaceName: spaceNames[run.SpaceID],
					Run:       run,
				})
			}
			mu.Lock()
			reports = append(reports, chatReports...)
			mu.Unlock()
			return nil
		})
	}
	_ = group.Wait()
	return reports
}

func (s *Service) knowledgeSpaceNames(ctx context.Context) map[int64]string {
	if s.store == nil || s.store.KnowledgeSpaces == nil {
		return nil
	}
	spaces, err := s.store.KnowledgeSpaces.List(ctx)
	if err != nil {
		return nil
	}
	names := make(map[int64]string, len(spaces))
	for _, space := range spaces {
		names[space.ID] = space.Name
	}
	return names
}

func uniqueChannelSourceChatIDs(chatIDs []int64) []int64 {
	seen := make(map[int64]struct{}, len(chatIDs))
	out := make([]int64, 0, len(chatIDs))
	for _, chatID := range chatIDs {
		if chatID <= 0 {
			continue
		}
		if _, ok := seen[chatID]; ok {
			continue
		}
		seen[chatID] = struct{}{}
		out = append(out, chatID)
	}
	return out
}

func (s *Service) runOnce(ctx context.Context) error {
	settings, err := s.store.Settings.Get(ctx)
	if err != nil {
		return err
	}

	chats, err := s.store.Chats.ListSummaryEnabled(ctx)
	if err != nil {
		return err
	}

	group, groupCtx := errgroup.WithContext(ctx)
	for _, chat := range chats {
		chat := chat
		timezone := settings.DefaultTimezone
		if !isDue(s.clock.Now(), chat, timezone) {
			continue
		}
		group.Go(func() error {
			date := targetDate(s.clock.Now(), timezone)
			item, found, err := s.lookupSummary(groupCtx, chat.ID, date)
			if err != nil {
				return err
			}

			switch decideScheduledAction(chat, item, found, timezone) {
			case scheduledActionSkip:
				return nil
			case scheduledActionDeliver:
				s.deliverExistingSummary(groupCtx, chat, item)
				return nil
			default:
				return s.RunNow(groupCtx, chat, date)
			}
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}

	return s.runChannelDeliveryOnce(ctx, settings)
}

func (s *Service) runChannelDeliveryOnce(ctx context.Context, settings model.AppSettings) error {
	if s.aggregator == nil {
		return nil
	}

	channels, err := s.store.DeliveryChannels.ListEnabled(ctx)
	if err != nil {
		return err
	}
	if len(channels) == 0 {
		return nil
	}

	for _, channel := range channels {
		timezone := resolveChannelTimezone(channel, settings.DefaultTimezone)
		if !isChannelDue(s.clock.Now(), channel, timezone) {
			continue
		}

		date := targetDate(s.clock.Now(), timezone)
		run, found, err := s.lookupChannelRun(ctx, channel.ID, date)
		if err != nil {
			return err
		}
		if channelRunDelivered(run, found) {
			continue
		}

		key := channelTaskKey(channel.ID, date)
		if !s.beginTask(key) {
			continue
		}
		started, err := s.store.DeliveryChannelRuns.TrySetRunning(ctx, channel.ID, date)
		if err != nil {
			s.finishTask(key)
			return err
		}
		if !started {
			s.finishTask(key)
			continue
		}

		go func(ch model.DeliveryChannel, d string, k string) {
			defer s.finishTask(k)
			runCtx := context.Background()
			if err := s.executeChannelSummary(runCtx, ch, d); err != nil {
				_ = s.store.DeliveryChannelRuns.MarkFailed(context.Background(), ch.ID, d, err.Error())
				fmt.Printf("channel %d summary failed: %v\n", ch.ID, err)
			}
		}(channel, date, key)
	}
	return nil
}

func (s *Service) executeChannelSummary(ctx context.Context, channel model.DeliveryChannel, date string) error {
	extractionReports := s.extractKnowledgeForChannel(ctx, channel, date)
	result, err := s.aggregator.RunAggregatedSummary(ctx, channel, date)
	if err != nil {
		return err
	}
	if result.Status != model.SummaryStatusSucceeded {
		return fmt.Errorf("aggregated summary failed: %s", result.ErrorMessage)
	}
	result.Content = appendChannelExtractionWarnings(result.Content, extractionReports, channel.TargetLanguage)

	if strings.TrimSpace(channel.TargetChatID) == "" {
		return fmt.Errorf("delivery channel target is not configured")
	}

	settings, err := s.store.Settings.Get(ctx)
	if err != nil {
		return err
	}
	if !settings.BotEnabled || strings.TrimSpace(settings.BotToken) == "" {
		return fmt.Errorf("bot is not enabled or token is not configured")
	}

	message := buildChannelDeliveryMessage(channel, result, date)
	if err := s.botService.SendMessageWithSummaryLanguage(ctx, settings.BotToken, channel.TargetChatID, message, channel.TargetLanguage); err != nil {
		return err
	}
	return s.store.DeliveryChannelRuns.MarkDelivered(ctx, channel.ID, date, result.Content, result.Model, result.GeneratedAt, s.clock.Now())
}

func appendChannelExtractionWarnings(content string, reports []extractionRunReport, language model.SummaryOutputLanguage) string {
	warning := formatChannelExtractionWarnings(reports, language)
	if warning == "" {
		return content
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return warning
	}
	return content + "\n\n" + warning
}

func formatChannelExtractionWarnings(reports []extractionRunReport, language model.SummaryOutputLanguage) string {
	lines := make([]string, 0)
	for _, report := range reports {
		line := formatChannelExtractionWarning(report, language)
		if line == "" {
			continue
		}
		lines = append(lines, "- "+line)
	}
	if len(lines) == 0 {
		return ""
	}
	title := "## 抽取状态提示"
	if model.NormalizeSummaryOutputLanguage(language) == model.SummaryLanguageEN {
		title = "## Extraction Status"
	}
	return title + "\n" + strings.Join(lines, "\n")
}

func formatChannelExtractionWarning(report extractionRunReport, language model.SummaryOutputLanguage) string {
	if report.Run.Status != model.KnowledgeRunStatusFailed && report.Run.Status != model.KnowledgeRunStatusRunning {
		return ""
	}
	subject := extractionWarningSubject(report)
	if model.NormalizeSummaryOutputLanguage(language) == model.SummaryLanguageEN {
		if report.Run.Status == model.KnowledgeRunStatusRunning {
			return subject + ": extraction is still running; this summary may not include the latest knowledge."
		}
		return subject + ": extraction failed; existing knowledge was used." + extractionWarningErrorSuffix(report.Run.ErrorMessage, model.SummaryLanguageEN)
	}
	if report.Run.Status == model.KnowledgeRunStatusRunning {
		return subject + "：抽取仍在运行，本次汇总可能未包含最新情报。"
	}
	return subject + "：抽取失败，本次汇总已使用已有情报。" + extractionWarningErrorSuffix(report.Run.ErrorMessage, model.SummaryLanguageZhCN)
}

func extractionWarningSubject(report extractionRunReport) string {
	chatTitle := strings.TrimSpace(report.ChatTitle)
	if chatTitle == "" {
		chatTitle = fmt.Sprintf("chat:%d", report.Run.ChatID)
	}
	spaceName := strings.TrimSpace(report.SpaceName)
	if spaceName == "" {
		spaceName = fmt.Sprintf("space:%d", report.Run.SpaceID)
	}
	return chatTitle + " / " + spaceName
}

func extractionWarningErrorSuffix(message string, language model.SummaryOutputLanguage) string {
	message = compactWarningText(message, 120)
	if message == "" {
		return ""
	}
	if model.NormalizeSummaryOutputLanguage(language) == model.SummaryLanguageEN {
		return " Error: " + message
	}
	return "错误：" + message
}

func compactWarningText(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "…"
}

func channelRunDelivered(run model.DeliveryChannelRun, found bool) bool {
	return found && run.Status == model.SummaryStatusSucceeded && run.DeliveredAt != nil
}

func (s *Service) lookupChannelRun(ctx context.Context, channelID int64, date string) (model.DeliveryChannelRun, bool, error) {
	run, err := s.store.DeliveryChannelRuns.GetByChannelAndDate(ctx, channelID, date)
	if err == nil {
		return run, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return model.DeliveryChannelRun{}, false, nil
	}
	return model.DeliveryChannelRun{}, false, err
}

func isChannelDue(now time.Time, channel model.DeliveryChannel, timezone string) bool {
	location, err := loadSummaryLocation(timezone)
	if err != nil {
		return false
	}
	localNow := now.In(location)
	scheduled, err := time.ParseInLocation("15:04", channel.SummaryTimeLocal, location)
	if err != nil {
		return false
	}

	scheduledTime := time.Date(
		localNow.Year(),
		localNow.Month(),
		localNow.Day(),
		scheduled.Hour(),
		scheduled.Minute(),
		0,
		0,
		location,
	)
	return !localNow.Before(scheduledTime)
}

func resolveChannelTimezone(channel model.DeliveryChannel, fallback string) string {
	if timezone := strings.TrimSpace(channel.SummaryTimezone); timezone != "" {
		return timezone
	}
	return fallback
}

func channelTaskKey(channelID int64, date string) string {
	return fmt.Sprintf("channel:%d:%s", channelID, date)
}

func buildChannelDeliveryMessage(channel model.DeliveryChannel, result summary.AggregatedSummaryResult, date string) string {
	header := fmt.Sprintf("**%s · %s**", channel.Name, date)
	content := strings.TrimSpace(result.Content)
	if content == "" {
		return header
	}
	return header + "\n\n" + content
}

func (s *Service) deliverExistingSummary(ctx context.Context, chat model.Chat, result model.Summary) {
	key := summaryTaskKey(chat.ID, result.SummaryDate)
	if !s.beginTask(key) {
		return
	}
	defer s.finishTask(key)
	s.tryDeliverSummary(ctx, chat, result)
}

func (s *Service) tryDeliverSummary(ctx context.Context, chat model.Chat, result model.Summary) {
	if chat.DeliveryMode != model.DeliveryModeBot {
		return
	}

	if err := s.deliverSummary(ctx, chat, result); err != nil {
		_ = s.store.Summaries.MarkDeliveryFailed(ctx, result.ChatID, result.SummaryDate, err.Error())
		return
	}
	_ = s.store.Summaries.MarkDelivered(ctx, result.ChatID, result.SummaryDate, s.clock.Now())
}

func (s *Service) deliverSummary(ctx context.Context, chat model.Chat, result model.Summary) error {
	settings, err := s.store.Settings.Get(ctx)
	if err != nil {
		return err
	}
	if !settings.BotEnabled {
		return fmt.Errorf("bot delivery is disabled")
	}
	if strings.TrimSpace(settings.BotToken) == "" {
		return fmt.Errorf("bot delivery target is not configured")
	}
	targetChatID := resolveBotDeliveryTarget(settings, chat)
	if targetChatID == "" {
		return fmt.Errorf("bot delivery target is not configured")
	}

	message := buildBotDeliveryMessage(chat, result)
	return s.botService.SendMessageWithSummaryLanguage(ctx, settings.BotToken, targetChatID, message, model.ResolveSummaryOutputLanguage(settings, chat))
}

func resolveBotDeliveryTarget(settings model.AppSettings, chat model.Chat) string {
	if target := strings.TrimSpace(chat.BotChatID); target != "" {
		return target
	}
	return strings.TrimSpace(settings.BotTargetChatID)
}

func buildBotDeliveryMessage(chat model.Chat, result model.Summary) string {
	header := fmt.Sprintf("**%s · %s**", chat.Title, result.SummaryDate)
	content := strings.TrimSpace(result.Content)
	if content == "" {
		return header
	}
	return header + "\n\n" + content
}

func (s *Service) lookupSummary(ctx context.Context, chatID int64, date string) (model.Summary, bool, error) {
	item, err := s.store.Summaries.GetByChatAndDate(ctx, chatID, date)
	if err == nil {
		return item, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Summary{}, false, nil
	}
	return model.Summary{}, false, err
}

func decideScheduledAction(chat model.Chat, item model.Summary, found bool, timezone string) scheduledAction {
	if !found {
		return scheduledActionGenerate
	}
	if item.Status != model.SummaryStatusSucceeded {
		return scheduledActionGenerate
	}
	if chat.DeliveryMode != model.DeliveryModeBot {
		return scheduledActionSkip
	}
	if item.DeliveredAt != nil {
		return scheduledActionSkip
	}
	if !summaryReadyForDelivery(item, timezone) {
		return scheduledActionGenerate
	}
	return scheduledActionDeliver
}

func summaryReadyForDelivery(item model.Summary, timezone string) bool {
	location, err := loadSummaryLocation(timezone)
	if err != nil {
		return false
	}
	summaryDate, err := time.ParseInLocation("2006-01-02", item.SummaryDate, location)
	if err != nil {
		return false
	}
	windowEnd := summaryDate.AddDate(0, 0, 1)
	return !item.GeneratedAt.Before(windowEnd)
}

func isRepairableEmptySummary(item model.Summary) bool {
	return item.Status == model.SummaryStatusSucceeded &&
		item.SourceMessageCount == 0 &&
		item.ChunkCount == 0
}

func isDue(now time.Time, chat model.Chat, timezone string) bool {
	location, err := loadSummaryLocation(timezone)
	if err != nil {
		return false
	}
	localNow := now.In(location)
	scheduled, err := time.ParseInLocation("15:04", chat.SummaryTimeLocal, location)
	if err != nil {
		return false
	}

	scheduledTime := time.Date(
		localNow.Year(),
		localNow.Month(),
		localNow.Day(),
		scheduled.Hour(),
		scheduled.Minute(),
		0,
		0,
		location,
	)
	return !localNow.Before(scheduledTime)
}

func targetDate(now time.Time, timezone string) string {
	location, err := loadSummaryLocation(timezone)
	if err != nil {
		location = time.Local
	}
	localNow := now.In(location)
	return localNow.AddDate(0, 0, -1).Format("2006-01-02")
}

func datesInRange(fromDate, toDate, timezone string) []string {
	start, _, err := summaryDayRange(fromDate, timezone)
	if err != nil {
		return nil
	}
	endStart, _, err := summaryDayRange(toDate, timezone)
	if err != nil {
		return nil
	}
	location, err := loadSummaryLocation(timezone)
	if err != nil {
		return nil
	}

	dates := make([]string, 0)
	for current := start.In(location); !current.After(endStart.In(location)); current = current.AddDate(0, 0, 1) {
		dates = append(dates, current.Format("2006-01-02"))
	}
	return dates
}

func summaryDayRange(date string, timezone string) (time.Time, time.Time, error) {
	location, err := loadSummaryLocation(timezone)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start, err := time.ParseInLocation("2006-01-02", date, location)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse summary date %s: %w", date, err)
	}
	end := start.AddDate(0, 0, 1)
	return start.UTC(), end.UTC(), nil
}

func loadSummaryLocation(timezone string) (*time.Location, error) {
	if strings.TrimSpace(timezone) == "" {
		return time.Local, nil
	}
	return time.LoadLocation(timezone)
}

func summaryTaskKey(chatID int64, date string) string {
	return fmt.Sprintf("%d:%s", chatID, date)
}

func (s *Service) beginTask(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.inflight[key]; exists {
		return false
	}
	s.inflight[key] = struct{}{}
	return true
}

func (s *Service) finishTask(key string) {
	s.mu.Lock()
	delete(s.inflight, key)
	s.mu.Unlock()
}
