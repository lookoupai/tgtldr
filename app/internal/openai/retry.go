package openai

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

var ErrStreamIncomplete = errors.New("openai stream ended before done")

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return "openai status " + strconv.Itoa(e.StatusCode) + ": " + strings.TrimSpace(e.Body)
}

type ChatClient interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

type RetryConfig struct {
	Attempts  int
	BaseDelay time.Duration
	Sleep     func(context.Context, time.Duration) error
}

func ChatWithRetry(ctx context.Context, client ChatClient, req ChatRequest, cfg RetryConfig) (ChatResponse, int, error) {
	attempts := cfg.Attempts
	if attempts <= 0 {
		attempts = 1
	}
	baseDelay := cfg.BaseDelay
	if baseDelay <= 0 {
		baseDelay = time.Second
	}
	sleep := cfg.Sleep
	if sleep == nil {
		sleep = sleepContext
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := client.Chat(ctx, req)
		if err == nil {
			return resp, attempt, nil
		}
		lastErr = err
		if attempt == attempts || !IsRetryableError(err) {
			return ChatResponse{}, attempt, err
		}
		if err := sleep(ctx, time.Duration(attempt)*baseDelay); err != nil {
			return ChatResponse{}, attempt, err
		}
	}
	return ChatResponse{}, attempts, lastErr
}

func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return isRetryableStatus(httpErr.StatusCode)
	}
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, ErrStreamIncomplete) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unexpected eof") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "openai stream ended before done") {
		return true
	}
	return strings.Contains(message, "openai status 429:") ||
		strings.Contains(message, "openai status 500:") ||
		strings.Contains(message, "openai status 502:") ||
		strings.Contains(message, "openai status 503:") ||
		strings.Contains(message, "openai status 504:")
}

func isRetryableStatus(status int) bool {
	switch status {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
