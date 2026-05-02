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
