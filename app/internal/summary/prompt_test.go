package summary

import (
	"strings"
	"testing"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestBuildStagePrompt(t *testing.T) {
	Convey("默认阶段提示词面向自由讨论群并保留额外要求", t, func() {
		prompt := buildStagePrompt(model.SummaryLanguageZhCN, "群里常说的 ATL 指的是 All Time Low。", "重点关注体验反馈。")

		So(prompt, ShouldContainSubstring, "自由发散讨论")
		So(prompt, ShouldContainSubstring, "群聊背景：\n群里常说的 ATL 指的是 All Time Low。")
		So(prompt, ShouldContainSubstring, "reply_to 和 reply_excerpt")
		So(prompt, ShouldContainSubstring, "## 分话题讨论摘要")
		So(prompt, ShouldContainSubstring, "额外要求：\n重点关注体验反馈。")
		So(strings.Contains(prompt, "待办事项"), ShouldBeFalse)
	})

	Convey("分话题阶段提示词包含用户配置的话题组", t, func() {
		prompt := buildStagePromptForChat(model.SummaryLanguageZhCN, model.Chat{
			SummaryMode: model.SummaryModeChatTopic,
			TopicGroups: []model.TopicGroup{
				{Name: "新闻", Description: "政策、市场、突发事件"},
				{Name: "活动", Description: "会议、线下活动、报名信息"},
			},
		})

		So(prompt, ShouldContainSubstring, "分话题阶段摘要器")
		So(prompt, ShouldContainSubstring, "- 新闻: 政策、市场、突发事件")
		So(prompt, ShouldContainSubstring, "- 活动: 会议、线下活动、报名信息")
		So(prompt, ShouldContainSubstring, "归入“其他”")
	})
}

func TestBuildFinalPrompt(t *testing.T) {
	Convey("默认最终提示词聚焦话题与群体判断", t, func() {
		prompt := buildFinalPrompt(model.SummaryLanguageZhCN, "", "")

		So(prompt, ShouldContainSubstring, "自由讨论群")
		So(prompt, ShouldContainSubstring, "## 今日主要结论")
		So(prompt, ShouldContainSubstring, "## 分话题总结")
		So(prompt, ShouldContainSubstring, "## 仍不确定的信息")
		So(strings.Contains(prompt, "## 待办事项"), ShouldBeFalse)
	})

	Convey("英文最终提示词要求英文输出", t, func() {
		prompt := buildFinalPrompt(model.SummaryLanguageEN, "ATL means All Time Low.", "Keep important links.")

		So(prompt, ShouldContainSubstring, "Write in English")
		So(prompt, ShouldContainSubstring, "## Key Takeaways")
		So(prompt, ShouldContainSubstring, "Group context:\nATL means All Time Low.")
		So(prompt, ShouldContainSubstring, "Additional requirements:\nKeep important links.")
		So(strings.Contains(prompt, "## 今日主要结论"), ShouldBeFalse)
	})

	Convey("俄语、阿拉伯语和自定义语言会写入明确输出语言要求", t, func() {
		So(buildFinalPrompt(model.SummaryLanguageRU, "", ""), ShouldContainSubstring, "Write the entire output in Russian")
		So(buildFinalPrompt(model.SummaryLanguageAR, "", ""), ShouldContainSubstring, "Write the entire output in Arabic")
		So(buildFinalPrompt(model.SummaryOutputLanguage("Japanese"), "", ""), ShouldContainSubstring, "Write the entire output in Japanese")
	})

	Convey("分话题最终提示词要求按话题日报输出", t, func() {
		prompt := buildFinalPromptForChat(model.SummaryLanguageZhCN, model.Chat{
			SummaryMode: model.SummaryModeChatTopic,
			TopicGroups: []model.TopicGroup{
				{Name: "体育", Description: "比赛、转会、赛事讨论"},
			},
		})

		So(prompt, ShouldContainSubstring, "最终分话题日报")
		So(prompt, ShouldContainSubstring, "- 体育: 比赛、转会、赛事讨论")
		So(prompt, ShouldContainSubstring, "## 分话题日报")
	})
}
