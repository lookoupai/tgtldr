package botquery

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/frederic/tgtldr/app/internal/bot"
	"github.com/frederic/tgtldr/app/internal/knowledge"
	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/store"
)

const (
	commandPollTimeoutSeconds = 10
	commandIdleDelay          = 15 * time.Second
	commandErrorDelay         = 30 * time.Second
	commandResultLimit        = 20
	pendingMaintenanceTTL     = 10 * time.Minute
	recentAnswerTTL           = 10 * time.Minute
	asyncNaturalQueryLimit    = 2
)

type Service struct {
	store              *store.Store
	bot                *bot.Service
	maintainer         knowledgeMaintainer
	pendingMaintenance *pendingMaintenance
	pendingSeq         int64
	recentAnswersMu    sync.Mutex
	recentAnswers      map[string]recentKnowledgeAnswer
	asyncQuerySlots    chan struct{}
}

type parsedCommand struct {
	query            string
	factType         string
	start            bool
	help             bool
	id               bool
	settings         bool
	factID           int64
	statusUpdate     model.KnowledgeFactStatus
	updateText       string
	naturalQueryText string
	confirm          bool
	confirmToken     string
	cancel           bool
}

type knowledgeMaintainer interface {
	ApplyMaintenanceText(ctx context.Context, text string) (knowledge.MaintenanceResult, error)
	AnswerQueryText(ctx context.Context, text string, opts knowledge.KnowledgeAnswerOptions) (knowledge.KnowledgeAnswerResult, error)
	ClassifyBotIntentText(ctx context.Context, text string) (knowledge.BotIntent, error)
	PreviewMaintenanceText(ctx context.Context, text string) (knowledge.MaintenanceResult, error)
	ParseQueryText(ctx context.Context, text string) (knowledge.KnowledgeQueryInstruction, error)
	RecordBotIntentFact(ctx context.Context, intent knowledge.BotIntent, source knowledge.InlineFactSource) (knowledge.InlineFactResult, error)
	UpdateFactStatus(ctx context.Context, factID int64, status model.KnowledgeFactStatus, source string, reason string, operatorText string, matchedQuery string) (model.KnowledgeFact, error)
}

type pendingMaintenance struct {
	token     string
	text      string
	result    knowledge.MaintenanceResult
	expiresAt time.Time
}

type recentKnowledgeAnswer struct {
	query     string
	factType  string
	expiresAt time.Time
}

type asyncKnowledgeRequest struct {
	text      string
	useIntent bool
}

type pollState struct {
	token        string
	targetChatID string
	botID        int64
	botUsername  string
	offset       int64
	initialized  bool
}

type responseTarget struct {
	chatID       int64
	language     model.SummaryOutputLanguage
	allowedUsers []string
	blockedUsers []string
}

type commandDefinition struct {
	command       string
	descriptionZH string
	descriptionEN string
}

var commandDefinitions = []commandDefinition{
	{command: "start", descriptionZH: "查看 Bot 说明", descriptionEN: "Show bot introduction"},
	{command: "help", descriptionZH: "查看命令帮助", descriptionEN: "Show command help"},
	{command: "id", descriptionZH: "查看当前 Chat ID 和用户 ID", descriptionEN: "Show current chat and user IDs"},
	{command: "knowledge", descriptionZH: "按关键词查询知识", descriptionEN: "Search knowledge by keyword"},
	{command: "ask", descriptionZH: "用自然语言提问", descriptionEN: "Ask a natural-language question"},
	{command: "demand", descriptionZH: "查询需求事实", descriptionEN: "Search demand facts"},
	{command: "supply", descriptionZH: "查询供应事实", descriptionEN: "Search supply facts"},
	{command: "type", descriptionZH: "按事实类型查询", descriptionEN: "Search by fact type"},
	{command: "update", descriptionZH: "用自然语言维护事实", descriptionEN: "Maintain facts with natural language"},
	{command: "settings", descriptionZH: "查看 Bot 绑定状态", descriptionEN: "Show bot binding status"},
}

func NewService(st *store.Store, botService *bot.Service, maintainer knowledgeMaintainer) *Service {
	return &Service{
		store:         st,
		bot:           botService,
		maintainer:    maintainer,
		recentAnswers: make(map[string]recentKnowledgeAnswer),
		asyncQuerySlots: make(
			chan struct{},
			asyncNaturalQueryLimit,
		),
	}
}

func BotCommands(language model.Language) []bot.Command {
	commands := make([]bot.Command, 0, len(commandDefinitions))
	for _, definition := range commandDefinitions {
		description := definition.descriptionZH
		if language == model.LanguageEN {
			description = definition.descriptionEN
		}
		commands = append(commands, bot.Command{
			Command:     definition.command,
			Description: description,
		})
	}
	return commands
}

func CommandsEqual(a []bot.Command, b []bot.Command) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func SyncBotCommands(ctx context.Context, botService *bot.Service, settings model.AppSettings) error {
	if botService == nil || !settings.BotEnabled || strings.TrimSpace(settings.BotToken) == "" {
		return nil
	}
	return botService.SetMyCommands(ctx, settings.BotToken, BotCommands(settings.Language))
}

