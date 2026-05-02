package api

import (
	"testing"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNormalizeKnowledgeFactStatusForUpdate(t *testing.T) {
	Convey("事实状态更新只允许 active 和 dismissed", t, func() {
		So(normalizeKnowledgeFactStatusForUpdate(model.KnowledgeFactStatusActive), ShouldEqual, model.KnowledgeFactStatusActive)
		So(normalizeKnowledgeFactStatusForUpdate(model.KnowledgeFactStatusDismissed), ShouldEqual, model.KnowledgeFactStatusDismissed)
		So(normalizeKnowledgeFactStatusForUpdate(model.KnowledgeFactStatusExpired), ShouldEqual, model.KnowledgeFactStatus(""))
		So(normalizeKnowledgeFactStatusForUpdate("unknown"), ShouldEqual, model.KnowledgeFactStatus(""))
	})
}

func TestKnowledgeFactTypeParam(t *testing.T) {
	Convey("事实类型参数应优先使用 type 并兼容 factType", t, func() {
		So(knowledgeFactTypeParam(" demand ", "supply"), ShouldEqual, "demand")
		So(knowledgeFactTypeParam("", " supply "), ShouldEqual, "supply")
		So(knowledgeFactTypeParam(" ", ""), ShouldEqual, "")
	})
}
