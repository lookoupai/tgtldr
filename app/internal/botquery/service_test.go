package botquery

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/frederic/tgtldr/app/internal/bot"
	"github.com/frederic/tgtldr/app/internal/knowledge"
	"github.com/frederic/tgtldr/app/internal/model"
)

func TestParseCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want parsedCommand
		ok   bool
	}{
		{
			name: "knowledge query",
			text: "/knowledge 4090",
			want: parsedCommand{query: "4090"},
			ok:   true,
		},
		{
			name: "demand query",
			text: "/demand 显卡",
			want: parsedCommand{query: "显卡", factType: "demand"},
			ok:   true,
		},
		{
			name: "supply query with bot username",
			text: "/supply@BotName camera",
			want: parsedCommand{query: "camera", factType: "supply"},
			ok:   true,
		},
		{
			name: "custom type query",
			text: "/type hiring remote engineer",
			want: parsedCommand{query: "remote engineer", factType: "hiring"},
			ok:   true,
		},
		{
			name: "custom type query with bot username",
			text: "/facts@BotName skill rust",
			want: parsedCommand{query: "rust", factType: "skill"},
			ok:   true,
		},
		{
			name: "custom type without query",
			text: "/fact event",
			want: parsedCommand{factType: "event"},
			ok:   true,
		},
		{
			name: "expire fact by id",
			text: "/expire #42",
			want: parsedCommand{factID: 42, statusUpdate: model.KnowledgeFactStatusExpired},
			ok:   true,
		},
		{
			name: "forget fact by id",
			text: "/forget 43",
			want: parsedCommand{factID: 43, statusUpdate: model.KnowledgeFactStatusDismissed},
			ok:   true,
		},
		{
			name: "restore fact by id",
			text: "/restore 44",
			want: parsedCommand{factID: 44, statusUpdate: model.KnowledgeFactStatusActive},
			ok:   true,
		},
		{
			name: "natural language maintenance command",
			text: "/update Alice 不再需要 Gmail 邮箱",
			want: parsedCommand{updateText: "Alice 不再需要 Gmail 邮箱"},
			ok:   true,
		},
		{
			name: "natural language query command",
			text: "/ask 谁了解炒币",
			want: parsedCommand{naturalQueryText: "谁了解炒币"},
			ok:   true,
		},
		{
			name: "confirm maintenance",
			text: "/confirm 000123",
			want: parsedCommand{confirm: true, confirmToken: "000123"},
			ok:   true,
		},
		{
			name: "cancel maintenance",
			text: "/cancel",
			want: parsedCommand{cancel: true},
			ok:   true,
		},
		{
			name: "type command without type shows help",
			text: "/type",
			want: parsedCommand{help: true},
			ok:   true,
		},
		{
			name: "plain text",
			text: "knowledge 4090",
			ok:   false,
		},
		{
			name: "start",
			text: "/start",
			want: parsedCommand{start: true},
			ok:   true,
		},
		{
			name: "help",
			text: "/help",
			want: parsedCommand{help: true},
			ok:   true,
		},
		{
			name: "settings",
			text: "/settings",
			want: parsedCommand{settings: true},
			ok:   true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := parseCommand(tt.text)
			if ok != tt.ok {
				t.Fatalf("parseCommand() ok = %v, want %v", ok, tt.ok)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseCommand() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestBotCommands(t *testing.T) {
	t.Parallel()

	got := BotCommands(model.LanguageZhCN)
	if len(got) == 0 {
		t.Fatal("BotCommands() returned no commands")
	}
	if got[0] != (bot.Command{Command: "start", Description: "查看 Bot 说明"}) {
		t.Fatalf("first command = %#v", got[0])
	}
	if got[len(got)-1].Command != "settings" {
		t.Fatalf("last command = %#v, want settings", got[len(got)-1])
	}
}

func TestCommandsEqual(t *testing.T) {
	t.Parallel()

	commands := []bot.Command{{Command: "help", Description: "查看命令帮助"}}
	if !CommandsEqual(commands, []bot.Command{{Command: "help", Description: "查看命令帮助"}}) {
		t.Fatal("CommandsEqual() should accept identical command lists")
	}
	if CommandsEqual(commands, []bot.Command{{Command: "start", Description: "查看 Bot 说明"}}) {
		t.Fatal("CommandsEqual() should reject different command lists")
	}
}

func TestResponseLanguage(t *testing.T) {
	t.Parallel()

	settings := model.AppSettings{Language: model.LanguageZhCN}
	if got := responseLanguage(settings, responseTarget{}); got != model.LanguageZhCN {
		t.Fatalf("global response language = %q", got)
	}
	if got := responseLanguage(settings, responseTarget{chatID: 1, language: model.SummaryLanguageEN}); got != model.LanguageEN {
		t.Fatalf("chat English response language = %q", got)
	}
	if got := responseLanguage(settings, responseTarget{chatID: 1, language: model.SummaryLanguageZhCN}); got != model.LanguageZhCN {
		t.Fatalf("chat Chinese response language = %q", got)
	}
}

func TestBotQueryReady(t *testing.T) {
	t.Parallel()

	if !botQueryReady(model.AppSettings{BotEnabled: true, BotToken: "token"}) {
		t.Fatal("botQueryReady() should not require a global target chat")
	}
	if botQueryReady(model.AppSettings{BotEnabled: true}) {
		t.Fatal("botQueryReady() should require a token")
	}
}

func TestNextOffset(t *testing.T) {
	t.Parallel()

	got := nextOffset([]bot.CommandUpdate{
		{UpdateID: 41},
		{UpdateID: 39},
		{UpdateID: 43},
	}, 40)

	if got != 44 {
		t.Fatalf("nextOffset() = %d, want 44", got)
	}
}

func TestNextOffsetKeepsCurrentWhenUpdatesAreOlder(t *testing.T) {
	t.Parallel()

	got := nextOffset([]bot.CommandUpdate{
		{UpdateID: 10},
		{UpdateID: 11},
	}, 20)

	if got != 20 {
		t.Fatalf("nextOffset() = %d, want 20", got)
	}
}

func TestMaintenanceTargetStatus(t *testing.T) {
	t.Parallel()

	if got := maintenanceTargetStatus("expire"); got != model.KnowledgeFactStatusExpired {
		t.Fatalf("expire status = %q", got)
	}
	if got := maintenanceTargetStatus("dismiss"); got != model.KnowledgeFactStatusDismissed {
		t.Fatalf("dismiss status = %q", got)
	}
	if got := maintenanceTargetStatus("restore"); got != model.KnowledgeFactStatusActive {
		t.Fatalf("restore status = %q", got)
	}
}

func TestStartCommandShowsIntroduction(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, nil)

	got, ok, err := service.responseForCommand(context.Background(), model.LanguageZhCN, "/start")
	if err != nil {
		t.Fatalf("responseForCommand() error = %v", err)
	}
	if !ok {
		t.Fatal("responseForCommand() ok = false")
	}
	if got == commandHelpText(model.LanguageZhCN) {
		t.Fatal("/start should show introduction instead of raw help text")
	}
	if !strings.Contains(got, "TGTLDR Bot 已就绪") {
		t.Fatalf("responseForCommand() = %q", got)
	}
}

func TestResponseForUpdateNaturalConversation(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, fakeKnowledgeMaintainer{
		answer: knowledge.KnowledgeAnswerResult{
			Query:  "Rust",
			Answer: "可以先看 @alice，依据 #42。",
		},
	})

	tests := []struct {
		name   string
		update bot.CommandUpdate
		wantOK bool
	}{
		{
			name: "private plain text",
			update: bot.CommandUpdate{
				ChatType: "private",
				Text:     "谁了解 Rust",
			},
			wantOK: true,
		},
		{
			name: "group mention",
			update: bot.CommandUpdate{
				ChatType: "supergroup",
				Text:     "@TgtldrBot 谁了解 Rust",
			},
			wantOK: true,
		},
		{
			name: "group reply to bot",
			update: bot.CommandUpdate{
				ChatType:     "supergroup",
				Text:         "谁了解 Rust",
				ReplyToBotID: 777,
			},
			wantOK: true,
		},
		{
			name: "plain group text ignored",
			update: bot.CommandUpdate{
				ChatType: "supergroup",
				Text:     "谁了解 Rust",
			},
			wantOK: false,
		},
		{
			name: "reply to other bot ignored",
			update: bot.CommandUpdate{
				ChatType:     "supergroup",
				Text:         "谁了解 Rust",
				ReplyToBotID: 778,
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok, err := service.responseForUpdate(context.Background(), model.LanguageZhCN, tt.update, 777, "TgtldrBot")
			if err != nil {
				t.Fatalf("responseForUpdate() error = %v", err)
			}
			if ok != tt.wantOK {
				t.Fatalf("responseForUpdate() ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && got != "可以先看 @alice，依据 #42。" {
				t.Fatalf("responseForUpdate() = %q", got)
			}
		})
	}
}

func TestExtractMentionQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
		ok   bool
	}{
		{name: "space", text: "@TgtldrBot 谁了解 Rust", want: "谁了解 Rust", ok: true},
		{name: "newline", text: "@tgtldrbot\n谁了解 Rust", want: "谁了解 Rust", ok: true},
		{name: "middle mention ignored", text: "问问 @TgtldrBot 谁了解 Rust", ok: false},
		{name: "bare mention ignored", text: "@TgtldrBot", ok: false},
		{name: "different mention ignored", text: "@OtherBot 谁了解 Rust", ok: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := extractMentionQuery(tt.text, "TgtldrBot")
			if ok != tt.ok || got != tt.want {
				t.Fatalf("extractMentionQuery() = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestNaturalQueryUsesKnowledgeAnswer(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, fakeKnowledgeMaintainer{
		answer: knowledge.KnowledgeAnswerResult{
			Query:    "Rust",
			FactType: "skill",
			Answer:   "可以先看 @alice，依据 #42。",
		},
	})

	got, ok, err := service.responseForCommand(context.Background(), model.LanguageZhCN, "/ask 谁了解 Rust")
	if err != nil {
		t.Fatalf("responseForCommand() error = %v", err)
	}
	if !ok {
		t.Fatal("responseForCommand() ok = false")
	}
	if got != "可以先看 @alice，依据 #42。" {
		t.Fatalf("responseForCommand() = %q", got)
	}
}

func TestNaturalQueryEmptyAnswerShowsEmptyQueryText(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, fakeKnowledgeMaintainer{})

	got, ok, err := service.responseForCommand(context.Background(), model.LanguageZhCN, "/ask ???")
	if err != nil {
		t.Fatalf("responseForCommand() error = %v", err)
	}
	if !ok {
		t.Fatal("responseForCommand() ok = false")
	}
	if got != commandNaturalQueryEmptyText(model.LanguageZhCN) {
		t.Fatalf("responseForCommand() = %q", got)
	}
}

type fakeKnowledgeMaintainer struct {
	answer knowledge.KnowledgeAnswerResult
}

func (f fakeKnowledgeMaintainer) ApplyMaintenanceText(context.Context, string) (knowledge.MaintenanceResult, error) {
	return knowledge.MaintenanceResult{}, nil
}

func (f fakeKnowledgeMaintainer) AnswerQueryText(context.Context, string, knowledge.KnowledgeAnswerOptions) (knowledge.KnowledgeAnswerResult, error) {
	return f.answer, nil
}

func (f fakeKnowledgeMaintainer) PreviewMaintenanceText(context.Context, string) (knowledge.MaintenanceResult, error) {
	return knowledge.MaintenanceResult{}, nil
}

func (f fakeKnowledgeMaintainer) ParseQueryText(context.Context, string) (knowledge.KnowledgeQueryInstruction, error) {
	return knowledge.KnowledgeQueryInstruction{}, nil
}

func (f fakeKnowledgeMaintainer) UpdateFactStatus(context.Context, int64, model.KnowledgeFactStatus, string, string, string, string) (model.KnowledgeFact, error) {
	return model.KnowledgeFact{}, nil
}