func (s *Service) Run(ctx context.Context) error {
	state := pollState{}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		settings, err := s.store.Settings.Get(ctx)
		if err != nil {
			if !sleep(ctx, commandErrorDelay) {
				return nil
			}
			continue
		}
		if !botQueryReady(settings) {
			state = pollState{}
			if !sleep(ctx, commandIdleDelay) {
				return nil
			}
			continue
		}

		token := strings.TrimSpace(settings.BotToken)
		targetChatID := strings.TrimSpace(settings.BotTargetChatID)
		if state.token != token || state.targetChatID != targetChatID {
			state = pollState{token: token, targetChatID: targetChatID}
		}

		if !state.initialized {
			self, err := s.bot.GetMe(ctx, token)
			if err != nil {
				s.markRuntimeError(ctx, state.botUsername, err)
				if !sleep(ctx, commandErrorDelay) {
					return nil
				}
				continue
			}
			state.botID = self.ID
			state.botUsername = strings.TrimSpace(self.Username)
			state.initialized = true
			continue
		}

		updates, err := s.bot.GetCommandUpdates(ctx, token, state.offset, commandPollTimeoutSeconds)
		if err != nil {
			s.markRuntimeError(ctx, state.botUsername, err)
			if !sleep(ctx, commandErrorDelay) {
				return nil
			}
			continue
		}
		s.markRuntimePoll(ctx, state.botUsername, len(updates) > 0)
		targets, err := s.responseTargets(ctx, settings)
		if err != nil {
			s.markRuntimeError(ctx, state.botUsername, err)
			if !sleep(ctx, commandErrorDelay) {
				return nil
			}
			continue
		}
		for _, update := range updates {
			if update.UpdateID >= state.offset {
				state.offset = update.UpdateID + 1
			}
			if shouldIgnoreBotOrigin(settings, update) {
				continue
			}
			s.recordTargetChatCandidate(ctx, state.botID, update)
			target, ok := targets[update.ChatID]
			if !ok && privateUpdateAllowed(settings, update) {
				target = responseTarget{}
				ok = true
			}
			language := responseLanguage(settings, target)
			if botUserBlocked(settings.BotBlockedUsers, target.blockedUsers, update) {
				if strings.EqualFold(strings.TrimSpace(update.ChatType), "private") {
					if err := s.bot.SendReplyWithLanguage(ctx, token, update.ChatID, commandBlockedUserText(language), language, update.MessageID); err != nil {
						s.markRuntimeError(ctx, state.botUsername, err)
						continue
					}
					s.markRuntimeHandled(ctx, state.botUsername)
				}
				continue
			}
			if !ok {
				if response, shouldReply := safeUtilityResponse(settings.Language, update); shouldReply {
					if err := s.bot.SendReplyWithLanguage(ctx, token, update.ChatID, response, settings.Language, update.MessageID); err != nil {
						s.markRuntimeError(ctx, state.botUsername, err)
						continue
					}
					s.markRuntimeHandled(ctx, state.botUsername)
				}
				continue
			}
			if !targetAllowsUpdate(target, update) {
				if response, shouldReply := safeUtilityResponse(language, update); shouldReply {
					if err := s.bot.SendReplyWithLanguage(ctx, token, update.ChatID, response, language, update.MessageID); err != nil {
						s.markRuntimeError(ctx, state.botUsername, err)
						continue
					}
					s.markRuntimeHandled(ctx, state.botUsername)
				}
				continue
			}
			if request, ok := s.asyncKnowledgeRequest(update, state.botID, state.botUsername); ok {
				if err := s.bot.SendReplyWithLanguage(ctx, token, update.ChatID, asyncNaturalQueryQueuedText(language), language, update.MessageID); err != nil {
					s.markRuntimeError(ctx, state.botUsername, err)
					continue
				}
				s.markRuntimeHandled(ctx, state.botUsername)
				s.startAsyncNaturalQuery(ctx, token, language, update, request, state.botUsername, target)
				continue
			}
			response, ok, err := s.responseForUpdate(ctx, language, update, state.botID, state.botUsername)
			if err != nil {
				response = commandErrorText(language, err)
				ok = true
			}
			if !ok {
				continue
			}
			if err := s.bot.SendReplyWithLanguage(ctx, token, update.ChatID, response, language, update.MessageID); err != nil {
				s.markRuntimeError(ctx, state.botUsername, err)
				continue
			}
			s.markRuntimeHandled(ctx, state.botUsername)
		}
	}
}

func shouldIgnoreBotOrigin(settings model.AppSettings, update bot.CommandUpdate) bool {
	return settings.BotIgnoreMessagesFromBots && update.FromIsBot
}

func (s *Service) responseTargets(ctx context.Context, settings model.AppSettings) (map[string]responseTarget, error) {
	targets := make(map[string]responseTarget)
	if targetChatID := strings.TrimSpace(settings.BotTargetChatID); targetChatID != "" {
		targets[targetChatID] = responseTarget{}
	}
	chats, err := s.store.Chats.ListBotInteractionEnabled(ctx)
	if err != nil {
		return nil, err
	}
	for _, chat := range chats {
		targetChatID := strings.TrimSpace(chat.BotChatID)
		if targetChatID == "" {
			continue
		}
		targets[targetChatID] = responseTarget{
			chatID:       chat.ID,
			language:     model.ResolveSummaryOutputLanguage(settings, chat),
			allowedUsers: chat.BotAllowedUsers,
			blockedUsers: chat.BotBlockedUsers,
		}
	}
	return targets, nil
}

func targetAllowsUpdate(target responseTarget, update bot.CommandUpdate) bool {
	if len(target.allowedUsers) == 0 {
		return true
	}
	return updateUserMatches(update, target.allowedUsers)
}

func botUserBlocked(globalBlockedUsers []string, targetBlockedUsers []string, update bot.CommandUpdate) bool {
	return updateUserMatches(update, globalBlockedUsers) || updateUserMatches(update, targetBlockedUsers)
}

func updateUserMatches(update bot.CommandUpdate, users []string) bool {
	fromID := ""
	if update.FromID != 0 {
		fromID = strconv.FormatInt(update.FromID, 10)
	}
	fromUsername := normalizeAllowedUsername(update.FromUsername)
	for _, user := range users {
		trimmed := strings.TrimSpace(user)
		if trimmed == "" {
			continue
		}
		if fromID != "" && trimmed == fromID {
			return true
		}
		if fromUsername != "" && normalizeAllowedUsername(trimmed) == fromUsername {
			return true
		}
	}
	return false
}

func privateUpdateAllowed(settings model.AppSettings, update bot.CommandUpdate) bool {
	if !strings.EqualFold(strings.TrimSpace(update.ChatType), "private") {
		return false
	}
	if len(settings.BotPrivateAllowedUsers) == 0 {
		return false
	}
	return targetAllowsUpdate(responseTarget{allowedUsers: settings.BotPrivateAllowedUsers}, update)
}

func normalizeAllowedUsername(value string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "@"))
}

func safeUtilityResponse(language model.Language, update bot.CommandUpdate) (string, bool) {
	command, ok := parseCommand(update.Text)
	if !ok {
		return "", false
	}
	if command.id {
		return commandIDText(language, update), true
	}
	if command.start || command.help {
		return commandUnboundHelpText(language), true
	}
	return "", false
}

