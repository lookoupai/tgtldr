package scheduler

import (
	"testing"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDecideScheduledAction(t *testing.T) {
	deliveredAt := time.Date(2026, time.April, 17, 9, 0, 0, 0, time.UTC)
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	readyAt := time.Date(2026, time.April, 18, 0, 1, 0, 0, shanghai)
	previewAt := time.Date(2026, time.April, 17, 16, 0, 0, 0, shanghai)
	now := time.Date(2026, time.April, 18, 9, 0, 0, 0, time.UTC)
	retryDueAt := now.Add(-time.Minute)
	retryFutureAt := now.Add(time.Minute)
	defaultSettings := model.AppSettings{SummaryRetryLimit: 2}

	tests := []struct {
		name     string
		chat     model.Chat
		summary  model.Summary
		found    bool
		settings model.AppSettings
		expected scheduledAction
	}{
		{
			name:     "不存在摘要时重新生成",
			chat:     model.Chat{DeliveryMode: model.DeliveryModeBot},
			found:    false,
			settings: defaultSettings,
			expected: scheduledActionGenerate,
		},
		{
			name:     "等待中摘要继续生成",
			chat:     model.Chat{DeliveryMode: model.DeliveryModeBot},
			found:    true,
			summary:  model.Summary{Status: model.SummaryStatusPending},
			settings: defaultSettings,
			expected: scheduledActionGenerate,
		},
		{
			name:     "失败摘要没有重试时间时跳过",
			chat:     model.Chat{DeliveryMode: model.DeliveryModeBot},
			found:    true,
			summary:  model.Summary{Status: model.SummaryStatusFailed},
			settings: defaultSettings,
			expected: scheduledActionSkip,
		},
		{
			name:     "失败摘要到达重试时间时执行重试",
			chat:     model.Chat{DeliveryMode: model.DeliveryModeBot},
			found:    true,
			summary:  model.Summary{Status: model.SummaryStatusFailed, RetryCount: 1, NextRetryAt: &retryDueAt},
			settings: defaultSettings,
			expected: scheduledActionRetry,
		},
		{
			name:     "失败摘要未到重试时间时跳过",
			chat:     model.Chat{DeliveryMode: model.DeliveryModeBot},
			found:    true,
			summary:  model.Summary{Status: model.SummaryStatusFailed, RetryCount: 1, NextRetryAt: &retryFutureAt},
			settings: defaultSettings,
			expected: scheduledActionSkip,
		},
		{
			name:     "失败摘要达到重试上限时跳过",
			chat:     model.Chat{DeliveryMode: model.DeliveryModeBot},
			found:    true,
			summary:  model.Summary{Status: model.SummaryStatusFailed, RetryCount: 2, NextRetryAt: &retryDueAt},
			settings: defaultSettings,
			expected: scheduledActionSkip,
		},
		{
			name:     "Bot 模式且摘要完整时只发送",
			chat:     model.Chat{DeliveryMode: model.DeliveryModeBot},
			found:    true,
			summary:  model.Summary{Status: model.SummaryStatusSucceeded, SummaryDate: "2026-04-17", GeneratedAt: readyAt},
			settings: defaultSettings,
			expected: scheduledActionDeliver,
		},
		{
			name:     "Bot 模式但摘要在当天未结束前生成时重跑",
			chat:     model.Chat{DeliveryMode: model.DeliveryModeBot},
			found:    true,
			summary:  model.Summary{Status: model.SummaryStatusSucceeded, SummaryDate: "2026-04-17", GeneratedAt: previewAt},
			settings: defaultSettings,
			expected: scheduledActionGenerate,
		},
		{
			name:  "发送失败后继续重试发送",
			chat:  model.Chat{DeliveryMode: model.DeliveryModeBot},
			found: true,
			summary: model.Summary{
				Status:        model.SummaryStatusSucceeded,
				SummaryDate:   "2026-04-17",
				GeneratedAt:   readyAt,
				DeliveryError: "bot delivery is disabled",
			},
			settings: defaultSettings,
			expected: scheduledActionDeliver,
		},
		{
			name:  "Bot 模式且已发送时跳过",
			chat:  model.Chat{DeliveryMode: model.DeliveryModeBot},
			found: true,
			summary: model.Summary{
				Status:      model.SummaryStatusSucceeded,
				SummaryDate: "2026-04-17",
				GeneratedAt: readyAt,
				DeliveredAt: &deliveredAt,
			},
			settings: defaultSettings,
			expected: scheduledActionSkip,
		},
		{
			name:     "非 Bot 模式直接跳过发送",
			chat:     model.Chat{DeliveryMode: model.DeliveryModeDashboard},
			found:    true,
			summary:  model.Summary{Status: model.SummaryStatusSucceeded, SummaryDate: "2026-04-17", GeneratedAt: readyAt},
			settings: defaultSettings,
			expected: scheduledActionSkip,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			actual := decideScheduledAction(testCase.chat, testCase.summary, testCase.found, "Asia/Shanghai", testCase.settings, now)
			if actual != testCase.expected {
				t.Fatalf("expected action %d, got %d", testCase.expected, actual)
			}
		})
	}
}

