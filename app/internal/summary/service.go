package summary

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/frederic/tgtldr/app/internal/clock"
	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/openai"
	"github.com/frederic/tgtldr/app/internal/store"
	"golang.org/x/sync/errgroup"
)

type Service struct {
	store         *store.Store
	clock         clock.Clock
	openAITimeout time.Duration
}

func NewService(st *store.Store, c clock.Clock, openAITimeout time.Duration) *Service {
	return &Service{store: st, clock: c, openAITimeout: openAITimeout}
}

func (s *Service) BuildContextPreview(ctx context.Context, summary model.Summary) (model.SummaryContextPreview, error) {
	settings, err := s.store.Settings.Get(ctx)
	if err != nil {
		return model.SummaryContextPreview{}, err
	}

	chat, err := s.store.Chats.GetByID(ctx, summary.ChatID)
	if err != nil {
		return model.SummaryContextPreview{}, err
	}

	timezone := resolveSummaryTimezone(chat, settings.DefaultTimezone)
	location, err := loadLocation(timezone)
	if err != nil {
		return model.SummaryContextPreview{}, err
	}
	start, end, err := dayRange(summary.SummaryDate, timezone)
	if err != nil {
		return model.SummaryContextPreview{}, err
	}

	messages, err := s.store.Messages.ListForRange(ctx, chat.ID, start, end)
	if err != nil {
		return model.SummaryContextPreview{}, err
	}

	filteredMessages, messageLookup, err := s.prepareMessages(ctx, chat, messages)
	if err != nil {
		return model.SummaryContextPreview{}, err
	}
	summaryLanguage := model.ResolveSummaryOutputLanguage(settings, chat)
	stagePrompt := buildStagePromptForChat(summaryLanguage, chat)
	finalPrompt := buildFinalPromptForChat(summaryLanguage, chat)
	budget := resolveSummaryBudget(settings, resolveSummaryModel(chat, settings), stagePrompt)
	chunks := SplitMessages(filteredMessages, budget.ChunkTokenBudget)
	preview := model.SummaryContextPreview{
		SummaryID:        summary.ID,
		ChatID:           summary.ChatID,
		SummaryDate:      summary.SummaryDate,
		Model:            resolveSummaryModel(chat, settings),
		SystemPrompt:     stagePrompt,
		FinalPrompt:      finalPrompt,
		MessageCount:     len(filteredMessages),
		ChunkCount:       len(chunks),
		FinalInputNotice: finalInputNotice(summaryLanguage),
		PreviewNotice:    previewNotice(summaryLanguage),
	}

	for _, chunk := range chunks {
		preview.Chunks = append(preview.Chunks, model.SummaryContextChunk{
			Index:        chunk.Index,
			MessageCount: len(chunk.Messages),
			Content:      BuildTranscript(chunk.Messages, messageLookup, location, summaryLanguage),
		})
	}
	if len(chunks) <= 1 {
		preview.FinalPrompt = ""
		preview.FinalInputNotice = ""
	}
	return preview, nil
}