func (s *Service) markRuntimePoll(ctx context.Context, username string, hasUpdates bool) {
	if s.store == nil || s.store.BotRuntime == nil {
		return
	}
	_ = s.store.BotRuntime.MarkPoll(ctx, username, hasUpdates)
}

func (s *Service) markRuntimeHandled(ctx context.Context, username string) {
	if s.store == nil || s.store.BotRuntime == nil {
		return
	}
	_ = s.store.BotRuntime.MarkHandled(ctx, username)
}

func (s *Service) markRuntimeError(ctx context.Context, username string, err error) {
	if s.store == nil || s.store.BotRuntime == nil {
		return
	}
	_ = s.store.BotRuntime.MarkError(ctx, username, err)
}

func (s *Service) recordTargetChatCandidate(ctx context.Context, botID int64, update bot.CommandUpdate) {
	if s.store == nil || s.store.BotTargetChats == nil {
		return
	}
	if botID == 0 || update.FromID == 0 || strings.TrimSpace(update.ChatID) == "" {
		return
	}
	if update.FromIsBot {
		return
	}
	messageDate := time.Now()
	if update.MessageDate > 0 {
		messageDate = time.Unix(update.MessageDate, 0)
	}
	_ = s.store.BotTargetChats.Upsert(ctx, model.BotTargetChatCandidate{
		BotID:        botID,
		ChatID:       update.ChatID,
		ChatType:     update.ChatType,
		Title:        update.ChatTitle,
		Username:     update.ChatUsername,
		FromUserID:   update.FromID,
		FromUsername: update.FromUsername,
		MessageDate:  messageDate,
		UpdateID:     update.UpdateID,
	})
}

func responseLanguage(settings model.AppSettings, target responseTarget) model.Language {
	if target.chatID == 0 {
		return settings.Language
	}
	if target.language == model.SummaryLanguageEN {
		return model.LanguageEN
	}
	return model.LanguageZhCN
}

func (s *Service) responseForUpdate(ctx context.Context, language model.Language, update bot.CommandUpdate, botID int64, botUsername string) (string, bool, error) {
	text := strings.TrimSpace(update.Text)
	if text == "" {
		return "", false, nil
	}
	if command, ok := parseCommand(text); ok && command.id {
		return commandIDText(language, update), true, nil
	}
	if strings.HasPrefix(text, "/") {
		return s.responseForCommandForChat(ctx, language, text, update.ChatID)
	}
	if strings.EqualFold(strings.TrimSpace(update.ChatType), "private") {
		return s.respondToNaturalQueryForChat(ctx, language, update.ChatID, text)
	}
	if query, ok := extractMentionQuery(text, botUsername); ok {
		return s.respondToNaturalQueryForChat(ctx, language, update.ChatID, query)
	}
	if botID != 0 && update.ReplyToBotID == botID {
		if maintenanceText, ok := s.replyCorrectionMaintenanceText(update.ChatID, text, time.Now()); ok {
			return s.applyMaintenanceText(ctx, language, maintenanceText)
		}
		return s.respondToNaturalQueryForChat(ctx, language, update.ChatID, text)
	}
	return "", false, nil
}

func (s *Service) asyncNaturalQueryText(update bot.CommandUpdate, botID int64, botUsername string) (string, bool) {
	request, ok := s.asyncKnowledgeRequest(update, botID, botUsername)
	if !ok {
		return "", false
	}
	return request.text, true
}

func (s *Service) asyncKnowledgeRequest(update bot.CommandUpdate, botID int64, botUsername string) (asyncKnowledgeRequest, bool) {
	text := strings.TrimSpace(update.Text)
	if text == "" {
		return asyncKnowledgeRequest{}, false
	}
	if strings.HasPrefix(text, "/") {
		command, ok := parseCommand(text)
		if !ok || command.naturalQueryText == "" {
			return asyncKnowledgeRequest{}, false
		}
		return asyncKnowledgeRequest{text: command.naturalQueryText}, true
	}
	if strings.EqualFold(strings.TrimSpace(update.ChatType), "private") {
		return asyncKnowledgeRequest{text: text, useIntent: !knowledge.LooksLikeQuestionText(text)}, true
	}
	if query, ok := extractMentionQuery(text, botUsername); ok {
		return asyncKnowledgeRequest{text: query, useIntent: !knowledge.LooksLikeQuestionText(query)}, true
	}
	if botID != 0 && update.ReplyToBotID == botID {
		if _, ok := s.replyCorrectionMaintenanceText(update.ChatID, text, time.Now()); ok {
			return asyncKnowledgeRequest{}, false
		}
		return asyncKnowledgeRequest{text: text, useIntent: !knowledge.LooksLikeQuestionText(text)}, true
	}
	return asyncKnowledgeRequest{}, false
}

func (s *Service) startAsyncNaturalQuery(
	parent context.Context,
	token string,
	language model.Language,
	update bot.CommandUpdate,
	request asyncKnowledgeRequest,
	botUsername string,
	target responseTarget,
) {
	if s == nil || s.bot == nil {
		return
	}
	go func() {
		select {
		case s.asyncQuerySlots <- struct{}{}:
			defer func() { <-s.asyncQuerySlots }()
		case <-parent.Done():
			return
		}

		var response string
		var err error
		if request.useIntent {
			response, _, err = s.respondToNaturalIntentForChat(parent, language, update.ChatID, request.text, inlineFactSource(update, target))
		} else {
			response, _, err = s.respondToNaturalQueryForChat(parent, language, update.ChatID, request.text)
		}
		if err != nil {
			response = commandErrorText(language, err)
		}
		if strings.TrimSpace(response) == "" {
			response = commandNaturalQueryEmptyText(language)
		}
		if err := s.bot.SendReplyWithLanguage(parent, token, update.ChatID, response, language, update.MessageID); err != nil {
			s.markRuntimeError(parent, botUsername, err)
			return
		}
		s.markRuntimeHandled(parent, botUsername)
	}()
}

