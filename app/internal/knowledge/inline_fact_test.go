package knowledge

import (
	"testing"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestInlineFactParsing(t *testing.T) {
	Convey("回复 Bot 的供给陈述会解析成可直接记录的供应事实", t, func() {
		instruction, ok := parseDirectInlineFactText("@tianyou158 是卖telegram账号的")

		So(ok, ShouldBeTrue)
		So(instruction.FactType, ShouldEqual, "supply")
		So(instruction.SubjectUsername, ShouldEqual, "tianyou158")
		So(instruction.SubjectName, ShouldEqual, "@tianyou158")
		So(instruction.Item, ShouldEqual, "telegram账号")
	})

	Convey("风险账号澄清会解析成 cleared 风险账号事实", t, func() {
		instruction, ok := parseDirectInlineFactText("@tianyou158 不是风险账号")

		So(ok, ShouldBeTrue)
		So(instruction.FactType, ShouldEqual, "risk_account")
		So(instruction.SubjectUsername, ShouldEqual, "tianyou158")
		So(instruction.Item, ShouldEqual, "")
	})

	Convey("风险账号问句不会解析成 cleared 事实", t, func() {
		_, ok := parseDirectInlineFactText("@tianyou158 不是风险账号吗")

		So(ok, ShouldBeFalse)
	})

	Convey("问题不会被误解析成写入事实", t, func() {
		_, ok := parseDirectInlineFactText("@tianyou158 是做什么的？")

		So(ok, ShouldBeFalse)
	})
}

func TestInlineFactSpaceSelection(t *testing.T) {
	Convey("供需事实优先写入供需空间", t, func() {
		supply := model.KnowledgeSpace{Name: "供需频道"}
		general := model.KnowledgeSpace{Name: defaultGeneralKnowledgeSpaceName}

		So(inlineFactSpaceScore(supply, "supply"), ShouldBeGreaterThan, inlineFactSpaceScore(general, "supply"))
	})

	Convey("schema 必须显式支持目标事实类型", t, func() {
		space := model.KnowledgeSpace{SchemaJSON: `{"types":{"supply":{}}}`}

		So(knowledgeSpaceSupportsFactType(space, "supply"), ShouldBeTrue)
		So(knowledgeSpaceSupportsFactType(space, "risk_account"), ShouldBeFalse)
	})
}

func TestInlineFactInstructionFromBotIntent(t *testing.T) {
	Convey("LLM 意图里的自然表达供应事实可转换为写入指令", t, func() {
		instruction, ok := inlineFactInstructionFromBotIntent(BotIntent{
			Intent:     BotIntentFactUpsert,
			Confidence: 0.9,
			FactType:   "supply",
			Subject:    "@tianyou158",
			Item:       "Telegram 账号",
			SourceText: "@tianyou158 搞飞机号的",
		})

		So(ok, ShouldBeTrue)
		So(instruction.FactType, ShouldEqual, "supply")
		So(instruction.SubjectUsername, ShouldEqual, "tianyou158")
		So(instruction.Item, ShouldEqual, "Telegram 账号")
		So(instruction.SourceText, ShouldEqual, "@tianyou158 搞飞机号的")
	})

	Convey("不支持的事实类型不会写库", t, func() {
		_, ok := inlineFactInstructionFromBotIntent(BotIntent{
			Intent:     BotIntentFactUpsert,
			Confidence: 0.9,
			FactType:   "event",
			Subject:    "@alice",
			Item:       "线下聚会",
		})

		So(ok, ShouldBeFalse)
	})
}
