package knowledge

import (
	"testing"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestSummaryExtractionSpaces(t *testing.T) {
	Convey("摘要前只自动抽取启用且允许并入摘要的知识空间", t, func() {
		spaces := []model.KnowledgeSpace{
			{ID: 1, Enabled: true, IncludeInSummary: true},
			{ID: 2, Enabled: false, IncludeInSummary: true},
			{ID: 3, Enabled: true, IncludeInSummary: false},
			{ID: 4, Enabled: true, IncludeInSummary: true, ChatIDs: []int64{99}},
			{ID: 5, Enabled: true, IncludeInSummary: true, ChatIDs: []int64{42}},
		}

		selected := summaryExtractionSpaces(spaces, 42)

		So(selected, ShouldHaveLength, 2)
		So(selected[0].ID, ShouldEqual, 1)
		So(selected[1].ID, ShouldEqual, 5)
	})
}

func TestFilterMessages(t *testing.T) {
	Convey("知识抽取复用群组过滤规则", t, func() {
		base := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
		messages := []model.Message{
			{
				TelegramMessageID: 1,
				SenderName:        "验证机器人",
				SenderUsername:    "verify_bot",
				SenderIsBot:       true,
				TextContent:       "请完成验证",
				MessageTime:       base,
			},
			{
				TelegramMessageID: 2,
				SenderName:        "Alice",
				SenderUsername:    "alice",
				TextContent:       "需要显卡",
				MessageTime:       base.Add(time.Minute),
			},
			{
				TelegramMessageID: 3,
				SenderName:        "Bob",
				SenderUsername:    "bob",
				TextContent:       "出售显示器",
				MessageTime:       base.Add(2 * time.Minute),
			},
			{
				TelegramMessageID: 4,
				SenderName:        "Carol",
				SenderUsername:    "carol",
				TextContent:       "验证码 1234",
				MessageTime:       base.Add(3 * time.Minute),
			},
		}

		filtered := filterMessages(messages, model.Chat{
			KeepBotMessages:  false,
			FilteredSenders:  []string{"@alice"},
			FilteredKeywords: []string{"验证码"},
		})

		So(filtered, ShouldHaveLength, 1)
		So(filtered[0].TelegramMessageID, ShouldEqual, 3)
	})
}