func inlineFactSource(update bot.CommandUpdate, target responseTarget) knowledge.InlineFactSource {
	messageTime := time.Time{}
	if update.MessageDate > 0 {
		messageTime = time.Unix(update.MessageDate, 0)
	}
	return knowledge.InlineFactSource{
		ChatID:         target.chatID,
		MessageID:      update.MessageID,
		SenderID:       update.FromID,
		SenderUsername: update.FromUsername,
		MessageTime:    messageTime,
	}
}

func (s *Service) responseForCommand(ctx context.Context, language model.Language, text string) (string, bool, error) {
	return s.responseForCommandForChat(ctx, language, text, "")
}

func (s *Service) responseForCommandForChat(ctx context.Context, language model.Language, text string, chatID string) (string, bool, error) {
	s.expirePendingMaintenance(time.Now())
	command, ok := parseCommand(text)
	if !ok {
		return "", false, nil
	}
	if command.start {
		return commandStartText(language), true, nil
	}
	if command.help {
		return commandHelpText(language), true, nil
	}
	if command.id {
		if language == model.LanguageEN {
			return "Send /id in Telegram to show the current Chat ID and your User ID.", true, nil
		}
		return "请在 Telegram 会话中发送 /id，以查看当前 Chat ID 和你的 User ID。", true, nil
	}
	if command.settings {
		return s.commandSettingsText(ctx, language)
	}
	if command.cancel {
		return s.cancelMaintenance(language), true, nil
	}
	if command.confirm {
		return s.confirmMaintenance(ctx, language, command.confirmToken)
	}
	if command.statusUpdate != "" {
		return s.updateFactStatus(ctx, language, command.factID, command.statusUpdate)
	}
	if command.updateText != "" {
		return s.applyMaintenanceText(ctx, language, command.updateText)
	}
	if command.naturalQueryText != "" {
		return s.respondToNaturalQueryForChat(ctx, language, chatID, command.naturalQueryText)
	}

	return s.renderKnowledgeQuery(ctx, language, command.query, command.factType)
}

func (s *Service) renderKnowledgeQuery(ctx context.Context, language model.Language, query string, factType string) (string, bool, error) {
	now := time.Now()
	if err := s.store.KnowledgeFacts.ExpireDue(ctx, now); err != nil {
		return "", true, err
	}
	facts, err := s.store.KnowledgeFacts.List(ctx, store.KnowledgeFactFilter{
		Status:   model.KnowledgeFactStatusActive,
		FactType: factType,
		Query:    query,
		Limit:    commandResultLimit,
	})
	if err != nil {
		return "", true, err
	}
	subjects, err := s.store.KnowledgeFacts.ListSubjects(ctx, store.KnowledgeSubjectFilter{
		FactType: factType,
		Query:    query,
		Limit:    commandResultLimit,
	})
	if err != nil {
		return "", true, err
	}
	return knowledge.FormatQueryResult(language, query, factType, facts, subjects), true, nil
}

func (s *Service) updateFactStatus(ctx context.Context, language model.Language, factID int64, status model.KnowledgeFactStatus) (string, bool, error) {
	if factID <= 0 {
		return commandHelpText(language), true, nil
	}
	var fact model.KnowledgeFact
	var err error
	if s.maintainer != nil {
		fact, err = s.maintainer.UpdateFactStatus(ctx, factID, status, knowledge.MaintenanceSourceBotCommand, "", "", fmt.Sprintf("#%d", factID))
	} else {
		fact, err = s.store.KnowledgeFacts.UpdateStatus(ctx, factID, status)
	}
	if err != nil {
		return "", true, err
	}
	return commandStatusUpdatedText(language, fact, status), true, nil
}

func (s *Service) applyMaintenanceText(ctx context.Context, language model.Language, text string) (string, bool, error) {
	if s.maintainer == nil {
		return "", true, fmt.Errorf("knowledge maintainer is not configured")
	}
	result, err := s.maintainer.PreviewMaintenanceText(ctx, text)
	if err != nil {
		return "", true, err
	}
	if !maintenanceResultNeedsConfirmation(result) {
		return commandMaintenanceResultText(language, result), true, nil
	}
	token := s.setPendingMaintenance(text, result)
	return commandMaintenancePreviewText(language, result, token), true, nil
}

func (s *Service) respondToNaturalQuery(ctx context.Context, language model.Language, text string) (string, bool, error) {
	return s.respondToNaturalQueryForChat(ctx, language, "", text)
}

func (s *Service) respondToNaturalIntentForChat(ctx context.Context, language model.Language, chatID string, text string, source knowledge.InlineFactSource) (string, bool, error) {
	if s.maintainer == nil {
		return "", true, fmt.Errorf("knowledge maintainer is not configured")
	}
	intent, err := s.maintainer.ClassifyBotIntentText(ctx, text)
	if err != nil {
		return "", true, err
	}

	switch intent.Intent {
	case knowledge.BotIntentQuery:
		return s.respondToNaturalQueryForChat(ctx, language, chatID, text)
	case knowledge.BotIntentFactUpsert:
		return s.respondToFactUpsertIntent(ctx, language, text, intent, source)
	case knowledge.BotIntentMaintenance, knowledge.BotIntentCorrection:
		if intent.LowConfidence() {
			return commandBotIntentAmbiguousText(language), true, nil
		}
		return s.applyMaintenanceText(ctx, language, text)
	case knowledge.BotIntentIgnore:
		return commandNaturalQueryEmptyText(language), true, nil
	default:
		return s.respondToNaturalQueryForChat(ctx, language, chatID, text)
	}
}

func (s *Service) respondToFactUpsertIntent(ctx context.Context, language model.Language, text string, intent knowledge.BotIntent, source knowledge.InlineFactSource) (string, bool, error) {
	if intent.LowConfidence() {
		return commandBotIntentAmbiguousText(language), true, nil
	}
	if source.ChatID <= 0 {
		return commandFactRecordNeedsBoundChatText(language), true, nil
	}
	if riskClearIntent(intent) {
		result, err := s.maintainer.PreviewMaintenanceText(ctx, text)
		if err != nil {
			return "", true, err
		}
		if maintenanceResultNeedsConfirmation(result) {
			token := s.setPendingMaintenance(text, result)
			return commandMaintenancePreviewText(language, result, token), true, nil
		}
	}
	result, err := s.maintainer.RecordBotIntentFact(ctx, intent, source)
	if err != nil {
		return "", true, err
	}
	if len(result.Facts) == 0 {
		return commandBotIntentAmbiguousText(language), true, nil
	}
	return commandInlineFactRecordedText(language, result.Facts), true, nil
}

