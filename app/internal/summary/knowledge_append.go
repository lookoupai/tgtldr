package summary

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/frederic/tgtldr/app/internal/model"
)

type knowledgeFactGroupKey struct {
	spaceID   int64
	spaceName string
	factType  string
}

type knowledgeFactGroup struct {
	key   knowledgeFactGroupKey
	facts []model.KnowledgeFact
}

func appendKnowledgeFacts(content string, facts []model.KnowledgeFact, language model.Language) string {
	normalized := normalizeKnowledgeFacts(facts)
	if len(normalized) == 0 {
		return content
	}

	section := formatKnowledgeFacts(normalized, language)
	if strings.TrimSpace(content) == "" {
		return section
	}
	return strings.TrimSpace(content) + "\n\n" + section
}

func normalizeKnowledgeFacts(facts []model.KnowledgeFact) []model.KnowledgeFact {
	seen := make(map[string]struct{}, len(facts))
	out := make([]model.KnowledgeFact, 0, len(facts))
	for _, fact := range facts {
		fact.Title = compactKnowledgeText(fact.Title)
		if fact.Title == "" {
			continue
		}
		fact.SpaceName = compactKnowledgeText(fact.SpaceName)
		fact.FactType = compactKnowledgeText(fact.FactType)
		key := strings.Join([]string{
			fmt.Sprint(fact.SpaceID),
			strings.ToLower(fact.SpaceName),
			strings.ToLower(fact.FactType),
			strings.ToLower(fact.Title),
			fmt.Sprint(fact.SubjectSenderID),
			strings.ToLower(strings.TrimPrefix(strings.TrimSpace(fact.SubjectUsername), "@")),
		}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, fact)
	}

	sort.SliceStable(out, func(i, j int) bool {
		left := out[i]
		right := out[j]
		if left.SpaceName != right.SpaceName {
			return strings.ToLower(left.SpaceName) < strings.ToLower(right.SpaceName)
		}
		if left.SpaceID != right.SpaceID {
			return left.SpaceID < right.SpaceID
		}
		if left.FactType != right.FactType {
			return strings.ToLower(left.FactType) < strings.ToLower(right.FactType)
		}
		if left.Confidence != right.Confidence {
			return left.Confidence > right.Confidence
		}
		if !left.LastSeenAt.Equal(right.LastSeenAt) {
			return left.LastSeenAt.After(right.LastSeenAt)
		}
		return left.ID > right.ID
	})
	return out
}

func formatKnowledgeFacts(facts []model.KnowledgeFact, language model.Language) string {
	groups := groupKnowledgeFacts(facts)
	multipleSpaces := hasMultipleKnowledgeSpaces(facts)
	lines := []string{knowledgeFactsSectionTitle(language)}
	for _, group := range groups {
		lines = append(lines, "", "### "+knowledgeFactsGroupTitle(group.key, multipleSpaces, language))
		for _, fact := range group.facts {
			lines = append(lines, "- "+formatKnowledgeFact(fact, language))
		}
	}
	return strings.Join(lines, "\n")
}

func groupKnowledgeFacts(facts []model.KnowledgeFact) []knowledgeFactGroup {
	indexByKey := make(map[knowledgeFactGroupKey]int)
	groups := make([]knowledgeFactGroup, 0)
	for _, fact := range facts {
		key := knowledgeFactGroupKey{
			spaceID:   fact.SpaceID,
			spaceName: fact.SpaceName,
			factType:  fact.FactType,
		}
		index, ok := indexByKey[key]
		if !ok {
			index = len(groups)
			indexByKey[key] = index
			groups = append(groups, knowledgeFactGroup{key: key})
		}
		groups[index].facts = append(groups[index].facts, fact)
	}
	return groups
}

func hasMultipleKnowledgeSpaces(facts []model.KnowledgeFact) bool {
	seen := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		key := fmt.Sprintf("%d:%s", fact.SpaceID, strings.ToLower(fact.SpaceName))
		seen[key] = struct{}{}
		if len(seen) > 1 {
			return true
		}
	}
	return false
}

func formatKnowledgeFact(fact model.KnowledgeFact, language model.Language) string {
	subject := knowledgeFactSubject(fact, language)
	title := compactKnowledgeText(fact.Title)
	if subject != "" {
		title = subject + knowledgeFactSeparator(language) + title
	}
	if confidence := knowledgeFactConfidence(fact.Confidence, language); confidence != "" {
		title += confidence
	}
	return title
}

func knowledgeFactsSectionTitle(language model.Language) string {
	if language == model.LanguageEN {
		return "## Newly Captured Knowledge"
	}
	return "## 今日新增情报"
}

func knowledgeFactsGroupTitle(key knowledgeFactGroupKey, multipleSpaces bool, language model.Language) string {
	factType := key.factType
	if factType == "" {
		factType = knowledgeFactsOtherLabel(language)
	}
	if multipleSpaces && key.spaceName != "" {
		return key.spaceName + " / " + factType
	}
	return factType
}

func knowledgeFactsOtherLabel(language model.Language) string {
	if language == model.LanguageEN {
		return "Other"
	}
	return "其他"
}

func knowledgeFactSubject(fact model.KnowledgeFact, language model.Language) string {
	if username := telegramUsername(fact.SubjectUsername); username != "" {
		return "@" + username
	}
	if name := compactKnowledgeText(fact.SubjectSenderName); name != "" {
		return name
	}
	if fact.SubjectSenderID != 0 {
		if language == model.LanguageEN {
			return fmt.Sprintf("User %d", fact.SubjectSenderID)
		}
		return fmt.Sprintf("用户 %d", fact.SubjectSenderID)
	}
	if language == model.LanguageEN {
		return "Unknown user"
	}
	return "未知用户"
}

func telegramUsername(username string) string {
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

func knowledgeFactSeparator(language model.Language) string {
	if language == model.LanguageEN {
		return ": "
	}
	return "："
}

func knowledgeFactConfidence(confidence float64, language model.Language) string {
	if confidence <= 0 {
		return ""
	}
	if confidence > 1 {
		confidence = 1
	}
	percent := int(math.Round(confidence * 100))
	if language == model.LanguageEN {
		return fmt.Sprintf(" (confidence %d%%)", percent)
	}
	return fmt.Sprintf("（置信度 %d%%）", percent)
}

func compactKnowledgeText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
