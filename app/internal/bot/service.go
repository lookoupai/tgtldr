package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
)

type Service struct {
	client *http.Client
}

func New() *Service {
	return &Service{client: &http.Client{Timeout: 20 * time.Second}}
}

func (s *Service) SendMessage(ctx context.Context, token, chatID, text string) error {
	return s.SendMessageWithLanguage(ctx, token, chatID, text, model.LanguageZhCN)
}

func (s *Service) SendMessageWithLanguage(ctx context.Context, token, chatID, text string, language model.Language) error {
	return s.sendMessageParts(ctx, token, chatID, text, language)
}

func (s *Service) SendMessageWithSummaryLanguage(ctx context.Context, token, chatID, text string, language model.SummaryOutputLanguage) error {
	return s.sendMessageParts(ctx, token, chatID, text, language)
}

func (s *Service) sendMessageParts(ctx context.Context, token, chatID, text string, language any) error {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(chatID) == "" {
		return fmt.Errorf("missing bot token or chat id")
	}

	parts := formatTelegramMessages(text, language)
	for index, part := range parts {
		if err := s.sendHTMLMessage(ctx, token, chatID, part); err != nil {
			return fmt.Errorf("send bot message part %d/%d: %w", index+1, len(parts), err)
		}
	}
	return nil
}

func (s *Service) sendHTMLMessage(ctx context.Context, token, chatID, text string) error {
	payload, err := json.Marshal(map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": false,
	})
	if err != nil {
		return fmt.Errorf("marshal bot payload: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://api.telegram.org/bot"+token+"/sendMessage",
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("build bot request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send bot message: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read bot response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("bot status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
