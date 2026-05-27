package summary

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/store"
	"github.com/frederic/tgtldr/app/internal/telegramfmt"
)

func appendSourceMessageLinks(ctx context.Context, st *store.Store, content string, messages []model.Message, lookup map[int]model.Message, language model.SummaryOutputLanguage) (string, error) {
	if st == nil || st.Chats == nil || strings.TrimSpace(content) == "" {
		return content, nil
	}
	links, err := collectSourceMessageLinks(ctx, st, messages, lookup)
	if err != nil {
		return "", err
	}
	if len(links) == 0 {
		return content, nil
	}
	return appendSourceMessageLinkFallbacks(content, links, language), nil
}

func appendSourceMessageLinkFallbacks(content string, links map[int64]string, language model.SummaryOutputLanguage) string {
	return summaryUserLinkPattern.ReplaceAllStringFunc(content, func(match string) string {
		groups := summaryUserLinkPattern.FindStringSubmatch(match)
		if len(groups) != 3 {
			return match
		}
		senderID, ok := parseTGUserID(groups[2])
		if !ok {
			return match
		}
		link := strings.TrimSpace(links[senderID])
		if link == "" {
			return match
		}
		return match + sourceMessageLinkSuffix(link, language)
	})
}

func collectSourceMessageLinks(ctx context.Context, st *store.Store, messages []model.Message, lookup map[int]model.Message) (map[int64]string, error) {
	sourceMessages := collectSenderSourceMessages(messages, lookup)
	links := make(map[int64]string, len(sourceMessages))
	chats := make(map[int64]model.Chat)
	for senderID, message := range sourceMessages {
		chat, ok := chats[message.ChatID]
		if !ok {
			var err error
			chat, err = st.Chats.GetByID(ctx, message.ChatID)
			if err != nil {
				return nil, fmt.Errorf("get source chat %d for message link: %w", message.ChatID, err)
			}
			chats[message.ChatID] = chat
		}
		if link := messagePermalink(chat, message); link != "" {
			links[senderID] = link
		}
	}
	return links, nil
}

func collectSenderSourceMessages(messages []model.Message, lookup map[int]model.Message) map[int64]model.Message {
	selected := make(map[int64]model.Message)
	addSenderSourceMessages(selected, messages)
	for _, message := range lookup {
		addSenderSourceMessage(selected, message)
	}
	return selected
}

func addSenderSourceMessages(selected map[int64]model.Message, messages []model.Message) {
	for _, message := range messages {
		addSenderSourceMessage(selected, message)
	}
}

func addSenderSourceMessage(selected map[int64]model.Message, message model.Message) {
	if message.TelegramSenderID <= 0 || message.TelegramMessageID <= 0 || message.ChatID <= 0 {
		return
	}
	current, ok := selected[message.TelegramSenderID]
	if !ok || message.MessageTime.After(current.MessageTime) {
		selected[message.TelegramSenderID] = message
	}
}

func messagePermalink(chat model.Chat, message model.Message) string {
	if username := telegramfmt.Username(chat.Username); username != "" {
		return "https://t.me/" + username + "/" + strconv.Itoa(message.TelegramMessageID)
	}
	if chat.TelegramChatID == 0 {
		return ""
	}
	return "https://t.me/c/" + strconv.FormatInt(internalTelegramChatID(chat.TelegramChatID), 10) + "/" + strconv.Itoa(message.TelegramMessageID)
}

func internalTelegramChatID(chatID int64) int64 {
	if chatID < 0 {
		chatID = -chatID
	}
	const channelPrefix int64 = 1000000000000
	if chatID > channelPrefix {
		return chatID - channelPrefix
	}
	return chatID
}

func parseTGUserID(link string) (int64, bool) {
	const prefix = "tg://user?id="
	if !strings.HasPrefix(link, prefix) {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(link, prefix), 10, 64)
	return id, err == nil && id > 0
}

func sourceMessageLinkSuffix(link string, language model.SummaryOutputLanguage) string {
	label := "消息"
	if model.NormalizeSummaryOutputLanguage(language) == model.SummaryLanguageEN {
		label = "message"
	}
	return "（[" + label + "](" + link + ")）"
}