func riskClearIntent(intent knowledge.BotIntent) bool {
	return strings.EqualFold(strings.TrimSpace(intent.FactType), "risk_account") &&
		strings.EqualFold(strings.TrimSpace(intent.Action), "cleared")
}

func (s *Service) respondToNaturalQueryForChat(ctx context.Context, language model.Language, chatID string, text string) (string, bool, error) {
	if s.maintainer == nil {
		return "", true, fmt.Errorf("knowledge maintainer is not configured")
	}
	answer, err := s.maintainer.AnswerQueryText(ctx, text, knowledge.KnowledgeAnswerOptions{Limit: commandResultLimit})
	if err == nil {
		if strings.TrimSpace(answer.Query) == "" && strings.TrimSpace(answer.FactType) == "" {
			return commandNaturalQueryEmptyText(language), true, nil
		}
		if strings.TrimSpace(answer.Answer) != "" {
			s.rememberRecentAnswer(chatID, answer, time.Now())
			return answer.Answer, true, nil
		}
	}

	query, err := s.maintainer.ParseQueryText(ctx, text)
	if err != nil {
		return "", true, err
	}
	if strings.TrimSpace(query.Query) == "" && strings.TrimSpace(query.FactType) == "" {
		return commandNaturalQueryEmptyText(language), true, nil
	}
	return s.renderKnowledgeQuery(ctx, language, query.Query, query.FactType)
}

func (s *Service) rememberRecentAnswer(chatID string, answer knowledge.KnowledgeAnswerResult, now time.Time) {
	if strings.TrimSpace(chatID) == "" || strings.TrimSpace(answer.Query) == "" {
		return
	}
	s.recentAnswersMu.Lock()
	defer s.recentAnswersMu.Unlock()
	if s.recentAnswers == nil {
		s.recentAnswers = make(map[string]recentKnowledgeAnswer)
	}
	s.recentAnswers[chatID] = recentKnowledgeAnswer{
		query:     strings.TrimSpace(answer.Query),
		factType:  strings.TrimSpace(answer.FactType),
		expiresAt: now.Add(recentAnswerTTL),
	}
}

func (s *Service) replyCorrectionMaintenanceText(chatID string, text string, now time.Time) (string, bool) {
	correction, ok := parseReplyCorrectionText(text)
	if !ok || strings.TrimSpace(chatID) == "" {
		return "", false
	}
	s.recentAnswersMu.Lock()
	recent, ok := s.recentAnswers[chatID]
	s.recentAnswersMu.Unlock()
	if !ok || now.After(recent.expiresAt) || strings.TrimSpace(recent.query) == "" {
		return "", false
	}
	verb := "是"
	switch strings.ToLower(strings.TrimSpace(recent.factType)) {
	case "supply":
		verb = "供应的是"
	case "demand":
		verb = "需要的是"
	}
	return fmt.Sprintf("%s %s %s，不是 %s", recent.query, verb, correction.replacement, correction.wrong), true
}

type replyCorrection struct {
	wrong       string
	replacement string
}

func parseReplyCorrectionText(text string) (replyCorrection, bool) {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		return replyCorrection{}, false
	}
	patterns := []struct {
		leftMarker  string
		rightMarker string
	}{
		{leftMarker: "不是", rightMarker: "，是"},
		{leftMarker: "不是", rightMarker: ",是"},
		{leftMarker: "不是", rightMarker: " 是"},
		{leftMarker: "不是", rightMarker: "，而是"},
		{leftMarker: "不是", rightMarker: ",而是"},
	}
	for _, pattern := range patterns {
		left := strings.Index(normalized, pattern.leftMarker)
		if left < 0 {
			continue
		}
		afterLeft := normalized[left+len(pattern.leftMarker):]
		right := strings.Index(afterLeft, pattern.rightMarker)
		if right < 0 {
			continue
		}
		wrong := strings.Trim(afterLeft[:right], " \t\r\n，,。.;；：:")
		replacement := strings.Trim(afterLeft[right+len(pattern.rightMarker):], " \t\r\n，,。.;；：:")
		if wrong != "" && replacement != "" {
			return replyCorrection{wrong: wrong, replacement: replacement}, true
		}
	}
	return replyCorrection{}, false
}

func (s *Service) confirmMaintenance(ctx context.Context, language model.Language, token string) (string, bool, error) {
	if s.maintainer == nil {
		return "", true, fmt.Errorf("knowledge maintainer is not configured")
	}
	pending := s.pendingMaintenance
	if pending == nil {
		return commandMaintenanceNoPendingText(language), true, nil
	}
	if strings.TrimSpace(token) == "" {
		return commandMaintenanceConfirmUsageText(language, pending.token), true, nil
	}
	if strings.TrimSpace(token) != pending.token {
		return commandMaintenanceTokenMismatchText(language), true, nil
	}
	if strings.EqualFold(pending.result.Action, "correct") {
		result, err := s.maintainer.ApplyMaintenanceText(ctx, pending.text)
		if err != nil {
			return "", true, err
		}
		s.pendingMaintenance = nil
		return commandMaintenanceResultText(language, result), true, nil
	}
	targetStatus := maintenanceTargetStatus(pending.result.Action)
	if targetStatus == "" {
		s.pendingMaintenance = nil
		return commandMaintenanceResultText(language, pending.result), true, nil
	}

	result := pending.result
	result.UpdatedFacts = nil
	for _, fact := range pending.result.MatchedFacts {
		updated, err := s.maintainer.UpdateFactStatus(
			ctx,
			fact.ID,
			targetStatus,
			knowledge.MaintenanceSourceBotUpdate,
			pending.result.Reason,
			pending.text,
			pending.result.TargetQuery,
		)
		if err != nil {
			return "", true, err
		}
		result.UpdatedFacts = append(result.UpdatedFacts, updated)
	}
	s.pendingMaintenance = nil
	return commandMaintenanceResultText(language, result), true, nil
}

