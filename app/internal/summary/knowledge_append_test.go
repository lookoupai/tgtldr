package summary

import (
	"testing"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAppendKnowledgeFacts(t *testing.T) {
	Convey("没有事实时不改变摘要正文", t, func() {
		content := "## 今日主要结论\n- 暂无"

		So(appendKnowledgeFacts(content, nil, model.LanguageZhCN), ShouldEqual, content)
	})

	Convey("中文摘要会按类型追加可点击用户名和置信度", t, func() {
		content := "## 今日主要结论\n- 有供需信息"
		facts := []model.KnowledgeFact{
			{
				SpaceID:           1,
				SpaceName:         "供需频道",
				FactType:          "需求",
				Title:             "需要 RTX 4090",
				SubjectUsername:   "alice_001",
				SubjectSenderName: "Alice",
				Confidence:        0.864,
			},
			{
				SpaceID:           1,
				SpaceName:         "供需频道",
				FactType:          "供应",
				Title:             "出售二手显示器",
				SubjectSenderName: "Bob",
				Confidence:        0.91,
			},
		}

		result := appendKnowledgeFacts(content, facts, model.LanguageZhCN)

		So(result, ShouldContainSubstring, "## 今日新增情报")
		So(result, ShouldContainSubstring, "### 供应")
		So(result, ShouldContainSubstring, "- Bob：出售二手显示器（置信度 91%）")
		So(result, ShouldContainSubstring, "### 需求")
		So(result, ShouldContainSubstring, "- @alice_001：需要 RTX 4090（置信度 86%）")
	})

	Convey("英文摘要会使用英文标题和用户兜底", t, func() {
		facts := []model.KnowledgeFact{
			{
				FactType:        "Supply",
				Title:           "Selling a camera",
				SubjectSenderID: 42,
				Confidence:      0.8,
			},
		}

		result := appendKnowledgeFacts("", facts, model.LanguageEN)

		So(result, ShouldContainSubstring, "## Newly Captured Knowledge")
		So(result, ShouldContainSubstring, "### Supply")
		So(result, ShouldContainSubstring, "- User 42: Selling a camera (confidence 80%)")
	})
}
