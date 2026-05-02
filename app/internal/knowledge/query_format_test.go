package knowledge

import (
	"testing"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFormatQueryResult(t *testing.T) {
	Convey("中文查询结果应包含条件、事实和相关用户", t, func() {
		seenAt := time.Date(2026, 5, 2, 9, 30, 0, 0, time.UTC)
		fact := model.KnowledgeFact{
			FactType:          "demand",
			Title:             "需要 RTX 4090",
			ChatTitle:         "供需群",
			SubjectSenderID:   7,
			SubjectSenderName: "Alice",
			LastSeenAt:        seenAt,
		}
		subject := model.KnowledgeSubject{
			DisplayName:       "Alice",
			FactCount:         1,
			FactTypes:         []string{"demand"},
			LastSeenAt:        seenAt,
			Facts:             []model.KnowledgeFact{fact},
			SubjectSenderID:   7,
			SubjectSenderName: "Alice",
		}

		content := FormatQueryResult(model.LanguageZhCN, "4090", "demand", []model.KnowledgeFact{fact}, []model.KnowledgeSubject{subject})

		So(content, ShouldContainSubstring, "## 知识查询结果")
		So(content, ShouldContainSubstring, "条件：关键词「4090」，类型「demand」")
		So(content, ShouldContainSubstring, "- [Alice](tg://user?id=7)：需要 RTX 4090（demand，供需群，2026-05-02 09:30）")
		So(content, ShouldContainSubstring, "- [Alice](tg://user?id=7)：1 条；类型：demand；代表：需要 RTX 4090")
	})

	Convey("没有事实时应返回空结果提示", t, func() {
		content := FormatQueryResult(model.LanguageEN, "camera", "supply", nil, nil)

		So(content, ShouldContainSubstring, "## Knowledge Query Results")
		So(content, ShouldContainSubstring, `Filters: keyword "camera", type "supply"`)
		So(content, ShouldContainSubstring, "No matching active knowledge facts were found.")
	})
}