func (s *Service) cancelMaintenance(language model.Language) string {
	if s.pendingMaintenance == nil {
		return commandMaintenanceNoPendingText(language)
	}
	s.pendingMaintenance = nil
	return commandMaintenanceCancelledText(language)
}

func (s *Service) setPendingMaintenance(text string, result knowledge.MaintenanceResult) string {
	s.pendingSeq++
	token := fmt.Sprintf("%06d", s.pendingSeq%1000000)
	s.pendingMaintenance = &pendingMaintenance{
		token:     token,
		text:      strings.TrimSpace(text),
		result:    result,
		expiresAt: time.Now().Add(pendingMaintenanceTTL),
	}
	return token
}

func (s *Service) expirePendingMaintenance(now time.Time) {
	if s.pendingMaintenance != nil && now.After(s.pendingMaintenance.expiresAt) {
		s.pendingMaintenance = nil
	}
}

func parseCommand(text string) (parsedCommand, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") {
		return parsedCommand{}, false
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return parsedCommand{}, false
	}

	name := strings.TrimPrefix(fields[0], "/")
	if index := strings.Index(name, "@"); index >= 0 {
		name = name[:index]
	}
	name = strings.ToLower(strings.TrimSpace(name))
	query := strings.TrimSpace(strings.TrimPrefix(trimmed, fields[0]))

	switch name {
	case "start":
		return parsedCommand{start: true}, true
	case "help":
		return parsedCommand{help: true}, true
	case "id":
		return parsedCommand{id: true}, true
	case "settings", "status":
		return parsedCommand{settings: true}, true
	case "confirm":
		return parsedCommand{confirm: true, confirmToken: strings.TrimSpace(query)}, true
	case "cancel", "abort":
		return parsedCommand{cancel: true}, true
	case "expire", "resolve":
		return parseStatusCommand(query, model.KnowledgeFactStatusExpired)
	case "forget", "dismiss":
		return parseStatusCommand(query, model.KnowledgeFactStatusDismissed)
	case "restore":
		return parseStatusCommand(query, model.KnowledgeFactStatusActive)
	case "update", "maintain":
		if query == "" {
			return parsedCommand{help: true}, true
		}
		return parsedCommand{updateText: query}, true
	case "ask", "question", "nlquery":
		if query == "" {
			return parsedCommand{help: true}, true
		}
		return parsedCommand{naturalQueryText: query}, true
	case "type", "fact", "facts":
		return parseTypedCommand(query)
	case "knowledge", "know", "query", "who":
		return parsedCommand{query: query}, true
	case "demand", "need":
		return parsedCommand{query: query, factType: "demand"}, true
	case "supply", "offer":
		return parsedCommand{query: query, factType: "supply"}, true
	default:
		return parsedCommand{}, false
	}
}

func extractMentionQuery(text string, botUsername string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	username := strings.TrimPrefix(strings.TrimSpace(botUsername), "@")
	if trimmed == "" || username == "" {
		return "", false
	}
	prefix := "@" + strings.ToLower(username)
	lower := strings.ToLower(trimmed)
	if lower == prefix {
		return "", false
	}
	if !strings.HasPrefix(lower, prefix+" ") &&
		!strings.HasPrefix(lower, prefix+"\n") &&
		!strings.HasPrefix(lower, prefix+"\t") {
		return "", false
	}
	query := strings.TrimSpace(trimmed[len(prefix):])
	if query == "" {
		return "", false
	}
	return query, true
}

func parseStatusCommand(input string, status model.KnowledgeFactStatus) (parsedCommand, bool) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) != 1 {
		return parsedCommand{help: true}, true
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(fields[0], "#"), 10, 64)
	if err != nil || id <= 0 {
		return parsedCommand{help: true}, true
	}
	return parsedCommand{factID: id, statusUpdate: status}, true
}

func parseTypedCommand(input string) (parsedCommand, bool) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return parsedCommand{help: true}, true
	}
	return parsedCommand{
		factType: fields[0],
		query:    strings.Join(fields[1:], " "),
	}, true
}

func nextOffset(updates []bot.CommandUpdate, current int64) int64 {
	next := current
	for _, update := range updates {
		if update.UpdateID >= next {
			next = update.UpdateID + 1
		}
	}
	return next
}

func botQueryReady(settings model.AppSettings) bool {
	return settings.BotEnabled &&
		strings.TrimSpace(settings.BotToken) != ""
}

func maintenanceResultNeedsConfirmation(result knowledge.MaintenanceResult) bool {
	return result.Action != "" && result.Action != "none" && len(result.MatchedFacts) > 0
}

func maintenanceTargetStatus(action string) model.KnowledgeFactStatus {
	switch action {
	case "expire":
		return model.KnowledgeFactStatusExpired
	case "dismiss":
		return model.KnowledgeFactStatusDismissed
	case "restore":
		return model.KnowledgeFactStatusActive
	default:
		return ""
	}
}

func commandHelpText(language model.Language) string {
	if language == model.LanguageEN {
		return strings.TrimSpace(`## Knowledge Bot Commands
- /knowledge <keyword>: search active facts
- /type <fact_type> <keyword>: search a custom fact type
- /demand <keyword>: search demand facts
- /supply <keyword>: search supply facts
- /who <keyword>: search people and their facts
- /ask <question>: answer from knowledge-base evidence
- /id: show the current Chat ID and your User ID
- /update <natural language>: preview a fact maintenance update
- /expire <fact_id>: mark a fact expired
- /forget <fact_id>: dismiss a fact
- /restore <fact_id>: restore an expired or dismissed fact
- /confirm <code>: apply a pending natural-language update
- /cancel: cancel a pending natural-language update
- /settings: show bot binding status

Examples:
/knowledge gpu
/type skill rust
/ask Who knows Rust?
/update @alice is not a scammer`)
	}
	return strings.TrimSpace(`## 知识 Bot 命令
- /knowledge <关键词>：查询有效事实
- /type <事实类型> <关键词>：查询自定义事实类型
- /demand <关键词>：查询需求事实
- /supply <关键词>：查询供应事实
- /who <关键词>：查询用户及相关事实
- /ask <问题>：基于知识库证据回答问题
- /id：查看当前 Chat ID 和你的 User ID
- /update <自然语言说明>：预览知识事实维护
- /expire <事实ID>：将事实标记为过期
- /forget <事实ID>：忽略一条事实
- /restore <事实ID>：恢复过期或忽略的事实
- /confirm <确认码>：执行待确认的自然语言维护
- /cancel：取消待确认的自然语言维护
- /settings：查看 Bot 绑定状态

示例：
/knowledge 显卡
/type skill rust
/ask 谁了解 Rust？
/update zhang lin 不是风险账号`)
}

