package botquery

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
)

type Service struct {
	store              *store.Store
	bot                *bot.Service
	maintainer         knowledgeMaintainer
	pendingMaintenance *pendingMaintenance
	pendingSeq         int64
}

type parsedCommand struct {
	query            string
	factType         string
	start            bool
	help             bool
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
	PreviewMaintenanceText(ctx context.Context, text string) (knowledge.MaintenanceResult, error)
	ParseQueryText(ctx context.Context, text string) (knowledge.KnowledgeQueryInstruction, error)
	UpdateFactStatus(ctx context.Context, factID int64, status model.KnowledgeFactStatus, source string, reason string, operatorText string, matchedQuery string) (model.KnowledgeFact, error)
}

type pendingMaintenance struct {
	token     string
	text      string
	result    knowledge.MaintenanceResult
	expiresAt time.Time
}

type pollState struct {
	token        string
	targetChatID string
	botID        int64
	botUsername  string
	offset       int64
	initialized  bool
}

type commandDefinition struct {
	command       string
	descriptionZH string
	descriptionEN string
}

var commandDefinitions = []commandDefinition{
	{command: "start", descriptionZH: "查看 Bot 说明", descriptionEN: "Show bot introduction"},
	{command: "help", descriptionZH: "查看命令帮助", descriptionEN: "Show command help"},
	{command: "knowledge", descriptionZH: "按关键词查询知识", descriptionEN: "Search knowledge by keyword"},
	{command: "ask", descriptionZH: "用自然语言提问", descriptionEN: "Ask a natural-language question"},
	{command: "demand", descriptionZH: "查询需求事实", descriptionEN: "Search demand facts"},
	{command: "supply", descriptionZH: "查询供应事实", descriptionEN: "Search supply facts"},
	{command: "type", descriptionZH: "按事实类型查询", descriptionEN: "Search by fact type"},
	{command: "update", descriptionZH: "用自然语言维护事实", descriptionEN: "Maintain facts with natural language"},
	{command: "settings", descriptionZH: "查看 Bot 绑定状态", descriptionEN: "Show bot binding status"},
}

func NewService(st *store.Store, botService *bot.Service, maintainer knowledgeMaintainer) *Service {
	return &Service{store: st, bot: botService, maintainer: maintainer}
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
				if !sleep(ctx, commandErrorDelay) {
					return nil
				}
				continue
			}
			state.botID = self.ID
			state.botUsername = strings.TrimSpace(self.Username)
			updates, err := s.bot.GetCommandUpdates(ctx, token, 0, 1)
			if err != nil {
				if !sleep(ctx, commandErrorDelay) {
					return nil
				}
				continue
			}
			state.offset = nextOffset(updates, state.offset)
			state.initialized = true
			continue
		}

		updates, err := s.bot.GetCommandUpdates(ctx, token, state.offset, commandPollTimeoutSeconds)
		if err != nil {
			if !sleep(ctx, commandErrorDelay) {
				return nil
			}
			continue
		}
		for _, update := range updates {
			if update.UpdateID >= state.offset {
				state.offset = update.UpdateID + 1
			}
			if update.ChatID != targetChatID {
				continue
			}
			response, ok, err := s.responseForUpdate(ctx, settings.Language, update, state.botID, state.botUsername)
			if err != nil {
				response = commandErrorText(settings.Language, err)
				ok = true
			}
			if !ok {
				continue
			}
			_ = s.bot.SendMessageWithLanguage(ctx, token, targetChatID, response, settings.Language)
		}
	}
}

func (s *Service) responseForUpdate(ctx context.Context, language model.Language, update bot.CommandUpdate, botID int64, botUsername string) (string, bool, error) {
	text := strings.TrimSpace(update.Text)
	if text == "" {
		return "", false, nil
	}
	if strings.HasPrefix(text, "/") {
		return s.responseForCommand(ctx, language, text)
	}
	if strings.EqualFold(strings.TrimSpace(update.ChatType), "private") {
		return s.respondToNaturalQuery(ctx, language, text)
	}
	if query, ok := extractMentionQuery(text, botUsername); ok {
		return s.respondToNaturalQuery(ctx, language, query)
	}
	if botID != 0 && update.ReplyToBotID == botID {
		return s.respondToNaturalQuery(ctx, language, text)
	}
	return "", false, nil
}

