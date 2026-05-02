package botquery

import (
	"reflect"
	"testing"

	"github.com/frederic/tgtldr/app/internal/bot"
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
