package openai

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeChatClient struct {
	responses []ChatResponse
	errs      []error
	calls     int
}

func (c *fakeChatClient) Chat(context.Context, ChatRequest) (ChatResponse, error) {
	index := c.calls
	c.calls++
	if index < len(c.errs) && c.errs[index] != nil {
		return ChatResponse{}, c.errs[index]
	}
	if index < len(c.responses) {
		return c.responses[index], nil
	}
	return ChatResponse{}, nil
}

func TestIsRetryableError(t *testing.T) {
	Convey("限流、网关和临时网络错误可以重试", t, func() {
		So(IsRetryableError(&HTTPError{StatusCode: 429, Body: "rate limit"}), ShouldBeTrue)
		So(IsRetryableError(&HTTPError{StatusCode: 502, Body: "bad gateway"}), ShouldBeTrue)
		So(IsRetryableError(fmt.Errorf("request chat completion: %w", context.DeadlineExceeded)), ShouldBeTrue)
		So(IsRetryableError(ErrStreamIncomplete), ShouldBeTrue)
	})

	Convey("认证、参数和解析类错误不应该盲目重试", t, func() {
		So(IsRetryableError(&HTTPError{StatusCode: 401, Body: "unauthorized"}), ShouldBeFalse)
		So(IsRetryableError(errors.New("decode chat completion: invalid character")), ShouldBeFalse)
		So(IsRetryableError(errors.New("openai returned no choices")), ShouldBeFalse)
	})
}

func TestChatWithRetry(t *testing.T) {
	Convey("临时错误后成功会返回实际尝试次数", t, func() {
		client := &fakeChatClient{
			responses: []ChatResponse{{Content: ""}, {Content: "ok", Model: "gpt-test"}},
			errs:      []error{&HTTPError{StatusCode: 503, Body: "overloaded"}, nil},
		}

		resp, attempts, err := ChatWithRetry(context.Background(), client, ChatRequest{}, RetryConfig{
			Attempts: 2,
			Sleep: func(context.Context, time.Duration) error {
				return nil
			},
		})

		So(err, ShouldBeNil)
		So(resp.Content, ShouldEqual, "ok")
		So(attempts, ShouldEqual, 2)
		So(client.calls, ShouldEqual, 2)
	})

	Convey("非临时错误不会继续重试", t, func() {
		client := &fakeChatClient{
			errs: []error{&HTTPError{StatusCode: 401, Body: "unauthorized"}},
		}

		_, attempts, err := ChatWithRetry(context.Background(), client, ChatRequest{}, RetryConfig{
			Attempts: 3,
			Sleep: func(context.Context, time.Duration) error {
				return nil
			},
		})

		So(err, ShouldNotBeNil)
		So(attempts, ShouldEqual, 1)
		So(client.calls, ShouldEqual, 1)
	})

	Convey("连续临时错误达到上限后返回最后一次错误", t, func() {
		client := &fakeChatClient{
			errs: []error{
				&HTTPError{StatusCode: 502, Body: "first"},
				&HTTPError{StatusCode: 503, Body: "last"},
			},
		}

		_, attempts, err := ChatWithRetry(context.Background(), client, ChatRequest{}, RetryConfig{
			Attempts: 2,
			Sleep: func(context.Context, time.Duration) error {
				return nil
			},
		})

		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "last")
		So(attempts, ShouldEqual, 2)
		So(client.calls, ShouldEqual, 2)
	})
}