func (s *Service) responseForCommand(ctx context.Context, language model.Language, text string) (string, bool, error) {
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
		return s.respondToNaturalQuery(ctx, language, command.naturalQueryText)
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
	if s.maintainer == nil {
		return "", true, fmt.Errorf("knowledge maintainer is not configured")
	}
	answer, err := s.maintainer.AnswerQueryText(ctx, text, knowledge.KnowledgeAnswerOptions{Limit: commandResultLimit})
	if err == nil {
		if strings.TrimSpace(answer.Query) == "" && strings.TrimSpace(answer.FactType) == "" {
			return commandNaturalQueryEmptyText(language), true, nil
		}
		if strings.TrimSpace(answer.Answer) != "" {
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

func (s *Service) confirmMaintenance(ctx context.Context, language model.Language, token string) (string, bool, error) {
	if s.maintainer == nil {
		return "", true, fmt.Errorf("knowledge maintainer is not configured")
	}
	pending := s.pendingMaintenance
	if pending == nil {
		return commandMaintenanceNoPendingText(language), true, nil
	}
	if strings.TrimSpace(token) != pending.token {
		return commandMaintenanceTokenMismatchText(language), true, nil
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
		strings.TrimSpace(settings.BotToken) != "" &&
		strings.TrimSpace(settings.BotTargetChatID) != ""
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
/ask Who knows Rust?`)
	}
	return strings.TrimSpace(`## 知识 Bot 命令
- /knowledge <关键词>：查询有效事实
- /type <事实类型> <关键词>：查询自定义事实类型
- /demand <关键词>：查询需求事实
- /supply <关键词>：查询供应事实
- /who <关键词>：查询用户及相关事实
- /ask <问题>：基于知识库证据回答问题
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
/ask 谁了解 Rust？`)
}

func commandStartText(language model.Language) string {
	if language == model.LanguageEN {
		return strings.TrimSpace(`TGTLDR Bot is ready.

Use /help to see commands. In groups, use explicit commands such as /ask <question> or /knowledge <keyword>. This bot only responds in the configured target chat.`)
	}
	return strings.TrimSpace(`TGTLDR Bot 已就绪。

发送 /help 查看命令。在群里建议使用 /ask <问题> 或 /knowledge <关键词> 这类明确命令。为避免泄露本地知识库，Bot 只响应已配置的目标会话。`)
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
			return "No safe knowledge update was detected. Please include an affected user and item/topic, or use /expire <fact_id>."
		}
		return "没有识别到可安全执行的知识维护。请同时说明受影响用户和物品/主题，或使用 /expire <事实ID>。"
	}
	if len(result.UpdatedFacts) == 0 {
		if language == model.LanguageEN {
			return "No matching knowledge facts were found. Try a fact ID command such as /expire <fact_id>."
		}
		return "没有找到匹配的知识事实。可以先查询并使用 /expire <事实ID> 这类命令。"
	}
	lines := make([]string, 0, len(result.UpdatedFacts)+1)
	if language == model.LanguageEN {
		lines = append(lines, fmt.Sprintf("Updated %d knowledge facts:", len(result.UpdatedFacts)))
		for _, fact := range result.UpdatedFacts {
			lines = append(lines, fmt.Sprintf("- #%d %s (%s)", fact.ID, fact.Title, fact.Status))
		}
		return strings.Join(lines, "\n")
	}
	lines = append(lines, fmt.Sprintf("已维护 %d 条知识事实：", len(result.UpdatedFacts)))
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
		lines = append(lines, fmt.Sprintf("Send /confirm %s to apply, or /cancel to discard.", token))
		return strings.Join(lines, "\n")
	}
	lines = append(lines, fmt.Sprintf("待确认维护：将对 %d 条知识事实执行 %s。", len(result.MatchedFacts), formatMaintenanceActionZH(result.Action)))
	for _, fact := range result.MatchedFacts {
		lines = append(lines, fmt.Sprintf("- #%d %s（当前 %s）", fact.ID, fact.Title, fact.Status))
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

func commandMaintenanceCancelledText(language model.Language) string {
	if language == model.LanguageEN {
		return "Pending knowledge update was cancelled."
	}
	return "已取消待确认的知识维护。"
}

func commandNaturalQueryEmptyText(language model.Language) string {
	if language == model.LanguageEN {
		return "I could not extract a safe knowledge query. Try /knowledge <keyword> or /type <fact_type> <keyword>."
	}
	return "没有识别到可执行的知识查询。可以改用 /knowledge <关键词> 或 /type <事实类型> <关键词>。"
}

func formatMaintenanceActionZH(action string) string {
	switch action {
	case "expire":
		return "过期"
	case "dismiss":
		return "忽略"
	case "restore":
		return "恢复"
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