func TestSummaryReadyForDelivery(t *testing.T) {
	Convey("生成时间必须晚于摘要日期结束边界", t, func() {
		shanghai, err := time.LoadLocation("Asia/Shanghai")
		So(err, ShouldBeNil)

		So(summaryReadyForDelivery(model.Summary{
			SummaryDate: "2026-04-17",
			GeneratedAt: time.Date(2026, time.April, 17, 23, 59, 59, 0, shanghai),
		}, "Asia/Shanghai"), ShouldBeFalse)

		So(summaryReadyForDelivery(model.Summary{
			SummaryDate: "2026-04-17",
			GeneratedAt: time.Date(2026, time.April, 18, 0, 0, 0, 0, shanghai),
		}, "Asia/Shanghai"), ShouldBeTrue)
	})
}

func TestChannelRunDelivered(t *testing.T) {
	Convey("只有成功且已经记录发送时间的投递频道任务才跳过", t, func() {
		deliveredAt := time.Date(2026, time.April, 17, 9, 0, 0, 0, time.UTC)

		So(channelRunDelivered(model.DeliveryChannelRun{}, false), ShouldBeFalse)
		So(channelRunDelivered(model.DeliveryChannelRun{Status: model.SummaryStatusSucceeded}, true), ShouldBeFalse)
		So(channelRunDelivered(model.DeliveryChannelRun{Status: model.SummaryStatusFailed, DeliveredAt: &deliveredAt}, true), ShouldBeFalse)
		So(channelRunDelivered(model.DeliveryChannelRun{Status: model.SummaryStatusSucceeded, DeliveredAt: &deliveredAt}, true), ShouldBeTrue)
	})
}

func TestChannelTaskKey(t *testing.T) {
	Convey("通道手动任务和定时任务使用同一个并发键", t, func() {
		So(channelTaskKey(7, "2026-05-10"), ShouldEqual, "channel:7:2026-05-10")
	})
}

func TestUniqueChannelSourceChatIDs(t *testing.T) {
	Convey("推送通道知识抽取会忽略无效和重复群组", t, func() {
		So(uniqueChannelSourceChatIDs([]int64{3, 0, 2, 3, -1, 2, 5}), ShouldResemble, []int64{3, 2, 5})
	})
}

func TestAppendChannelExtractionWarnings(t *testing.T) {
	Convey("失败和运行中的通道抽取会追加状态提示", t, func() {
		content := appendChannelExtractionWarnings("## 今日主要结论\n- 正常", []extractionRunReport{
			{
				ChatTitle: "SmsKoc",
				SpaceName: "通用群聊知识库",
				Run: model.KnowledgeRun{
					Status:       model.KnowledgeRunStatusFailed,
					ErrorMessage: "openai status 504: error code: 504",
				},
			},
			{
				ChatTitle: "合集网群",
				SpaceName: "风险账号库",
				Run: model.KnowledgeRun{
					Status: model.KnowledgeRunStatusRunning,
				},
			},
			{
				ChatTitle: "合集网群",
				SpaceName: "供需频道",
				Run: model.KnowledgeRun{
					Status: model.KnowledgeRunStatusSucceeded,
				},
			},
		}, model.SummaryLanguageZhCN)

		So(content, ShouldContainSubstring, "## 抽取状态提示")
		So(content, ShouldContainSubstring, "- SmsKoc / 通用群聊知识库：抽取失败，本次汇总已使用已有情报。错误：openai status 504: error code: 504")
		So(content, ShouldContainSubstring, "- 合集网群 / 风险账号库：抽取仍在运行，本次汇总可能未包含最新情报。")
		So(content, ShouldNotContainSubstring, "供需频道")
	})

	Convey("抽取全部成功时不追加提示", t, func() {
		content := appendChannelExtractionWarnings("正文", []extractionRunReport{
			{Run: model.KnowledgeRun{Status: model.KnowledgeRunStatusSucceeded}},
		}, model.SummaryLanguageZhCN)

		So(content, ShouldEqual, "正文")
	})
}

func TestResolveBotDeliveryTarget(t *testing.T) {
	Convey("群组 Bot Chat ID 优先于全局默认目标", t, func() {
		settings := model.AppSettings{BotTargetChatID: "global-target"}
		chat := model.Chat{BotChatID: "chat-target"}

		So(resolveBotDeliveryTarget(settings, chat), ShouldEqual, "chat-target")
	})

	Convey("群组未配置时回退到全局默认目标", t, func() {
		settings := model.AppSettings{BotTargetChatID: "global-target"}

		So(resolveBotDeliveryTarget(settings, model.Chat{}), ShouldEqual, "global-target")
	})
}

func TestDatesInRange(t *testing.T) {
	Convey("日期范围会包含首尾两天", t, func() {
		So(datesInRange("2026-04-10", "2026-04-12", "Asia/Shanghai"), ShouldResemble, []string{
			"2026-04-10",
			"2026-04-11",
			"2026-04-12",
		})
	})
}

func TestIsRepairableEmptySummary(t *testing.T) {
	Convey("只有成功且消息数和分块数都为零的摘要才会被修复", t, func() {
		So(isRepairableEmptySummary(model.Summary{
			Status:             model.SummaryStatusSucceeded,
			SourceMessageCount: 0,
			ChunkCount:         0,
		}), ShouldBeTrue)

		So(isRepairableEmptySummary(model.Summary{
			Status:             model.SummaryStatusFailed,
			SourceMessageCount: 0,
			ChunkCount:         0,
		}), ShouldBeFalse)

		So(isRepairableEmptySummary(model.Summary{
			Status:             model.SummaryStatusSucceeded,
			SourceMessageCount: 1,
			ChunkCount:         0,
		}), ShouldBeFalse)
	})
}
