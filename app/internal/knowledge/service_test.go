package knowledge

import (
	"strings"
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

func TestSplitExtractionMessages(t *testing.T) {
	Convey("知识抽取会按预算切分大批量消息", t, func() {
		base := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
		messages := []model.Message{
			{TelegramMessageID: 1, TextContent: strings.Repeat("a", 1500), MessageTime: base},
			{TelegramMessageID: 2, TextContent: strings.Repeat("b", 1500), MessageTime: base.Add(time.Minute)},
			{TelegramMessageID: 3, TextContent: strings.Repeat("c", 1500), MessageTime: base.Add(2 * time.Minute)},
		}

		chunks := splitExtractionMessagesWithBudget(messages, 500)

		So(chunks, ShouldHaveLength, 3)
		So(chunks[0].Messages[0].TelegramMessageID, ShouldEqual, 1)
		So(chunks[1].Messages[0].TelegramMessageID, ShouldEqual, 2)
		So(chunks[2].Messages[0].TelegramMessageID, ShouldEqual, 3)
	})
}

func TestFlattenKnowledgeFacts(t *testing.T) {
	Convey("合并各 chunk 抽取出的事实", t, func() {
		facts := flattenKnowledgeFacts([][]model.KnowledgeFact{
			{{ID: 1}},
			nil,
			{{ID: 2}, {ID: 3}},
		})

		So(facts, ShouldHaveLength, 3)
		So(facts[0].ID, ShouldEqual, 1)
		So(facts[1].ID, ShouldEqual, 2)
		So(facts[2].ID, ShouldEqual, 3)
	})
}

func TestStatusUpdateFacts(t *testing.T) {
	Convey("status_update 会从普通事实中拆出", t, func() {
		persisted, updates := splitStatusUpdateFacts([]model.KnowledgeFact{
			{FactType: "demand", Title: "需要 Gmail"},
			{FactType: "status_update", Title: "不再需要 Gmail"},
		})

		So(persisted, ShouldHaveLength, 1)
		So(updates, ShouldHaveLength, 1)
		So(updates[0].Title, ShouldEqual, "不再需要 Gmail")
	})

	Convey("状态变更只匹配同一用户、可失效类型和关键词命中的 active 事实", t, func() {
		update := model.KnowledgeFact{
			FactType:        "status_update",
			Title:           "Alice 不再需要 Gmail",
			DataJSON:        `{"target_type":"demand","target_query":"Gmail","action":"no_longer_needed","target_user":"alice"}`,
			SubjectUsername: "alice",
		}
		match := parseStatusUpdateMatch(update)

		So(match.shouldExpire(), ShouldBeTrue)
		So(statusUpdateMatchesCandidate(update, match, model.KnowledgeFact{
			ID:              10,
			FactType:        "demand",
			Title:           "购买 Gmail 邮箱",
			DataJSON:        `{"item":"Gmail"}`,
			SubjectUsername: "alice",
			Status:          model.KnowledgeFactStatusActive,
		}), ShouldBeTrue)
		So(statusUpdateMatchesCandidate(update, match, model.KnowledgeFact{
			ID:              11,
			FactType:        "demand",
			Title:           "购买 Gmail 邮箱",
			DataJSON:        `{"item":"Gmail"}`,
			SubjectUsername: "bob",
			Status:          model.KnowledgeFactStatusActive,
		}), ShouldBeFalse)
		So(statusUpdateMatchesCandidate(update, match, model.KnowledgeFact{
			ID:              12,
			FactType:        "solution",
			Title:           "Gmail 配置教程",
			DataJSON:        `{"topic":"Gmail"}`,
			SubjectUsername: "alice",
			Status:          model.KnowledgeFactStatusActive,
		}), ShouldBeFalse)
	})

	Convey("显式 target_user 存在时不会按维护消息发送者误匹配", t, func() {
		update := model.KnowledgeFact{
			FactType:        "status_update",
			Title:           "Bob 不再需要 Gmail",
			DataJSON:        `{"target_type":"demand","target_query":"Gmail","action":"no_longer_needed","target_user":"bob"}`,
			SubjectSenderID: 1,
			SubjectUsername: "alice",
		}
		match := parseStatusUpdateMatch(update)

		So(statusUpdateMatchesCandidate(update, match, model.KnowledgeFact{
			ID:              20,
			FactType:        "demand",
			Title:           "购买 Gmail 邮箱",
			DataJSON:        `{"item":"Gmail"}`,
			SubjectSenderID: 1,
			SubjectUsername: "alice",
			Status:          model.KnowledgeFactStatusActive,
		}), ShouldBeFalse)
		So(statusUpdateMatchesCandidate(update, match, model.KnowledgeFact{
			ID:              21,
			FactType:        "demand",
			Title:           "购买 Gmail 邮箱",
			DataJSON:        `{"item":"Gmail"}`,
			SubjectSenderID: 2,
			SubjectUsername: "bob",
			Status:          model.KnowledgeFactStatusActive,
		}), ShouldBeTrue)
	})
}
