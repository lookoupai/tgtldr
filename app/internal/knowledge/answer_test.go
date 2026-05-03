package knowledge

import (
	"strings"
	"testing"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestKnowledgeAnswerEvidence(t *testing.T) {
	Convey("问答证据上下文应保留事实 ID、用户、置信度和结构化数据", t, func() {
		seenAt := time.Date(2026, 5, 2, 9, 30, 0, 0, time.UTC)

		evidence := buildKnowledgeAnswerEvidence(model.LanguageZhCN, []model.KnowledgeFact{
			{
				ID:              42,
				FactType:        "skill",
				Title:           "Rust 后端经验",
				DataJSON:        `{"area":"Rust","evidence":"持续回答 Tokio 问题"}`,
				SubjectUsername: "alice",
				Confidence:      0.82,
				LastSeenAt:      seenAt,
			},
		}, []model.KnowledgeSubject{
			{
				SubjectUsername: "alice",
				FactCount:       1,
				FactTypes:       []string{"skill"},
				LastSeenAt:      seenAt,
				Facts:           []model.KnowledgeFact{{Title: "Rust 后端经验"}},
			},
		})

		So(evidence, ShouldContainSubstring, "id=#42")
		So(evidence, ShouldContainSubstring, "subject=@alice")
		So(evidence, ShouldContainSubstring, "confidence=82%")
		So(evidence, ShouldContainSubstring, `"area":"Rust"`)
		So(evidence, ShouldContainSubstring, "相关用户")
	})

	Convey("证据上下文会按最大长度截断", t, func() {
		value := truncateRunes(strings.Repeat("a", 20), 8)

		So(value, ShouldEqual, "aaaaaaaa...")
	})
}

func TestKnowledgeAnswerNoEvidenceText(t *testing.T) {
	Convey("没有证据时应明确说明不会编造答案", t, func() {
		content := knowledgeAnswerNoEvidenceText(model.LanguageZhCN, "Rust", "skill")

		So(content, ShouldContainSubstring, "没有找到足够回答这个问题")
		So(content, ShouldContainSubstring, "关键词「Rust」")
		So(content, ShouldContainSubstring, "类型「skill」")
	})
}

func TestEnsureKnowledgeAnswerCitations(t *testing.T) {
	Convey("模型回答缺少事实 ID 时应自动补充依据", t, func() {
		answer := ensureKnowledgeAnswerCitations(model.LanguageZhCN, "可以先联系 Alice。", []model.KnowledgeFact{
			{ID: 42},
			{ID: 43},
		})

		So(answer, ShouldContainSubstring, "依据事实：#42、#43")
	})

	Convey("模型回答已经包含事实 ID 时不重复补充", t, func() {
		answer := ensureKnowledgeAnswerCitations(model.LanguageZhCN, "可以先联系 Alice，依据 #42。", []model.KnowledgeFact{
			{ID: 42},
		})

		So(answer, ShouldEqual, "可以先联系 Alice，依据 #42。")
	})
}
