package bot

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
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

func TestSetMyCommands(t *testing.T) {
	Convey("会向 Telegram 注册 Bot 命令菜单", t, func() {
		transport := &commandCaptureTransport{}
		service := &Service{client: &http.Client{Transport: transport}}

		err := service.SetMyCommands(context.Background(), "test-token", []Command{
			{Command: "help", Description: "查看命令帮助"},
			{Command: "knowledge", Description: "按关键词查询知识"},
		})

		So(err, ShouldBeNil)
		So(transport.path, ShouldEqual, "/bottest-token/setMyCommands")
		So(transport.commands, ShouldResemble, []Command{
			{Command: "help", Description: "查看命令帮助"},
			{Command: "knowledge", Description: "按关键词查询知识"},
		})
	})

	Convey("注册失败时返回 Telegram 错误描述", t, func() {
		transport := &commandCaptureTransport{statusCode: http.StatusBadRequest}
		service := &Service{client: &http.Client{Transport: transport}}

		err := service.SetMyCommands(context.Background(), "test-token", []Command{
			{Command: "help", Description: "查看命令帮助"},
		})

		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "bad command list")
	})
}

func TestGetMe(t *testing.T) {
	service := &Service{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/bottest-token/getMe" {
			t.Fatalf("request path = %q, want /bottest-token/getMe", req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":{"id":777,"username":"TgtldrBot"}}`)),
		}, nil
	})}}

	got, err := service.GetMe(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	want := Self{ID: 777, Username: "TgtldrBot"}
	if got != want {
		t.Fatalf("GetMe() = %#v, want %#v", got, want)
	}
}

func TestGetMyCommands(t *testing.T) {
	service := &Service{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/bottest-token/getMyCommands" {
			t.Fatalf("request path = %q, want /bottest-token/getMyCommands", req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":[{"command":"help","description":"查看命令帮助"}]}`)),
		}, nil
	})}}

	got, err := service.GetMyCommands(context.Background(), "test-token")
	if err != nil {
		t.Fatalf("GetMyCommands() error = %v", err)
	}
	want := []Command{{Command: "help", Description: "查看命令帮助"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetMyCommands() = %#v, want %#v", got, want)
	}
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

type capturedCommandRequest struct {
	Commands []Command `json:"commands"`
}

type commandCaptureTransport struct {
	statusCode int
	path       string
	commands   []Command
}

func (t *commandCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var payload capturedCommandRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	t.path = req.URL.Path
	t.commands = payload.Commands

	status := t.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	responseBody := `{"ok":true,"result":true}`
	if status >= 300 {
		responseBody = `{"ok":false,"description":"bad command list"}`
	}
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(responseBody)),
	}, nil
}
