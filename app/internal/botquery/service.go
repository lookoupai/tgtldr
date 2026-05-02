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
)

type Service struct {
	store      *store.Store
	bot        *bot.Service
	maintainer knowledgeMaintainer
}

type parsedCommand struct {
	query        string
	factType     string
	help         bool
	factID       int64
	statusUpdate model.KnowledgeFactStatus
	updateText   string
}

type knowledgeMaintainer interface {
	ApplyMaintenanceText(ctx context.Context, text string) (knowledge.MaintenanceResult, error)
	UpdateFactStatus(ctx context.Context, factID int64, status model.KnowledgeFactStatus, source string, reason string, operatorText string, matchedQuery string) (model.KnowledgeFact, error)
}

type pollState struct {
	token        string
	targetChatID string
	offset       int64
	initialized  bool
}

func NewService(st *store.Store, botService *bot.Service, maintainer knowledgeMaintainer) *Service {
	return &Service{store: st, bot: botService, maintainer: maintainer}
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
			response, ok, err := s.responseForCommand(ctx, settings.Language, update.Text)
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

func (s *Service) responseForCommand(ctx context.Context, language model.Language, text string) (string, bool, error) {
	command, ok := parseCommand(text)
	if !ok {
		return "", false, nil
	}
	if command.help {
		return commandHelpText(language), true, nil
	}
	if command.statusUpdate != "" {
		return s.updateFactStatus(ctx, language, command.factID, command.statusUpdate)
	}
	if command.updateText != "" {
		return s.applyMaintenanceText(ctx, language, command.updateText)
	}

	now := time.Now()
	if err := s.store.KnowledgeFacts.ExpireDue(ctx, now); err != nil {
		return "", true, err
	}
	facts, err := s.store.KnowledgeFacts.List(ctx, store.KnowledgeFactFilter{
		Status:   model.KnowledgeFactStatusActive,
		FactType: command.factType,
		Query:    command.query,
		Limit:    commandResultLimit,
	})
	if err != nil {
		return "", true, err
	}
	subjects, err := s.store.KnowledgeFacts.ListSubjects(ctx, store.KnowledgeSubjectFilter{
		FactType: command.factType,
		Query:    command.query,
		Limit:    commandResultLimit,
	})
	if err != nil {
		return "", true, err
	}
	return knowledge.FormatQueryResult(language, command.query, command.factType, facts, subjects), true, nil
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
	result, err := s.maintainer.ApplyMaintenanceText(ctx, text)
	if err != nil {
		return "", true, err
	}
	return commandMaintenanceResultText(language, result), true, nil
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
	case "start", "help":
		return parsedCommand{help: true}, true
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

func commandHelpText(language model.Language) string {
	if language == model.LanguageEN {
		return strings.TrimSpace(`
## Knowledge Bot Commands
- /knowledge <keyword>: search active facts
- /type <fact_type> <keyword>: search a custom fact type
- /fact <fact_type> <keyword>: alias of /type
- /facts <fact_type> <keyword>: alias of /type
- /demand <keyword>: search demand facts
- /supply <keyword>: search supply facts
- /who <keyword>: search people and their facts
- /expire <fact_id>: mark a fact expired
- /forget <fact_id>: dismiss a fact
- /restore <fact_id>: restore an expired or dismissed fact
- /update <natural language>: update facts from a maintenance note, such as "Alice no longer needs Gmail"
`)
	}
	return strings.TrimSpace(`
## 知识 Bot 命令
- /knowledge <关键词>：查询有效事实
- /type <事实类型> <关键词>：查询自定义事实类型
- /fact <事实类型> <关键词>：/type 的别名
- /facts <事实类型> <关键词>：/type 的别名
- /demand <关键词>：查询需求事实
- /supply <关键词>：查询供应事实
- /who <关键词>：查询用户及相关事实
- /expire <事实ID>：将事实标记为过期
- /forget <事实ID>：忽略一条事实
- /restore <事实ID>：恢复过期或忽略的事实
- /update <自然语言说明>：根据说明维护事实，例如“A 不再需要 Gmail 邮箱”
`)
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
