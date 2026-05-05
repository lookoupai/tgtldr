package bot

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestMatchTargetChatCandidates(t *testing.T) {
	t.Parallel()

	updates := []botUpdate{
		{
			UpdateID: 10,
			Message: &botMessage{
				Date: 100,
				From: &botUser{ID: 42},
				Chat: botChat{
					ID:   1001,
					Type: "private",
				},
			},
		},
		{
			UpdateID: 11,
			Message: &botMessage{
				Date: 101,
				From: &botUser{ID: 7},
				Chat: botChat{
					ID:    -1002,
					Type:  "supergroup",
					Title: "别人的群",
				},
			},
		},
		{
			UpdateID: 12,
			Message: &botMessage{
				Date: 110,
				From: &botUser{ID: 42},
				Chat: botChat{
					ID:        -1002,
					Type:      "supergroup",
					Title:     "团队群",
					Username:  "team_group",
					FirstName: "ignored",
				},
			},
		},
		{
			UpdateID: 13,
			Message: &botMessage{
				Date: 109,
				From: &botUser{ID: 42},
				Chat: botChat{
					ID:    -1002,
					Type:  "supergroup",
					Title: "旧团队群",
				},
			},
		},
		{
			UpdateID: 14,
			Message: &botMessage{
				Date: 108,
				From: &botUser{ID: 42},
				Chat: botChat{
					ID:        1003,
					Type:      "private",
					FirstName: "Frederic",
					LastName:  "Zhang",
					Username:  "frederic",
				},
			},
		},
		{
			UpdateID: 15,
			Message:  nil,
		},
	}

	got := matchTargetChatCandidates(updates, 42)
	want := []TargetChatCandidate{
		{
			ChatID:   "-1002",
			ChatType: "supergroup",
			Title:    "团队群",
			Username: "team_group",
		},
		{
			ChatID:   "1003",
			ChatType: "private",
			Title:    "Frederic Zhang",
			Username: "frederic",
		},
		{
			ChatID:   "1001",
			ChatType: "private",
			Title:    "与 Bot 的私聊",
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("matchTargetChatCandidates() mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestMatchTargetChatCandidatesZeroUserID(t *testing.T) {
	t.Parallel()

	got := matchTargetChatCandidates([]botUpdate{{
		UpdateID: 1,
		Message: &botMessage{
			Date: 1,
			From: &botUser{ID: 42},
			Chat: botChat{ID: 1001, Type: "private"},
		},
	}}, 0)

	if len(got) != 0 {
		t.Fatalf("expected no candidates, got %#v", got)
	}
}

func TestGetCommandUpdates(t *testing.T) {
	t.Parallel()

	service := &Service{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/bottest-token/getUpdates" {
			t.Fatalf("request path = %q, want /bottest-token/getUpdates", req.URL.Path)
		}
		query := req.URL.Query()
		if query.Get("offset") != "30" {
			t.Fatalf("offset = %q, want 30", query.Get("offset"))
		}
		if query.Get("timeout") != "3" {
			t.Fatalf("timeout = %q, want 3", query.Get("timeout"))
		}
		if query.Get("allowed_updates") != `["message"]` {
			t.Fatalf("allowed_updates = %q, want [\"message\"]", query.Get("allowed_updates"))
		}
		body := `{
			"ok": true,
			"result": [
				{
					"update_id": 30,
					"message": {
						"message_id": 101,
						"from": {"id": 501, "username": "alice"},
						"chat": {"id": -1001, "type": "supergroup"},
						"text": "  /knowledge gpu  "
					}
				},
				{
					"update_id": 31,
					"message": {
						"message_id": 102,
						"chat": {"id": -1002, "type": "supergroup"},
						"text": "   "
					}
				},
				{"update_id": 32},
				{
					"update_id": 33,
					"message": {
						"message_id": 103,
						"from": {"id": 503, "username": "carol"},
						"chat": {"id": 0, "type": "private"},
						"text": "/help",
						"reply_to_message": {
							"message_id": 99,
							"from": {"id": 777, "is_bot": true, "username": "TgtldrBot"},
							"chat": {"id": 0, "type": "private"}
						}
					}
				}
			]
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})}}

	got, err := service.GetCommandUpdates(context.Background(), "test-token", 30, 3)
	if err != nil {
		t.Fatalf("GetCommandUpdates() error = %v", err)
	}
	want := []CommandUpdate{
		{UpdateID: 30, MessageID: 101, ChatID: "-1001", ChatType: "supergroup", Text: "/knowledge gpu", FromID: 501, FromUsername: "alice"},
		{UpdateID: 31, MessageID: 102, ChatID: "-1002", ChatType: "supergroup"},
		{UpdateID: 32},
		{UpdateID: 33, MessageID: 103, ChatType: "private", Text: "/help", FromID: 503, FromUsername: "carol", ReplyToBotID: 777},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetCommandUpdates() = %#v, want %#v", got, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
