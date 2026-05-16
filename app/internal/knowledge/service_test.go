package knowledge

import (
	"errors"
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

func TestDefaultGeneralKnowledgeSpace(t *testing.T) {
	Convey("默认通用知识空间应启用并覆盖状态变更 schema", t, func() {
		space := DefaultGeneralKnowledgeSpace()

		So(space.Name, ShouldEqual, defaultGeneralKnowledgeSpaceName)
		So(space.Enabled, ShouldBeTrue)
		So(space.IncludeInSummary, ShouldBeTrue)
		So(space.ChatIDs, ShouldHaveLength, 0)
		So(space.SchemaJSON, ShouldContainSubstring, `"status_update"`)
		So(space.ExtractPrompt, ShouldContainSubstring, "status_update")
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

func TestIsTransientOpenAIError(t *testing.T) {
	Convey("临时网关和限流错误会触发抽取重试", t, func() {
		So(isTransientOpenAIError(errors.New("openai status 429: rate limit")), ShouldBeTrue)
		So(isTransientOpenAIError(errors.New("openai status 502: error code: 502")), ShouldBeTrue)
		So(isTransientOpenAIError(errors.New("openai status 503: system cpu overloaded")), ShouldBeTrue)
		So(isTransientOpenAIError(errors.New("openai status 504: error code: 504")), ShouldBeTrue)
	})

	Convey("解析错误和认证错误不会盲目重试", t, func() {
		So(isTransientOpenAIError(errors.New("parse extraction response: unexpected end of JSON input")), ShouldBeFalse)
		So(isTransientOpenAIError(errors.New("openai status 401: unauthorized")), ShouldBeFalse)
	})
}

func TestExtractionRunReuse(t *testing.T) {
	Convey("已有成功抽取时直接复用", t, func() {
		now := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)
		run, ok := reusableExtractionRun([]model.KnowledgeRun{
			{ID: 7, Status: model.KnowledgeRunStatusSucceeded, StartedAt: now.Add(-time.Hour)},
		}, now)

		So(ok, ShouldBeTrue)
		So(run.ID, ShouldEqual, 7)
	})

	Convey("未超时的运行中抽取直接复用", t, func() {
		now := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)
		run, ok := reusableExtractionRun([]model.KnowledgeRun{
			{ID: 8, Status: model.KnowledgeRunStatusRunning, StartedAt: now.Add(-5 * time.Minute)},
		}, now)

		So(ok, ShouldBeTrue)
		So(run.ID, ShouldEqual, 8)
	})

	Convey("失败冷却和失败上限会阻止自动重跑", t, func() {
		now := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)

		So(extractionRetryAllowed([]model.KnowledgeRun{
			{Status: model.KnowledgeRunStatusFailed, StartedAt: now.Add(-time.Minute)},
		}, now), ShouldBeFalse)

		So(extractionRetryAllowed([]model.KnowledgeRun{
			{Status: model.KnowledgeRunStatusFailed, StartedAt: now.Add(-time.Hour)},
			{Status: model.KnowledgeRunStatusFailed, StartedAt: now.Add(-2 * time.Hour)},
			{Status: model.KnowledgeRunStatusFailed, StartedAt: now.Add(-3 * time.Hour)},
		}, now), ShouldBeFalse)

		So(extractionRetryAllowed([]model.KnowledgeRun{
			{Status: model.KnowledgeRunStatusFailed, StartedAt: now.Add(-time.Hour)},
		}, now), ShouldBeTrue)
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

func TestMaintenanceInstruction(t *testing.T) {
	Convey("维护指令解析会规整动作、类型和查询", t, func() {
		instruction, err := parseMaintenanceInstruction(`{"action":"resolved","targetType":"need","targetQuery":" Gmail 邮箱 ","targetUser":" @alice ","reason":"已经买到","confidence":0.91}`)

		So(err, ShouldBeNil)
		So(instruction.Action, ShouldEqual, "expire")
		So(instruction.TargetType, ShouldEqual, "demand")
		So(instruction.TargetQuery, ShouldEqual, "Gmail 邮箱")
		So(instruction.TargetUser, ShouldEqual, "@alice")
	})

	Convey("维护匹配要求用户、关键词和可维护类型同时命中", t, func() {
		match := statusUpdateMatch{
			factType:        "demand",
			query:           "Gmail",
			subjectAliases:  compactNormalizedStrings([]string{"alice"}),
			explicitSubject: true,
		}

		So(maintenanceMatchesCandidate(match, model.KnowledgeFact{
			ID:              30,
			FactType:        "demand",
			Title:           "需要 Gmail 邮箱",
			DataJSON:        `{"item":"Gmail"}`,
			SubjectUsername: "alice",
			Status:          model.KnowledgeFactStatusActive,
		}, model.KnowledgeFactStatusActive), ShouldBeTrue)
		So(maintenanceMatchesCandidate(match, model.KnowledgeFact{
			ID:              31,
			FactType:        "demand",
			Title:           "需要 Gmail 邮箱",
			DataJSON:        `{"item":"Gmail"}`,
			SubjectUsername: "alice",
			Status:          model.KnowledgeFactStatusExpired,
		}, model.KnowledgeFactStatusActive), ShouldBeFalse)
	})

	Convey("用户级忽略支持匹配该用户下所有可维护事实", t, func() {
		match := statusUpdateMatch{
			query:           "*",
			subjectAliases:  compactNormalizedStrings([]string{"@KinhRoBot"}),
			explicitSubject: true,
		}

		So(maintenanceMatchesCandidate(match, model.KnowledgeFact{
			ID:              40,
			FactType:        "supply",
			Title:           "供应 Telegram API",
			DataJSON:        `{"item":"Telegram API"}`,
			SubjectUsername: "KinhRoBot",
			Status:          model.KnowledgeFactStatusActive,
		}, model.KnowledgeFactStatusActive), ShouldBeTrue)
		So(maintenanceMatchesCandidate(match, model.KnowledgeFact{
			ID:              41,
			FactType:        "resource",
			Title:           "Telegram API 购买地址",
			DataJSON:        `{"url":"https://example.test"}`,
			SubjectUsername: "KinhRoBot",
			Status:          model.KnowledgeFactStatusActive,
		}, model.KnowledgeFactStatusActive), ShouldBeTrue)
		So(maintenanceMatchesCandidate(match, model.KnowledgeFact{
			ID:              42,
			FactType:        "supply",
			Title:           "供应 Gmail",
			DataJSON:        `{"item":"Gmail"}`,
			SubjectUsername: "alice",
			Status:          model.KnowledgeFactStatusActive,
		}, model.KnowledgeFactStatusActive), ShouldBeFalse)
	})

	Convey("口语化澄清风险账号会解析成忽略风险账号事实", t, func() {
		instruction, ok := parseDirectMaintenanceText("zhang lin 不是风险账号")

		So(ok, ShouldBeTrue)
		So(instruction.Action, ShouldEqual, "dismiss")
		So(instruction.TargetType, ShouldEqual, "risk_account")
		So(instruction.TargetQuery, ShouldEqual, "*")
		So(instruction.TargetUser, ShouldEqual, "zhang lin")
	})

	Convey("风险账号维护支持按用户名匹配", t, func() {
		match := statusUpdateMatch{
			factType:        "risk_account",
			query:           "*",
			subjectAliases:  compactNormalizedStrings([]string{"zhang lin"}),
			explicitSubject: true,
		}

		So(maintenanceMatchesCandidate(match, model.KnowledgeFact{
			ID:                50,
			FactType:          "risk_account",
			Title:             "zhang lin 被误判为风险账号",
			DataJSON:          `{"reported_account_name":"zhang lin"}`,
			SubjectSenderName: "zhang lin",
			Status:            model.KnowledgeFactStatusActive,
		}, model.KnowledgeFactStatusActive), ShouldBeTrue)
	})
}

func TestKnowledgeQueryInstruction(t *testing.T) {
	Convey("自然语言查询指令解析会规整关键词和事实类型", t, func() {
		instruction, err := parseKnowledgeQueryInstruction("```json\n{\"query\":\" 炒币 \",\"factType\":\"skill\"}\n```")

		So(err, ShouldBeNil)
		So(instruction.Query, ShouldEqual, "炒币")
		So(instruction.FactType, ShouldEqual, "skill")
	})

	Convey("自然语言查询类型会复用供需别名规整", t, func() {
		instruction, err := parseKnowledgeQueryInstruction(`{"query":"Gmail","factType":"seller"}`)

		So(err, ShouldBeNil)
		So(instruction.Query, ShouldEqual, "Gmail")
		So(instruction.FactType, ShouldEqual, "supply")
	})

	Convey("自然语言查询会保留风险账号类型", t, func() {
		instruction, err := parseKnowledgeQueryInstruction(`{"query":"alice","factType":"risk_account"}`)

		So(err, ShouldBeNil)
		So(instruction.Query, ShouldEqual, "alice")
		So(instruction.FactType, ShouldEqual, "risk_account")
	})
}

func TestParseExtractionFacts(t *testing.T) {
	Convey("抽取结果可用正文联系人覆盖频道发送者作为事实主体", t, func() {
		now := time.Date(2026, 5, 9, 9, 0, 0, 0, time.UTC)
		message := model.Message{
			TelegramMessageID: 100,
			TelegramSenderID:  10,
			SenderName:        "合集网 - 接码供应需求信息发布",
			SenderUsername:    "hejiwang",
			TextContent:       "投稿人：@real_seller 出售美国 API",
			MessageTime:       now,
		}

		facts, err := parseExtractionFacts(
			`{"facts":[{"type":"supply","title":"美国 API 供应","data":{"item":"美国 API"},"subjectMessageRef":"m001","subjectUsername":"@real_seller","sourceMessageRefs":["m001"],"confidence":0.9}]}`,
			model.KnowledgeSpace{ID: 1, ConfidenceThreshold: 0.75, RetentionDays: 30},
			model.Chat{ID: 2},
			map[string]model.Message{"m001": message},
			now,
		)

		So(err, ShouldBeNil)
		So(facts, ShouldHaveLength, 1)
		So(facts[0].SubjectSenderID, ShouldEqual, 10)
		So(facts[0].SubjectSenderName, ShouldEqual, "合集网 - 接码供应需求信息发布")
		So(facts[0].SubjectUsername, ShouldEqual, "real_seller")
	})

	Convey("抽取结果未覆盖联系人时保留消息发送者 username", t, func() {
		now := time.Date(2026, 5, 9, 9, 0, 0, 0, time.UTC)
		message := model.Message{
			TelegramMessageID: 101,
			TelegramSenderID:  11,
			SenderName:        "Alice",
			SenderUsername:    "alice_api",
			TextContent:       "出售美国 API",
			MessageTime:       now,
		}

		facts, err := parseExtractionFacts(
			`{"facts":[{"type":"supply","title":"美国 API 供应","data":{"item":"美国 API"},"subjectMessageRef":"m001","sourceMessageRefs":["m001"],"confidence":0.9}]}`,
			model.KnowledgeSpace{ID: 1, ConfidenceThreshold: 0.75, RetentionDays: 30},
			model.Chat{ID: 2},
			map[string]model.Message{"m001": message},
			now,
		)

		So(err, ShouldBeNil)
		So(facts, ShouldHaveLength, 1)
		So(facts[0].SubjectSenderName, ShouldEqual, "Alice")
		So(facts[0].SubjectUsername, ShouldEqual, "alice_api")
	})

	Convey("普通敏感发言不会被兜底保存为风险账号", t, func() {
		now := time.Date(2026, 5, 9, 9, 0, 0, 0, time.UTC)
		message := model.Message{
			TelegramMessageID: 102,
			TelegramSenderID:  12,
			SenderName:        "Alice",
			SenderUsername:    "alice",
			TextContent:       "出售成人博彩广告资源，价格私聊",
			MessageTime:       now,
		}

		facts, err := parseExtractionFacts(
			`{"facts":[{"type":"risk_account","title":"Alice 敏感业务","data":{"reported_account_username":"alice","risk_type":"敏感交易","allegation":"发布成人博彩广告资源","evidence":"出售成人博彩广告资源，价格私聊","status":"reported"},"subjectMessageRef":"m001","sourceMessageRefs":["m001"],"confidence":0.95}]}`,
			model.KnowledgeSpace{ID: 1, ConfidenceThreshold: 0.75, RetentionDays: 30},
			model.Chat{ID: 2},
			map[string]model.Message{"m001": message},
			now,
		)

		So(err, ShouldBeNil)
		So(facts, ShouldHaveLength, 0)
	})

	Convey("明确曝光诈骗账号会保存为风险账号", t, func() {
		now := time.Date(2026, 5, 9, 9, 0, 0, 0, time.UTC)
		message := model.Message{
			TelegramMessageID: 103,
			TelegramSenderID:  13,
			SenderName:        "Bob",
			SenderUsername:    "bob",
			TextContent:       "曝光 @alice 是骗子，收款不发货后拉黑我",
			MessageTime:       now,
		}

		facts, err := parseExtractionFacts(
			`{"facts":[{"type":"risk_account","title":"@alice 被曝光收款不发货","data":{"reported_account_username":"alice","reporter":"Bob","risk_type":"诈骗","allegation":"收款不发货后拉黑","evidence":"曝光 @alice 是骗子，收款不发货后拉黑我","status":"reported"},"subjectMessageRef":"m001","sourceMessageRefs":["m001"],"confidence":0.95}]}`,
			model.KnowledgeSpace{ID: 1, ConfidenceThreshold: 0.75, RetentionDays: 30},
			model.Chat{ID: 2},
			map[string]model.Message{"m001": message},
			now,
		)

		So(err, ShouldBeNil)
		So(facts, ShouldHaveLength, 1)
		So(facts[0].FactType, ShouldEqual, "risk_account")
		So(facts[0].SubjectSenderName, ShouldEqual, "Bob")
	})
}
