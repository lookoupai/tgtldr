package summary

import (
	"context"
	"testing"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/frederic/tgtldr/app/internal/openai"
	. "github.com/smartystreets/goconvey/convey"
)

type fakeSummaryChatClient struct {
	responses []openai.ChatResponse
	errs      []error
	calls     int
}

func (c *fakeSummaryChatClient) Chat(context.Context, openai.ChatRequest) (openai.ChatResponse, error) {
	index := c.calls
	c.calls++
	if index < len(c.errs) && c.errs[index] != nil {
		return openai.ChatResponse{}, c.errs[index]
	}
	if index < len(c.responses) {
		return c.responses[index], nil
	}
	return openai.ChatResponse{}, nil
}

func TestChatOpenAIForSummary(t *testing.T) {
	original := summaryOpenAIRetryConfig
	defer func() {
		summaryOpenAIRetryConfig = original
	}()
	summaryOpenAIRetryConfig = openai.RetryConfig{
		Attempts: 2,
		Sleep: func(context.Context, time.Duration) error {
			return nil
		},
	}

	Convey("摘要 OpenAI 调用会重试临时错误并返回成功结果", t, func() {
		client := &fakeSummaryChatClient{
			responses: []openai.ChatResponse{{}, {Content: "ok", Model: "gpt-test"}},
			errs:      []error{&openai.HTTPError{StatusCode: 502, Body: "bad gateway"}, nil},
		}

		resp, err := chatOpenAIForSummary(context.Background(), client, openai.ChatRequest{}, summaryOpenAICallContext{
			Kind:        "summary",
			Stage:       "final",
			Model:       "gpt-test",
			RequestMode: model.OpenAIRequestModeStream,
		})

		So(err, ShouldBeNil)
		So(resp.Content, ShouldEqual, "ok")
		So(client.calls, ShouldEqual, 2)
	})

	Convey("摘要 OpenAI 失败会包含阶段、模型、chunk 和原始错误", t, func() {
		client := &fakeSummaryChatClient{
			errs: []error{
				&openai.HTTPError{StatusCode: 502, Body: "first"},
				&openai.HTTPError{StatusCode: 503, Body: "last"},
			},
		}

		_, err := chatOpenAIForSummary(context.Background(), client, openai.ChatRequest{
			SystemPrompt: "system prompt",
			UserPrompt:   "user prompt",
		}, summaryOpenAICallContext{
			Kind:               "summary",
			Stage:              "chunk",
			ChatID:             42,
			SummaryDate:        "2026-05-23",
			Timezone:           "Asia/Shanghai",
			Model:              "gpt-test",
			BaseURL:            "https://proxy.example/v1",
			RequestMode:        model.OpenAIRequestModeStream,
			MaxOutput:          1200,
			Parallelism:        2,
			ChunkIndex:         1,
			ChunkCount:         3,
			SourceMessageCount: 99,
			ChunkMessageCount:  40,
			InputRunes:         2048,
		})

		So(err, ShouldNotBeNil)
		message := err.Error()
		So(message, ShouldContainSubstring, "AI summary failed")
		So(message, ShouldContainSubstring, "stage=chunk")
		So(message, ShouldContainSubstring, "chatId=42")
		So(message, ShouldContainSubstring, "date=2026-05-23")
		So(message, ShouldContainSubstring, "model=gpt-test")
		So(message, ShouldContainSubstring, "requestMode=stream")
		So(message, ShouldContainSubstring, "chunk=2/3")
		So(message, ShouldContainSubstring, "sourceMessages=99")
		So(message, ShouldContainSubstring, "chunkMessages=40")
		So(message, ShouldContainSubstring, "attempts=2")
		So(message, ShouldContainSubstring, "openai status 503")
		So(message, ShouldContainSubstring, "last")
		So(summaryOpenAIErrorContext(err), ShouldContainSubstring, "stage=chunk")
		So(summaryOpenAIErrorSystemPrompt(err), ShouldEqual, "system prompt")
		So(summaryOpenAIErrorUserPrompt(err), ShouldEqual, "user prompt")
		So(summaryOpenAIErrorRetryable(err), ShouldBeTrue)
	})

	Convey("聚合摘要 final 失败会包含 channel 上下文", t, func() {
		client := &fakeSummaryChatClient{
			errs: []error{&openai.HTTPError{StatusCode: 401, Body: "unauthorized"}},
		}

		_, err := chatOpenAIForSummary(context.Background(), client, openai.ChatRequest{}, summaryOpenAICallContext{
			Kind:                 "aggregated_summary",
			Stage:                "final",
			ChannelID:            7,
			SummaryDate:          "2026-05-23",
			Model:                "gpt-test",
			RequestMode:          model.OpenAIRequestModeNonStream,
			ChunkCount:           4,
			FilteredMessageCount: 120,
			SourceChatIDs:        []int64{1, 2},
			ContentFilterEnabled: true,
			FilteredFactTypes:    []string{"risk", "supply"},
		})

		So(err, ShouldNotBeNil)
		message := err.Error()
		So(message, ShouldContainSubstring, "kind=aggregated_summary")
		So(message, ShouldContainSubstring, "stage=final")
		So(message, ShouldContainSubstring, "channelId=7")
		So(message, ShouldContainSubstring, "chunks=4")
		So(message, ShouldContainSubstring, "filteredMessages=120")
		So(message, ShouldContainSubstring, "sourceChats=1,2")
		So(message, ShouldContainSubstring, "contentFilter=true")
		So(message, ShouldContainSubstring, "filteredFactTypes=risk,supply")
		So(client.calls, ShouldEqual, 1)
	})
}
