package knowledge

import (
	"fmt"
	"strings"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/telegramfmt"
)

func FormatQueryResult(language model.Language, query string, factType string, facts []model.KnowledgeFact, subjects []model.KnowledgeSubject) string {
	lines := []string{queryTitle(language)}
	if condition := queryCondition(language, query, factType); condition != "" {
		lines = append(lines, "", condition)
	}
	if len(facts) == 0 {
		lines = append(lines, "", queryEmptyFacts(language))
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "", queryFactsHeading(language))
	for _, fact := range facts {
		lines = append(lines, "- "+formatQueryFact(language, fact))
	}

	if len(subjects) > 0 {
		lines = append(lines, "", querySubjectsHeading(language))
		for _, subject := range subjects {
			lines = append(lines, "- "+formatQuerySubject(language, subject))
		}
	}
	return strings.Join(lines, "\n")
}

func queryTitle(language model.Language) string {
	if language == model.LanguageEN {
		return "## Knowledge Query Results"
	}
	return "## 知识查询结果"
}

func queryCondition(language model.Language, query string, factType string) string {
	parts := make([]string, 0, 2)
	if trimmed := strings.TrimSpace(query); trimmed != "" {
		if language == model.LanguageEN {
			parts = append(parts, `keyword "`+trimmed+`"`)
		} else {
			parts = append(parts, "关键词「"+trimmed+"」")
		}
	}
	if trimmed := strings.TrimSpace(factType); trimmed != "" {
		if language == model.LanguageEN {
			parts = append(parts, `type "`+trimmed+`"`)
		} else {
			parts = append(parts, "类型「"+trimmed+"」")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	if language == model.LanguageEN {
		return "Filters: " + strings.Join(parts, ", ")
	}
	return "条件：" + strings.Join(parts, "，")
}

func queryEmptyFacts(language model.Language) string {
	if language == model.LanguageEN {
		return "No matching active knowledge facts were found."
	}
	return "未找到匹配的有效知识事实。"
}

func queryFactsHeading(language model.Language) string {
	if language == model.LanguageEN {
		return "### Matching Facts"
	}
	return "### 匹配事实"
}

func querySubjectsHeading(language model.Language) string {
	if language == model.LanguageEN {
		return "### Related People"
	}
	return "### 相关用户"
}

func formatQueryFact(language model.Language, fact model.KnowledgeFact) string {
	title := compactText(fact.Title)
	if subject := querySubjectName(language, fact.SubjectSenderID, fact.SubjectSenderName, fact.SubjectUsername); subject != "" {
		title = subject + querySeparator(language) + title
	}

	meta := make([]string, 0, 3)
	if fact.ID > 0 {
		meta = append(meta, fmt.Sprintf("#%d", fact.ID))
	}
	if fact.FactType != "" {
		meta = append(meta, fact.FactType)
	}
	if fact.ChatTitle != "" {
		meta = append(meta, fact.ChatTitle)
	}
	if !fact.LastSeenAt.IsZero() {
		meta = append(meta, formatQueryTime(fact.LastSeenAt))
	}
	if len(meta) == 0 {
		return title
	}
	if language == model.LanguageEN {
		return title + " (" + strings.Join(meta, ", ") + ")"
	}
	return title + "（" + strings.Join(meta, "，") + "）"
}

func formatQuerySubject(language model.Language, subject model.KnowledgeSubject) string {
	name := querySubjectName(language, subject.SubjectSenderID, subject.SubjectSenderName, subject.SubjectUsername)
	if name == "" {
		name = compactText(subject.DisplayName)
	}
	if name == "" {
		name = telegramfmt.UnknownUserLabel(language)
	}

	factWord := "条"
	if language == model.LanguageEN {
		if subject.FactCount == 1 {
			factWord = "fact"
		} else {
			factWord = "facts"
		}
	}
	prefix := fmt.Sprintf("%s%s%d %s", name, querySeparator(language), subject.FactCount, factWord)
	if len(subject.FactTypes) == 0 && len(subject.Facts) == 0 {
		return prefix
	}

	parts := make([]string, 0, 2)
	if len(subject.FactTypes) > 0 {
		if language == model.LanguageEN {
			parts = append(parts, "types: "+strings.Join(subject.FactTypes, ", "))
		} else {
			parts = append(parts, "类型："+strings.Join(subject.FactTypes, "、"))
		}
	}
	if titles := subjectFactTitles(subject); titles != "" {
		if language == model.LanguageEN {
			parts = append(parts, "examples: "+titles)
		} else {
			parts = append(parts, "代表："+titles)
		}
	}
	if language == model.LanguageEN {
		return prefix + "; " + strings.Join(parts, "; ")
	}
	return prefix + "；" + strings.Join(parts, "；")
}

func querySubjectName(language model.Language, senderID int64, senderName string, username string) string {
	return telegramfmt.UserReference(language, senderID, senderName, username)
}

func subjectFactTitles(subject model.KnowledgeSubject) string {
	titles := make([]string, 0, 3)
	for _, fact := range subject.Facts {
		if title := compactText(fact.Title); title != "" {
			titles = append(titles, title)
		}
		if len(titles) == 3 {
			break
		}
	}
	return strings.Join(titles, "；")
}

func querySeparator(language model.Language) string {
	if language == model.LanguageEN {
		return ": "
	}
	return "："
}

func compactText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func formatQueryTime(value time.Time) string {
	return value.Format("2006-01-02 15:04")
}