func commandStartText(language model.Language) string {
	if language == model.LanguageEN {
		return strings.TrimSpace(`TGTLDR Bot is ready.

Use /help to see commands. In groups, use explicit commands such as /ask <question> or /knowledge <keyword>. This bot only responds in the configured target chat.`)
	}
	return strings.TrimSpace(`TGTLDR Bot 已就绪。

发送 /help 查看命令。在群里建议使用 /ask <问题> 或 /knowledge <关键词> 这类明确命令。为避免泄露本地知识库，Bot 只响应已配置的目标会话。`)
}

func commandUnboundHelpText(language model.Language) string {
	if language == model.LanguageEN {
		return strings.TrimSpace(`TGTLDR Bot is reachable, but this chat is not bound for knowledge queries yet.

Use /id to show this chat's Chat ID and your User ID. The admin can add your User ID to private chat access, or bind the Chat ID as the default target in TGTLDR's Bot page.`)
	}
	return strings.TrimSpace(`TGTLDR Bot 可以收到这条消息，但当前会话还没有授权为知识查询目标。

发送 /id 查看当前 Chat ID 和你的 User ID。管理员可以在 TGTLDR 的 Bot 页面把 User ID 加到“允许私聊用户”，或把 Chat ID 设为默认目标。`)
}

func commandIDText(language model.Language, update bot.CommandUpdate) string {
	chatID := strings.TrimSpace(update.ChatID)
	if chatID == "" {
		chatID = "unknown"
	}
	userID := "unknown"
	if update.FromID != 0 {
		userID = strconv.FormatInt(update.FromID, 10)
	}
	username := strings.TrimSpace(update.FromUsername)
	if username == "" {
		username = "-"
	} else if !strings.HasPrefix(username, "@") {
		username = "@" + username
	}
	if language == model.LanguageEN {
		return fmt.Sprintf("Current Chat ID: %s\nYour User ID: %s\nUsername: %s", chatID, userID, username)
	}
	return fmt.Sprintf("当前 Chat ID：%s\n你的 User ID：%s\n用户名：%s", chatID, userID, username)
}

func (s *Service) commandSettingsText(ctx context.Context, language model.Language) (string, bool, error) {
	if s.store == nil {
		if language == model.LanguageEN {
			return "Bot commands are configured. This bot only responds in the configured target chat.", true, nil
		}
		return "Bot 命令已配置。为避免泄露本地知识库，当前只响应已配置的目标会话。", true, nil
	}
	settings, err := s.store.Settings.Get(ctx)
	if err != nil {
		return "", true, err
	}
	targetChatID := strings.TrimSpace(settings.BotTargetChatID)
	if language == model.LanguageEN {
		if targetChatID == "" {
			return "Bot delivery is enabled, but no target chat is bound yet.", true, nil
		}
		return fmt.Sprintf("Bot is bound to target chat %s. Commands from other chats are ignored.", targetChatID), true, nil
	}
	if targetChatID == "" {
		return "Bot 已启用，但尚未绑定目标会话。", true, nil
	}
	return fmt.Sprintf("Bot 当前绑定目标会话：%s。其他会话里的命令会被忽略。", targetChatID), true, nil
}

func commandStatusUpdatedText(language model.Language, fact model.KnowledgeFact, status model.KnowledgeFactStatus) string {
	if language == model.LanguageEN {
		return fmt.Sprintf("Knowledge fact #%d was marked as %s: %s", fact.ID, status, fact.Title)
	}
	switch status {
	case model.KnowledgeFactStatusActive:
		return fmt.Sprintf("已恢复知识事实 #%d：%s", fact.ID, fact.Title)
	case model.KnowledgeFactStatusExpired:
		return fmt.Sprintf("已将知识事实 #%d 标记为过期：%s", fact.ID, fact.Title)
	default:
		return fmt.Sprintf("已忽略知识事实 #%d：%s", fact.ID, fact.Title)
	}
}

func commandMaintenanceResultText(language model.Language, result knowledge.MaintenanceResult) string {
	if result.Action == "" || result.Action == "none" {
		if language == model.LanguageEN {
			return "I did not find a safe update to preview. Try a natural sentence like `/update @alice is not a scammer`, or search first with `/ask is @alice risky?` and then use `/forget <fact_id>`."
		}
		return "我没找到可安全维护的记录。你可以直接说：`/update zhang lin 不是风险账号`；如果还是找不到，先用 `/ask zhang lin 是风险账号吗` 查到记录，再点 `/forget <事实ID>`。"
	}
	if len(result.UpdatedFacts) == 0 {
		if language == model.LanguageEN {
			return "I understood the update, but no matching fact was found. Try the exact @username/display name, or search first with `/ask <name> risk account`."
		}
		return "我理解这次维护，但没找到匹配记录。请试试更准确的 @username 或显示名；也可以先发 `/ask 名字 是风险账号吗` 查到记录后再 `/forget <事实ID>`。"
	}
	lines := make([]string, 0, len(result.UpdatedFacts)+1)
	if language == model.LanguageEN {
		lines = append(lines, fmt.Sprintf("Updated %d knowledge facts:", len(result.UpdatedFacts)))
		for _, fact := range result.UpdatedFacts {
			lines = append(lines, fmt.Sprintf("- #%d %s (%s)", fact.ID, fact.Title, fact.Status))
		}
		return strings.Join(lines, "\n")
	}
	if strings.EqualFold(result.Action, "correct") {
		lines = append(lines, fmt.Sprintf("已纠正 %d 条知识事实：", len(result.UpdatedFacts)))
	} else {
		lines = append(lines, fmt.Sprintf("已维护 %d 条知识事实：", len(result.UpdatedFacts)))
	}
	for _, fact := range result.UpdatedFacts {
		lines = append(lines, fmt.Sprintf("- #%d %s（%s）", fact.ID, fact.Title, fact.Status))
	}
	return strings.Join(lines, "\n")
}

