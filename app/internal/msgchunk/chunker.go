package msgchunk

import (
	"strings"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
)

type Chunk struct {
	Index    int
	Messages []model.Message
}

func SplitMessages(messages []model.Message, maxTokens int) []Chunk {
	if len(messages) == 0 {
		return nil
	}

	const (
		preferredGap           = 90 * time.Minute
		minGapSplitFillPercent = 0.35
	)
	var chunks []Chunk
	current := make([]model.Message, 0, 64)
	currentTokens := 0
	chunkIndex := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		cloned := make([]model.Message, len(current))
		copy(cloned, current)
		chunks = append(chunks, Chunk{Index: chunkIndex, Messages: cloned})
		chunkIndex++
		current = current[:0]
		currentTokens = 0
	}

	for idx, message := range messages {
		messageTokens := EstimateTokens(message.SummaryText())
		if messageTokens == 0 {
			messageTokens = 10
		}

		if idx > 0 {
			gap := message.MessageTime.Sub(messages[idx-1].MessageTime)
			minGapSplitTokens := int(float64(maxTokens) * minGapSplitFillPercent)
			if gap >= preferredGap && len(current) > 0 && currentTokens >= minGapSplitTokens {
				flush()
			}
		}

		if len(current) > 0 && currentTokens+messageTokens > maxTokens {
			flush()
		}

		current = append(current, message)
		currentTokens += messageTokens
	}

	flush()
	return chunks
}

func EstimateTokens(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	return len([]rune(trimmed))/4 + 16
}
