package summary

import (
	"testing"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAppendAggregatedMessages(t *testing.T) {
	Convey("推送通道聚合复用群组过滤规则", t, func() {
		base := time.Date(2026, 5, 25, 9, 0, 0, 0, time.UTC)
		messages := []model.Message{
			{
				TelegramMessageID: 10,
				TelegramSenderID:  100,
				SenderName:        "每日摘要",
				SenderIsBot:       true,
				TextContent:       "昨天的推送摘要",
				MessageTime:       base,
			},
			{
				TelegramMessageID: 11,
				TelegramSenderID:  200,
				SenderName:        "Alice",
				TextContent:       "今天的新供需信息",
				MessageTime:       base.Add(time.Minute),
			},
		}
		chat := model.Chat{KeepBotMessages: false}
		lookup := make(map[int]model.Message)

		filtered := appendAggregatedMessages(nil, lookup, messages, chat)

		So(len(filtered), ShouldEqual, 1)
		So(filtered[0].TelegramMessageID, ShouldEqual, 11)
		So(len(lookup), ShouldEqual, 2)
	})
}
