package summary

import (
	"testing"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAppendSourceMessageLinkFallbacks(t *testing.T) {
	Convey("给 tg 用户链接追加源消息链接", t, func() {
		content := "- [Alice](tg://user?id=42)：提供接码"
		got := appendSourceMessageLinkFallbacks(content, map[int64]string{
			42: "https://t.me/example/100",
		}, model.SummaryLanguageZhCN)

		So(got, ShouldEqual, "- [Alice](tg://user?id=42)（[消息](https://t.me/example/100)）：提供接码")
	})

	Convey("不处理没有来源消息的用户链接", t, func() {
		content := "- [Alice](tg://user?id=42)：提供接码"
		got := appendSourceMessageLinkFallbacks(content, map[int64]string{}, model.SummaryLanguageZhCN)

		So(got, ShouldEqual, content)
	})

	Convey("不处理 username 链接", t, func() {
		content := "- [Alice](https://t.me/alice_001)：提供接码"
		got := appendSourceMessageLinkFallbacks(content, map[int64]string{
			42: "https://t.me/example/100",
		}, model.SummaryLanguageZhCN)

		So(got, ShouldEqual, content)
	})
}

func TestMessagePermalink(t *testing.T) {
	Convey("公开群组使用 username 消息链接", t, func() {
		link := messagePermalink(
			model.Chat{Username: "ExampleGroup", TelegramChatID: 12345},
			model.Message{TelegramMessageID: 100},
		)

		So(link, ShouldEqual, "https://t.me/ExampleGroup/100")
	})

	Convey("私有超级群使用 /c/ 消息链接", t, func() {
		link := messagePermalink(
			model.Chat{TelegramChatID: -1001317979642},
			model.Message{TelegramMessageID: 100},
		)

		So(link, ShouldEqual, "https://t.me/c/1317979642/100")
	})
}

func TestCollectSenderSourceMessages(t *testing.T) {
	Convey("同一发送者选择最近消息作为来源链接", t, func() {
		base := time.Date(2026, 5, 27, 9, 0, 0, 0, time.UTC)
		oldMessage := model.Message{
			ChatID:            1,
			TelegramMessageID: 10,
			TelegramSenderID:  42,
			MessageTime:       base,
		}
		newMessage := oldMessage
		newMessage.TelegramMessageID = 11
		newMessage.MessageTime = base.Add(time.Minute)

		selected := collectSenderSourceMessages([]model.Message{oldMessage, newMessage}, nil)

		So(selected[42].TelegramMessageID, ShouldEqual, 11)
	})
}