func (s *Service) RunDailySummary(ctx context.Context, chat model.Chat, date string) (model.Summary, error) {
	settings, err := s.store.Settings.Get(ctx)
	if err != nil {
		return model.Summary{}, err
	}
	summaryLanguage := model.ResolveSummaryOutputLanguage(settings, chat)

	timezone := resolveSummaryTimezone(chat, settings.DefaultTimezone)
	location, err := loadLocation(timezone)
	if err != nil {
		return model.Summary{}, err
	}
	start, end, err := dayRange(date, timezone)
	if err != nil {
		return model.Summary{}, err
	}

	messages, err := s.store.Messages.ListForRange(ctx, chat.ID, start, end)
	if err != nil {
		return model.Summary{}, err
	}
	filteredMessages, messageLookup, err := s.prepareMessages(ctx, chat, messages)
	if err != nil {
		return model.Summary{}, err
	}

	summary := model.Summary{
		ChatID:             chat.ID,
		SummaryDate:        date,
		Status:             model.SummaryStatusSucceeded,
		Model:              resolveSummaryModel(chat, settings),
		SourceMessageCount: len(filteredMessages),
		GeneratedAt:        s.clock.Now(),
	}
	if len(filteredMessages) == 0 {
		summary.Content = emptySummaryContent(summaryLanguage)
		if err := s.appendKnowledgeFacts(ctx, &summary, summaryLanguage, chat.SummaryKnowledgeDays, end); err != nil {
			return model.Summary{}, err
		}
		return summary, nil
	}

	client := openai.New(openai.Config{
		BaseURL: settings.OpenAIBaseURL,
		APIKey:  settings.OpenAIAPIKey,
		Model:   resolveSummaryModel(chat, settings),
		Timeout: s.openAITimeout,
		Stream:  settings.OpenAIStreamEnabled(),
	})

	stagePrompt := buildStagePromptForChat(summaryLanguage, chat)
	finalPrompt := buildFinalPromptForChat(summaryLanguage, chat)
	budget := resolveSummaryBudget(settings, resolveSummaryModel(chat, settings), stagePrompt)
	chunks := SplitMessages(filteredMessages, budget.ChunkTokenBudget)
	summary.ChunkCount = len(chunks)

	partials := make([]string, len(chunks))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(budget.Parallelism)

	for index, chunk := range chunks {
		index := index
		chunk := chunk
		group.Go(func() error {
			transcript := BuildTranscript(chunk.Messages, messageLookup, location, summaryLanguage)
			resp, err := chatOpenAIForSummary(groupCtx, client, openai.ChatRequest{
				SystemPrompt: stagePrompt,
				UserPrompt:   transcript,
				Temperature:  settings.OpenAITemperature,
				MaxOutput:    budget.StageRequestMax,
			}, summaryOpenAICallContext{
				Kind:               "summary",
				Stage:              "chunk",
				ChatID:             chat.ID,
				SummaryDate:        date,
				Timezone:           timezone,
				Model:              summary.Model,
				BaseURL:            settings.OpenAIBaseURL,
				RequestMode:        model.NormalizeOpenAIRequestMode(settings.OpenAIRequestMode),
				Temperature:        settings.OpenAITemperature,
				MaxOutput:          budget.StageRequestMax,
				Parallelism:        budget.Parallelism,
				ChunkIndex:         index,
				ChunkCount:         len(chunks),
				SourceMessageCount: len(filteredMessages),
				ChunkMessageCount:  len(chunk.Messages),
				InputRunes:         len([]rune(transcript)),
			})
			if err != nil {
				return err
			}
			partials[index] = strings.TrimSpace(resp.Content)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		summary.Status = model.SummaryStatusFailed
		setSummaryOpenAIError(&summary, err)
		return summary, nil
	}

	finalInput := strings.Join(partials, "\n\n---\n\n")
	finalResp, err := chatOpenAIForSummary(ctx, client, openai.ChatRequest{
		SystemPrompt: finalPrompt,
		UserPrompt:   finalInput,
		Temperature:  settings.OpenAITemperature,
		MaxOutput:    budget.FinalRequestMax,
	}, summaryOpenAICallContext{
		Kind:               "summary",
		Stage:              "final",
		ChatID:             chat.ID,
		SummaryDate:        date,
		Timezone:           timezone,
		Model:              summary.Model,
		BaseURL:            settings.OpenAIBaseURL,
		RequestMode:        model.NormalizeOpenAIRequestMode(settings.OpenAIRequestMode),
		Temperature:        settings.OpenAITemperature,
		MaxOutput:          budget.FinalRequestMax,
		Parallelism:        budget.Parallelism,
		ChunkCount:         len(chunks),
		SourceMessageCount: len(filteredMessages),
		InputRunes:         len([]rune(finalInput)),
	})
	if err != nil {
		summary.Status = model.SummaryStatusFailed
		setSummaryOpenAIError(&summary, err)
		return summary, nil
	}

	summary.Content = sanitizeSummaryUserLinks(strings.TrimSpace(finalResp.Content), filteredMessages, messageLookup)
	summary.Model = finalResp.Model
	if err := s.appendKnowledgeFacts(ctx, &summary, summaryLanguage, chat.SummaryKnowledgeDays, end); err != nil {
		return model.Summary{}, err
	}
	return summary, nil
}

func setSummaryOpenAIError(summary *model.Summary, err error) {
	summary.ErrorMessage = err.Error()
	summary.ErrorContext = summaryOpenAIErrorContext(err)
	summary.ErrorSystemPrompt = summaryOpenAIErrorSystemPrompt(err)
	summary.ErrorUserPrompt = summaryOpenAIErrorUserPrompt(err)
	summary.RetryableError = summaryOpenAIErrorRetryable(err)
}

func (s *Service) appendKnowledgeFacts(ctx context.Context, summary *model.Summary, language model.SummaryOutputLanguage, days int, before time.Time) error {
	if s.store == nil || s.store.KnowledgeFacts == nil || summary == nil || summary.Status != model.SummaryStatusSucceeded {
		return nil
	}
	content, err := appendKnowledgeFactsForChats(ctx, s.store, s.clock.Now(), summary.Content, []int64{summary.ChatID}, language, days, before, nil)
	if err != nil {
		return err
	}
	summary.Content = content
	return nil
}

func resolveSummaryModel(chat model.Chat, settings model.AppSettings) string {
	if strings.TrimSpace(chat.ModelOverride) != "" {
		return strings.TrimSpace(chat.ModelOverride)
	}
	return settings.OpenAIModel
}

func resolveSummaryTimezone(chat model.Chat, fallback string) string {
	if timezone := strings.TrimSpace(chat.SummaryTimezone); timezone != "" {
		return timezone
	}
	if timezone := strings.TrimSpace(fallback); timezone != "" {
		return timezone
	}
	return time.Local.String()
}

func loadLocation(timezone string) (*time.Location, error) {
	if strings.TrimSpace(timezone) == "" {
		return time.Local, nil
	}

	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load location %s: %w", timezone, err)
	}
	return location, nil
}

func dayRange(date string, timezone string) (time.Time, time.Time, error) {
	location, err := loadLocation(timezone)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start, err := time.ParseInLocation("2006-01-02", date, location)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parse date %s: %w", date, err)
	}
	end := start.Add(24 * time.Hour)
	return start.UTC(), end.UTC(), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Service) prepareMessages(ctx context.Context, chat model.Chat, messages []model.Message) ([]model.Message, map[int]model.Message, error) {
	lookup := make(map[int]model.Message, len(messages))
	for _, message := range messages {
		lookup[message.TelegramMessageID] = message
	}

	missingReplyIDs := make([]int, 0)
	for _, message := range messages {
		if message.ReplyToMessageID == 0 {
			continue
		}
		if _, ok := lookup[message.ReplyToMessageID]; ok {
			continue
		}
		missingReplyIDs = append(missingReplyIDs, message.ReplyToMessageID)
	}

	if len(missingReplyIDs) > 0 && s.store != nil && s.store.Messages != nil {
		referenced, err := s.store.Messages.LookupByTelegramIDs(ctx, chat.ID, uniqueInts(missingReplyIDs))
		if err != nil {
			return nil, nil, err
		}
		for messageID, message := range referenced {
			lookup[messageID] = message
		}
	}

	if s.store != nil && s.store.Messages != nil {
		if err := enrichSenderUsernames(ctx, s.store.Messages, messages, lookup); err != nil {
			return nil, nil, err
		}
	}

	return filterMessagesForSummary(messages, chat), lookup, nil
}

