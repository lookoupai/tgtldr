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

type Command struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

type Self struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

func (s *Service) GetMe(ctx context.Context, token string) (Self, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return Self{}, fmt.Errorf("missing bot token")
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"https://api.telegram.org/bot"+trimmed+"/getMe",
		nil,
	)
	if err != nil {
		return Self{}, fmt.Errorf("build getMe request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return Self{}, fmt.Errorf("get bot identity: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Self{}, fmt.Errorf("read getMe response: %w", err)
	}

	var payload botAPIResponse[Self]
	if err := json.Unmarshal(body, &payload); err != nil {
		if resp.StatusCode >= 300 {
			return Self{}, fmt.Errorf("bot status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return Self{}, fmt.Errorf("decode getMe response: %w", err)
	}
	if resp.StatusCode >= 300 || !payload.OK {
		description := strings.TrimSpace(payload.Description)
		if description == "" {
			description = strings.TrimSpace(string(body))
		}
		return Self{}, fmt.Errorf("bot status %d: %s", resp.StatusCode, description)
	}
	return payload.Result, nil
}

func (s *Service) SetMyCommands(ctx context.Context, token string, commands []Command) error {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return fmt.Errorf("missing bot token")
	}
	if len(commands) == 0 {
		return fmt.Errorf("missing bot commands")
	}

	payload, err := json.Marshal(map[string]any{
		"commands": commands,
	})
	if err != nil {
		return fmt.Errorf("marshal bot commands payload: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://api.telegram.org/bot"+trimmed+"/setMyCommands",
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("build setMyCommands request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("set bot commands: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read setMyCommands response: %w", err)
	}

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		if resp.StatusCode >= 300 {
			return fmt.Errorf("bot status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("decode setMyCommands response: %w", err)
	}
	if resp.StatusCode >= 300 || !result.OK {
		description := strings.TrimSpace(result.Description)
		if description == "" {
			description = strings.TrimSpace(string(body))
		}
		return fmt.Errorf("bot status %d: %s", resp.StatusCode, description)
	}
	return nil
}

func (s *Service) GetMyCommands(ctx context.Context, token string) ([]Command, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return nil, fmt.Errorf("missing bot token")
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"https://api.telegram.org/bot"+trimmed+"/getMyCommands",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build getMyCommands request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get bot commands: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read getMyCommands response: %w", err)
	}

	var payload botAPIResponse[[]Command]
	if err := json.Unmarshal(body, &payload); err != nil {
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("bot status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return nil, fmt.Errorf("decode getMyCommands response: %w", err)
	}
	if resp.StatusCode >= 300 || !payload.OK {
		description := strings.TrimSpace(payload.Description)
		if description == "" {
			description = strings.TrimSpace(string(body))
		}
		return nil, fmt.Errorf("bot status %d: %s", resp.StatusCode, description)
	}
	return payload.Result, nil
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
