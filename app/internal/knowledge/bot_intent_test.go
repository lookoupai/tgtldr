package knowledge

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBotIntentParsing(t *testing.T) {
	Convey("模型意图 JSON 应规整为统一动作和事实类型", t, func() {
		intent, err := parseBotIntent("```json\n{\"intent\":\"record\",\"confidence\":0.91,\"factType\":\"seller\",\"subject\":\" @tianyou158 \",\"item\":\" Telegram 账号 \",\"action\":\"\",\"needsConfirmation\":false}\n```")

		So(err, ShouldBeNil)
		So(intent.Intent, ShouldEqual, BotIntentFactUpsert)
		So(intent.Confidence, ShouldEqual, 0.91)
		So(intent.FactType, ShouldEqual, "supply")
		So(intent.Subject, ShouldEqual, "@tianyou158")
		So(intent.Item, ShouldEqual, "Telegram 账号")
		So(intent.Query, ShouldEqual, "Telegram 账号")
	})

	Convey("直接供给陈述会转成高置信 fact_upsert", t, func() {
		intent, ok := directBotIntent("@tianyou158 是卖telegram账号的")

		So(ok, ShouldBeTrue)
		So(intent.Intent, ShouldEqual, BotIntentFactUpsert)
		So(intent.Confidence, ShouldEqual, 1)
		So(intent.FactType, ShouldEqual, "supply")
		So(intent.Subject, ShouldEqual, "@tianyou158")
		So(intent.Item, ShouldEqual, "telegram账号")
	})

	Convey("风险账号澄清会转成 cleared 意图", t, func() {
		intent, ok := directBotIntent("@tianyou158 不是风险账号")

		So(ok, ShouldBeTrue)
		So(intent.Intent, ShouldEqual, BotIntentFactUpsert)
		So(intent.FactType, ShouldEqual, "risk_account")
		So(intent.Action, ShouldEqual, "cleared")
		So(intent.Subject, ShouldEqual, "@tianyou158")
	})

	Convey("风险账号问句不会走直接维护或写入意图", t, func() {
		_, ok := directBotIntent("@tianyou158 是风险账号吗")

		So(ok, ShouldBeFalse)
	})
}
