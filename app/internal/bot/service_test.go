package bot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestSendMessageWithLanguage(t *testing.T) {
	Convey("超长摘要会按顺序发送多条 Telegram 消息", t, func() {
		transport := &captureTransport{}
		service := &Service{client: &http.Client{Transport: transport}}

		err := service.SendMessageWithLanguage(
			context.Background(),
			"test-token",
			"12345",
			"## 今日主要结论\n\n- "+repeatText("长摘要内容。", 900),
			model.LanguageZhCN,
		)

		So(err, ShouldBeNil)
		So(len(transport.requests), ShouldBeGreaterThan, 1)
		for _, request := range transport.requests {
			So(request.ChatID, ShouldEqual, "12345")
			So(request.ParseMode, ShouldEqual, "HTML")
			So(telegramVisibleLength(request.Text) <= telegramMessageVisibleLimit, ShouldBeTrue)
		}
		So(transport.urls[0], ShouldContainSubstring, "/bottest-token/sendMessage")
	})

	Convey("任一分段发送失败时返回分段位置", t, func() {
		transport := &captureTransport{statusCodes: []int{http.StatusOK, http.StatusBadGateway}}
		service := &Service{client: &http.Client{Transport: transport}}

		err := service.SendMessageWithLanguage(
			context.Background(),
			"test-token",
			"12345",
			"## 今日主要结论\n\n- "+repeatText("长摘要内容。", 900),
			model.LanguageZhCN,
		)

		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "part 2/")
		So(len(transport.requests), ShouldEqual, 2)
	})
}

type capturedBotRequest struct {
	ChatID                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview"`
}

type captureTransport struct {
	statusCodes []int
	requests    []capturedBotRequest
	urls        []string
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var payload capturedBotRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	t.requests = append(t.requests, payload)
	t.urls = append(t.urls, req.URL.String())

	status := http.StatusOK
	if index := len(t.requests) - 1; index < len(t.statusCodes) {
		status = t.statusCodes[index]
	}
	responseBody := `{"ok":true}`
	if status >= 300 {
		responseBody = `{"ok":false,"description":"forced failure"}`
	}
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(responseBody)),
	}, nil
}
