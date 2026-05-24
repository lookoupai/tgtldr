package summary

import (
	"regexp"
	"strconv"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/telegramfmt"
)

var summaryUserLinkPattern = regexp.MustCompile(`\[(.*?)\]\((tg://user\?id=[^)]+|https://t\.me/[^\s)]+)\)`)

func sanitizeSummaryUserLinks(content string, messages []model.Message, lookup map[int]model.Message) string {
	allowed := collectAllowedSummaryUserLinks(messages, lookup)
	if len(allowed) == 0 {
		return summaryUserLinkPattern.ReplaceAllStringFunc(content, stripDisallowedSummaryUserLink)
	}
	return summaryUserLinkPattern.ReplaceAllStringFunc(content, func(match string) string {
		groups := summaryUserLinkPattern.FindStringSubmatch(match)
		if len(groups) != 3 {
			return match
		}
		if _, ok := allowed[groups[2]]; ok {
			return match
		}
		return groups[1]
	})
}

func collectAllowedSummaryUserLinks(messages []model.Message, lookup map[int]model.Message) map[string]struct{} {
	allowed := make(map[string]struct{}, len(messages)+len(lookup))
	addAllowedSummaryUserLinks(allowed, messages)
	for _, message := range lookup {
		addAllowedSummaryUserLink(allowed, message)
	}
	return allowed
}

func addAllowedSummaryUserLinks(allowed map[string]struct{}, messages []model.Message) {
	for _, message := range messages {
		addAllowedSummaryUserLink(allowed, message)
	}
}

func addAllowedSummaryUserLink(allowed map[string]struct{}, message model.Message) {
	if message.TelegramSenderID > 0 {
		allowed["tg://user?id="+strconv.FormatInt(message.TelegramSenderID, 10)] = struct{}{}
		return
	}
	if username := telegramfmt.Username(message.SenderUsername); username != "" {
		allowed["https://t.me/"+username] = struct{}{}
	}
}

func stripDisallowedSummaryUserLink(match string) string {
	groups := summaryUserLinkPattern.FindStringSubmatch(match)
	if len(groups) != 3 {
		return match
	}
	return groups[1]
}
