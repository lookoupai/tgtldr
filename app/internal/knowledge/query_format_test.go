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
			SubjectSenderName: "Alice",
			SubjectUsername:   "alice_001",
			LastSeenAt:        seenAt,
		}
		subject := model.KnowledgeSubject{
			DisplayName:     "@alice_001",
			FactCount:       1,
			FactTypes:       []string{"demand"},
			LastSeenAt:      seenAt,
			Facts:           []model.KnowledgeFact{fact},
			SubjectUsername: "alice_001",
		}

		content := FormatQueryResult(model.LanguageZhCN, "4090", "demand", []model.KnowledgeFact{fact}, []model.KnowledgeSubject{subject})

		So(content, ShouldContainSubstring, "## 知识查询结果")
		So(content, ShouldContainSubstring, "条件：关键词「4090」，类型「demand」")
		So(content, ShouldContainSubstring, "- @alice_001：需要 RTX 4090（demand，供需群，2026-05-02 09:30）")
		So(content, ShouldContainSubstring, "- @alice_001：1 条；类型：demand；代表：需要 RTX 4090")
	})

	Convey("没有事实时应返回空结果提示", t, func() {
		content := FormatQueryResult(model.LanguageEN, "camera", "supply", nil, nil)

		So(content, ShouldContainSubstring, "## Knowledge Query Results")
		So(content, ShouldContainSubstring, `Filters: keyword "camera", type "supply"`)
		So(content, ShouldContainSubstring, "No matching active knowledge facts were found.")
	})
}
