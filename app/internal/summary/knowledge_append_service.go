package summary

import (
	"context"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/store"
)

func appendKnowledgeFactsForChats(ctx context.Context, st *store.Store, now time.Time, content string, chatIDs []int64, language model.SummaryOutputLanguage, days int, before time.Time, factTypes []string) (string, error) {
	if st == nil || st.KnowledgeFacts == nil {
		return content, nil
	}
	if err := st.KnowledgeFacts.ExpireDue(ctx, now); err != nil {
		return "", err
	}

	facts := make([]model.KnowledgeFact, 0)
	var since *time.Time
	var beforePtr *time.Time
	if days > 0 {
		if before.IsZero() {
			before = now
		}
		value := before.Add(-time.Duration(days) * 24 * time.Hour)
		since = &value
		beforeValue := before
		beforePtr = &beforeValue
	}
	for _, chatID := range uniqueInt64s(chatIDs) {
		chatFacts, err := st.KnowledgeFacts.ListForSummaryWithFilter(ctx, store.KnowledgeSummaryFilter{
			ChatID:    chatID,
			Now:       now,
			Since:     since,
			Before:    beforePtr,
			FactTypes: factTypes,
		})
		if err != nil {
			return "", err
		}
		facts = append(facts, chatFacts...)
	}
	return appendKnowledgeFacts(content, facts, language), nil
}