func commandMaintenancePreviewText(language model.Language, result knowledge.MaintenanceResult, token string) string {
	lines := make([]string, 0, len(result.MatchedFacts)+3)
	if language == model.LanguageEN {
		lines = append(lines, fmt.Sprintf("Pending update: %s %d knowledge facts.", result.Action, len(result.MatchedFacts)))
		for _, fact := range result.MatchedFacts {
			lines = append(lines, fmt.Sprintf("- #%d %s (%s)", fact.ID, fact.Title, fact.Status))
		}
		if strings.EqualFold(result.Action, "correct") && strings.TrimSpace(result.Replacement) != "" {
			lines = append(lines, fmt.Sprintf("Replacement: %s", result.Replacement))
		}
		lines = append(lines, fmt.Sprintf("Send /confirm %s to apply, or /cancel to discard.", token))
		return strings.Join(lines, "\n")
	}
	lines = append(lines, fmt.Sprintf("待确认维护：将对 %d 条知识事实执行 %s。", len(result.MatchedFacts), formatMaintenanceActionZH(result.Action)))
	for _, fact := range result.MatchedFacts {
		lines = append(lines, fmt.Sprintf("- #%d %s（当前 %s）", fact.ID, fact.Title, fact.Status))
	}
	if strings.EqualFold(result.Action, "correct") && strings.TrimSpace(result.Replacement) != "" {
		lines = append(lines, fmt.Sprintf("纠正内容：%s", result.Replacement))
	}
	lines = append(lines, fmt.Sprintf("发送 /confirm %s 执行，或发送 /cancel 取消。", token))
	return strings.Join(lines, "\n")
}

func commandMaintenanceNoPendingText(language model.Language) string {
	if language == model.LanguageEN {
		return "There is no pending knowledge update."
	}
	return "没有待确认的知识维护。"
}

func commandMaintenanceTokenMismatchText(language model.Language) string {
	if language == model.LanguageEN {
		return "Confirmation code does not match the pending knowledge update."
	}
	return "确认码与待确认的知识维护不匹配。"
}

func commandMaintenanceConfirmUsageText(language model.Language, token string) string {
	if language == model.LanguageEN {
		return fmt.Sprintf("Send /confirm %s to apply the pending knowledge update, or /cancel to discard it.", token)
	}
	return fmt.Sprintf("请发送 /confirm %s 执行当前待确认维护，或发送 /cancel 取消。", token)
}

func commandMaintenanceCancelledText(language model.Language) string {
	if language == model.LanguageEN {
		return "Pending knowledge update was cancelled."
	}
	return "已取消待确认的知识维护。"
}

func commandInlineFactRecordedText(language model.Language, facts []model.KnowledgeFact) string {
	lines := make([]string, 0, len(facts)+1)
	if language == model.LanguageEN {
		lines = append(lines, fmt.Sprintf("Recorded %d knowledge fact(s):", len(facts)))
		for _, fact := range facts {
			lines = append(lines, "- "+inlineFactLine(language, fact))
		}
		return strings.Join(lines, "\n")
	}
	lines = append(lines, fmt.Sprintf("已记录 %d 条知识事实：", len(facts)))
	for _, fact := range facts {
		lines = append(lines, "- "+inlineFactLine(language, fact))
	}
	return strings.Join(lines, "\n")
}

func inlineFactLine(language model.Language, fact model.KnowledgeFact) string {
	parts := make([]string, 0, 3)
	if fact.ID > 0 {
		parts = append(parts, fmt.Sprintf("#%d", fact.ID))
	}
	if strings.TrimSpace(fact.FactType) != "" {
		parts = append(parts, strings.TrimSpace(fact.FactType))
	}
	if strings.TrimSpace(fact.ChatTitle) != "" {
		parts = append(parts, strings.TrimSpace(fact.ChatTitle))
	}
	if len(parts) == 0 {
		return strings.TrimSpace(fact.Title)
	}
	if language == model.LanguageEN {
		return fmt.Sprintf("%s (%s)", strings.TrimSpace(fact.Title), strings.Join(parts, ", "))
	}
	return fmt.Sprintf("%s（%s）", strings.TrimSpace(fact.Title), strings.Join(parts, "，"))
}

func commandNaturalQueryEmptyText(language model.Language) string {
	if language == model.LanguageEN {
		return "I could not extract a safe knowledge query. Try /knowledge <keyword> or /type <fact_type> <keyword>."
	}
	return "没有识别到可执行的知识查询。可以改用 /knowledge <关键词> 或 /type <事实类型> <关键词>。"
}

func commandBotIntentAmbiguousText(language model.Language) string {
	if language == model.LanguageEN {
		return "I understood this may affect the knowledge base, but the subject or fact is not clear enough. Use /ask for questions or /update for maintenance."
	}
	return "我判断这可能要操作知识库，但主体或事实不够明确。查询请用 /ask，维护请用 /update。"
}

func commandBlockedUserText(language model.Language) string {
	if language == model.LanguageEN {
		return "You do not have permission to use this bot."
	}
	return "你没有权限使用此 Bot。"
}

func commandFactRecordNeedsBoundChatText(language model.Language) string {
	if language == model.LanguageEN {
		return "This kind of fact can only be recorded from a bound group chat, so I can associate it with the correct knowledge space."
	}
	return "这类事实需要在已绑定的群聊里记录，才能关联到正确的知识空间。"
}

func asyncNaturalQueryQueuedText(language model.Language) string {
	if language == model.LanguageEN {
		return "Received. I am processing the knowledge-base request and will send the result here when it is ready."
	}
	return "收到，正在处理知识库请求，整理好后会在这里回复。"
}

func formatMaintenanceActionZH(action string) string {
	switch action {
	case "expire":
		return "过期"
	case "dismiss":
		return "忽略"
	case "restore":
		return "恢复"
	case "correct":
		return "纠正"
	default:
		return action
	}
}

func commandErrorText(language model.Language, err error) string {
	if language == model.LanguageEN {
		return fmt.Sprintf("Knowledge query failed: %s", err.Error())
	}
	return fmt.Sprintf("知识查询失败：%s", err.Error())
}

func sleep(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
