package summary

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/openai"
)

const summaryOpenAIRetryAttempts = 2

type summaryOpenAICallContext struct {
	Kind                 string
	Stage                string
	ChatID               int64
	ChannelID            int64
	SummaryDate          string
	Timezone             string
	Model                string
	BaseURL              string
	RequestMode          model.OpenAIRequestMode
	Temperature          float64
	MaxOutput            int
	Parallelism          int
	ChunkIndex           int
	ChunkCount           int
	SourceMessageCount   int
	ChunkMessageCount    int
	FilteredMessageCount int
	SourceChatIDs        []int64
	ContentFilterEnabled bool
	FilteredFactTypes    []string
	InputRunes           int
}

var summaryOpenAIRetryConfig = openai.RetryConfig{Attempts: summaryOpenAIRetryAttempts}

func chatOpenAIForSummary(ctx context.Context, client openai.ChatClient, req openai.ChatRequest, call summaryOpenAICallContext) (openai.ChatResponse, error) {
	cfg := summaryOpenAIRetryConfig
	if cfg.Attempts <= 0 {
		cfg.Attempts = summaryOpenAIRetryAttempts
	}
	resp, attempts, err := openai.ChatWithRetry(ctx, client, req, cfg)
	if err != nil {
		return openai.ChatResponse{}, summaryOpenAIError(err, req, call, attempts)
	}
	return resp, nil
}

type summaryOpenAIRequestError struct {
	err          error
	context      string
	systemPrompt string
	userPrompt   string
	retryable    bool
}

func (e *summaryOpenAIRequestError) Error() string {
	return fmt.Sprintf("AI summary failed: %s: %v", e.context, e.err)
}

func (e *summaryOpenAIRequestError) Unwrap() error {
	return e.err
}

func summaryOpenAIError(err error, req openai.ChatRequest, call summaryOpenAICallContext, attempts int) error {
	return &summaryOpenAIRequestError{
		err:          err,
		context:      formatSummaryOpenAIErrorContext(call, attempts),
		systemPrompt: req.SystemPrompt,
		userPrompt:   req.UserPrompt,
		retryable:    openai.IsRetryableError(err),
	}
}

func summaryOpenAIErrorContext(err error) string {
	var requestErr *summaryOpenAIRequestError
	if errors.As(err, &requestErr) {
		return requestErr.context
	}
	return ""
}

func summaryOpenAIErrorSystemPrompt(err error) string {
	var requestErr *summaryOpenAIRequestError
	if errors.As(err, &requestErr) {
		return requestErr.systemPrompt
	}
	return ""
}

func summaryOpenAIErrorUserPrompt(err error) string {
	var requestErr *summaryOpenAIRequestError
	if errors.As(err, &requestErr) {
		return requestErr.userPrompt
	}
	return ""
}

func summaryOpenAIErrorRetryable(err error) bool {
	var requestErr *summaryOpenAIRequestError
	if errors.As(err, &requestErr) {
		return requestErr.retryable
	}
	return openai.IsRetryableError(err)
}

func formatSummaryOpenAIErrorContext(call summaryOpenAICallContext, attempts int) string {
	if attempts <= 0 {
		attempts = 1
	}

	fields := []string{
		fmt.Sprintf("kind=%s", valueOr(call.Kind, "summary")),
		fmt.Sprintf("stage=%s", valueOr(call.Stage, "unknown")),
	}
	if call.ChatID != 0 {
		fields = append(fields, fmt.Sprintf("chatId=%d", call.ChatID))
	}
	if call.ChannelID != 0 {
		fields = append(fields, fmt.Sprintf("channelId=%d", call.ChannelID))
	}
	if call.SummaryDate != "" {
		fields = append(fields, fmt.Sprintf("date=%s", call.SummaryDate))
	}
	if call.Timezone != "" {
		fields = append(fields, fmt.Sprintf("timezone=%s", call.Timezone))
	}
	if call.Model != "" {
		fields = append(fields, fmt.Sprintf("model=%s", call.Model))
	}
	if call.RequestMode != "" {
		fields = append(fields, fmt.Sprintf("requestMode=%s", call.RequestMode))
	}
	if call.BaseURL != "" {
		fields = append(fields, fmt.Sprintf("baseURL=%s", call.BaseURL))
	}
	if call.ChunkCount > 0 {
		if call.Stage == "chunk" && call.ChunkIndex >= 0 {
			fields = append(fields, fmt.Sprintf("chunk=%d/%d", call.ChunkIndex+1, call.ChunkCount))
		} else {
			fields = append(fields, fmt.Sprintf("chunks=%d", call.ChunkCount))
		}
	}
	if call.SourceMessageCount > 0 {
		fields = append(fields, fmt.Sprintf("sourceMessages=%d", call.SourceMessageCount))
	}
	if call.ChunkMessageCount > 0 {
		fields = append(fields, fmt.Sprintf("chunkMessages=%d", call.ChunkMessageCount))
	}
	if call.FilteredMessageCount > 0 {
		fields = append(fields, fmt.Sprintf("filteredMessages=%d", call.FilteredMessageCount))
	}
	if len(call.SourceChatIDs) > 0 {
		fields = append(fields, fmt.Sprintf("sourceChats=%s", formatInt64List(call.SourceChatIDs)))
	}
	if call.ContentFilterEnabled {
		fields = append(fields, "contentFilter=true")
	}
	if len(call.FilteredFactTypes) > 0 {
		fields = append(fields, fmt.Sprintf("filteredFactTypes=%s", strings.Join(call.FilteredFactTypes, ",")))
	}
	fields = append(fields,
		fmt.Sprintf("temperature=%.3g", call.Temperature),
		fmt.Sprintf("maxOutput=%d", call.MaxOutput),
		fmt.Sprintf("parallelism=%d", call.Parallelism),
		fmt.Sprintf("inputRunes=%d", call.InputRunes),
		fmt.Sprintf("attempts=%d", attempts),
	)

	return strings.Join(fields, " ")
}

func valueOr(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func formatInt64List(values []int64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprint(value))
	}
	return strings.Join(parts, ",")
}