func enrichSenderUsernames(ctx context.Context, messagesRepo *store.MessageRepository, messages []model.Message, lookup map[int]model.Message) error {
	if messagesRepo == nil {
		return nil
	}
	usernames, err := messagesRepo.LatestSenderUsernames(ctx, collectSenderIDs(messages, lookup))
	if err != nil {
		return err
	}
	if len(usernames) == 0 {
		return nil
	}
	applySenderUsernames(messages, lookup, usernames)
	return nil
}

func collectSenderIDs(messages []model.Message, lookup map[int]model.Message) []int64 {
	ids := make([]int64, 0, len(messages)+len(lookup))
	for _, message := range messages {
		ids = append(ids, message.TelegramSenderID)
	}
	for _, message := range lookup {
		ids = append(ids, message.TelegramSenderID)
	}
	return ids
}

func applySenderUsernames(messages []model.Message, lookup map[int]model.Message, usernames map[int64]string) {
	for index := range messages {
		if strings.TrimSpace(messages[index].SenderUsername) != "" {
			continue
		}
		if username := strings.TrimSpace(usernames[messages[index].TelegramSenderID]); username != "" {
			messages[index].SenderUsername = username
		}
	}
	for key, message := range lookup {
		if strings.TrimSpace(message.SenderUsername) != "" {
			continue
		}
		if username := strings.TrimSpace(usernames[message.TelegramSenderID]); username != "" {
			message.SenderUsername = username
			lookup[key] = message
		}
	}
}

func filterMessagesForSummary(messages []model.Message, chat model.Chat) []model.Message {
	filtered := make([]model.Message, 0, len(messages))
	for _, message := range messages {
		if shouldSkipMessage(message, chat) {
			continue
		}
		if strings.TrimSpace(message.SummaryText()) == "" {
			continue
		}
		filtered = append(filtered, message)
	}
	return filtered
}

func shouldSkipMessage(message model.Message, chat model.Chat) bool {
	if !chat.KeepBotMessages && message.SenderIsBot {
		return true
	}
	if matchesFilteredSender(message, chat.FilteredSenders) {
		return true
	}
	return matchesFilteredKeyword(message, chat.FilteredKeywords)
}

func matchesFilteredSender(message model.Message, filters []string) bool {
	if len(filters) == 0 {
		return false
	}

	name := normalizeFilterToken(message.SenderName)
	username := normalizeFilterToken(message.SenderUsername)

	for _, filter := range filters {
		target := normalizeFilterToken(filter)
		if target == "" {
			continue
		}
		if target == name || target == username {
			return true
		}
		if strings.HasPrefix(target, "@") && strings.TrimPrefix(target, "@") == username {
			return true
		}
	}
	return false
}

func matchesFilteredKeyword(message model.Message, filters []string) bool {
	if len(filters) == 0 {
		return false
	}

	text := normalizeFilterToken(message.SummaryText())
	if text == "" {
		return false
	}

	for _, filter := range filters {
		target := normalizeFilterToken(filter)
		if target == "" {
			continue
		}
		if strings.Contains(text, target) {
			return true
		}
	}
	return false
}

func normalizeFilterToken(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(trimmed)
}
