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

func TestNormalizeKnowledgeQueryLimit(t *testing.T) {
	Convey("知识查询限制应有默认值和上限", t, func() {
		So(normalizeKnowledgeQueryLimit(0), ShouldEqual, 20)
		So(normalizeKnowledgeQueryLimit(-1), ShouldEqual, 20)
		So(normalizeKnowledgeQueryLimit(30), ShouldEqual, 30)
		So(normalizeKnowledgeQueryLimit(101), ShouldEqual, 100)
	})
}

func TestOrderedSourceMessages(t *testing.T) {
	Convey("来源消息应按事实中的消息 ID 顺序返回并跳过缺失项", t, func() {
		messages := orderedSourceMessages([]int{5, 3, 8}, map[int]model.Message{
			3: {TelegramMessageID: 3, TextContent: "three"},
			5: {TelegramMessageID: 5, TextContent: "five"},
		})

		So(messages, ShouldHaveLength, 2)
		So(messages[0].TelegramMessageID, ShouldEqual, 5)
		So(messages[1].TelegramMessageID, ShouldEqual, 3)
	})
}
