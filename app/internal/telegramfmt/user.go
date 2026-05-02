package telegramfmt

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/frederic/tgtldr/app/internal/model"
)

func UserReference(language model.Language, senderID int64, senderName string, username string) string {
	if normalized := Username(username); normalized != "" {
		return "@" + normalized
	}

	label := compactText(senderName)
	if label == "" && senderID != 0 {
		label = fallbackUserLabel(language, senderID)
	}
	if label == "" {
		return ""
	}
	if senderID > 0 {
		return "[" + markdownLinkLabel(label) + "](tg://user?id=" + strconv.FormatInt(senderID, 10) + ")"
	}
	return label
}

func UnknownUserLabel(language model.Language) string {
	if language == model.LanguageEN {
		return "Unknown user"
	}
	return "未知用户"
}

func Username(username string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(username), "@")
	if len([]rune(trimmed)) < 5 {
		return ""
	}
	for _, r := range trimmed {
		if r == '_' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9') {
			continue
		}
		return ""
	}
	return trimmed
}

func fallbackUserLabel(language model.Language, senderID int64) string {
	if language == model.LanguageEN {
		return fmt.Sprintf("User %d", senderID)
	}
	return fmt.Sprintf("用户 %d", senderID)
}

func markdownLinkLabel(value string) string {
	return strings.NewReplacer("[", "(", "]", ")", "\n", " ", "\r", " ").Replace(value)
}

func compactText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
