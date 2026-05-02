package botquery

import (
	"context"
	"fmt"
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
	store *store.Store
	bot   *bot.Service
}

type parsedCommand struct {
	query    string
	factType string
	help     bool
}

type pollState struct {
	token        string
	targetChatID string
	offset       int64
	initialized  bool
}

func NewService(st *store.Store, botService *bot.Service) *Service {
	return &Service{store: st, bot: botService}
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
- /demand <keyword>: search demand facts
- /supply <keyword>: search supply facts
- /who <keyword>: search people and their facts
`)
	}
	return strings.TrimSpace(`
## 知识 Bot 命令
- /knowledge <关键词>：查询有效事实
- /demand <关键词>：查询需求事实
- /supply <关键词>：查询供应事实
- /who <关键词>：查询用户及相关事实
`)
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
