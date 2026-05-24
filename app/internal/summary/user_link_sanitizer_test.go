package summary

import (
	"testing"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestSanitizeSummaryUserLinks(t *testing.T) {
	Convey("保留输入里已有的 tg 用户链接", t, func() {
		messages := []model.Message{
			{
				TelegramMessageID: 1,
				TelegramSenderID:  42,
				SenderName:        "Alice",
				MessageTime:       time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC),
				TextContent:       "出售显示器",
			},
		}

		content := "- [Alice](tg://user?id=42) 提供现货"
		got := sanitizeSummaryUserLinks(content, messages, map[int]model.Message{1: messages[0]})

		So(got, ShouldEqual, content)
	})

	Convey("去掉模型臆造的占位 tg 用户链接", t, func() {
		messages := []model.Message{
			{
				TelegramMessageID: 1,
				TelegramSenderID:  42,
				SenderName:        "Alice",
				MessageTime:       time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC),
				TextContent:       "出售显示器",
			},
		}

		content := "- [James Vega](tg://user?id=xxx) 提供 API 号码"
		got := sanitizeSummaryUserLinks(content, messages, map[int]model.Message{1: messages[0]})

		So(got, ShouldEqual, "- James Vega 提供 API 号码")
	})

	Convey("保留输入里已有的用户名链接", t, func() {
		messages := []model.Message{
			{
				TelegramMessageID: 1,
				SenderUsername:    "alice_001",
				SenderName:        "Alice",
				MessageTime:       time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC),
				TextContent:       "需要 RTX 4090",
			},
		}

		content := "- [Alice](https://t.me/alice_001) 需要 RTX 4090"
		got := sanitizeSummaryUserLinks(content, messages, map[int]model.Message{1: messages[0]})

		So(got, ShouldEqual, content)
	})

	Convey("回复引用消息里的用户链接也算合法来源", t, func() {
		reference := model.Message{
			TelegramMessageID: 100,
			TelegramSenderID:  7335028714,
			SenderName:        "OKOTP Support",
			MessageTime:       time.Date(2026, 5, 24, 8, 0, 0, 0, time.UTC),
			TextContent:       "谷歌邮箱授权",
		}
		reply := model.Message{
			TelegramMessageID: 101,
			TelegramSenderID:  9,
			SenderName:        "Bob",
			ReplyToMessageID:  100,
			MessageTime:       time.Date(2026, 5, 24, 8, 1, 0, 0, time.UTC),
			TextContent:       "这个怎么卖",
		}

		content := "- [OKOTP Support](tg://user?id=7335028714) 提供谷歌邮箱授权"
		got := sanitizeSummaryUserLinks(content, []model.Message{reply}, map[int]model.Message{
			100: reference,
			101: reply,
		})

		So(got, ShouldEqual, content)
	})
}
