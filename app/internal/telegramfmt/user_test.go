package telegramfmt

import (
	"testing"

	"github.com/frederic/tgtldr/app/internal/model"
)

func TestUserReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		language   model.Language
		senderID   int64
		senderName string
		username   string
		want       string
	}{
		{
			name:       "sender id remains primary with username fallback label",
			language:   model.LanguageZhCN,
			senderID:   42,
			senderName: "Alice",
			username:   "alice_001",
			want:       "[Alice (@alice_001)](tg://user?id=42)",
		},
		{
			name:     "username only",
			language: model.LanguageZhCN,
			username: "alice_001",
			want:     "[@alice_001](https://t.me/alice_001)",
		},
		{
			name:       "sender id with name",
			language:   model.LanguageZhCN,
			senderID:   42,
			senderName: "Alice",
			want:       "[Alice](tg://user?id=42)",
		},
		{
			name:     "sender id fallback",
			language: model.LanguageEN,
			senderID: 42,
			want:     "[User 42](tg://user?id=42)",
		},
		{
			name:       "name only",
			language:   model.LanguageZhCN,
			senderName: "Channel",
			want:       "Channel",
		},
		{
			name:     "empty",
			language: model.LanguageZhCN,
			want:     "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := UserReference(tt.language, tt.senderID, tt.senderName, tt.username)
			if got != tt.want {
				t.Fatalf("UserReference() = %q, want %q", got, tt.want)
			}
		})
	}
}
