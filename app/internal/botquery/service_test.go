package botquery

import (
	"context"
	"reflect"
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
			name: "help",
			text: "/help",
			want: parsedCommand{help: true},
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
