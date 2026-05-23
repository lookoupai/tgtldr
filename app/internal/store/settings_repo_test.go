package store

import (
	"testing"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNormalizeAppSettingsLanguage(t *testing.T) {
	Convey("语言设置为空或非法时默认使用中文", t, func() {
		So(normalizeAppSettings(model.AppSettings{}).Language, ShouldEqual, model.LanguageZhCN)
		So(normalizeAppSettings(model.AppSettings{Language: "fr"}).Language, ShouldEqual, model.LanguageZhCN)
	})

	Convey("语言设置为英文时保留英文", t, func() {
		settings := normalizeAppSettings(model.AppSettings{Language: model.LanguageEN})

		So(settings.Language, ShouldEqual, model.LanguageEN)
	})

	Convey("摘要输出语言支持内置语言和自定义语言", t, func() {
		So(normalizeAppSettings(model.AppSettings{}).SummaryOutputLanguage, ShouldEqual, model.SummaryLanguageZhCN)
		So(normalizeAppSettings(model.AppSettings{SummaryOutputLanguage: model.SummaryLanguageAR}).SummaryOutputLanguage, ShouldEqual, model.SummaryLanguageAR)
		So(normalizeAppSettings(model.AppSettings{SummaryOutputLanguage: "Japanese"}).SummaryOutputLanguage, ShouldEqual, model.SummaryOutputLanguage("Japanese"))
	})

	Convey("私聊 Bot 授权用户会去除空行和首尾空白", t, func() {
		settings := normalizeAppSettings(model.AppSettings{BotPrivateAllowedUsers: []string{" 123 ", "", " @alice "}})

		So(settings.BotPrivateAllowedUsers, ShouldResemble, []string{"123", "@alice"})
	})

	Convey("OpenAI 调用方式为空或非法时默认使用流式", t, func() {
		So(normalizeAppSettings(model.AppSettings{}).OpenAIRequestMode, ShouldEqual, model.OpenAIRequestModeStream)
		So(normalizeAppSettings(model.AppSettings{OpenAIRequestMode: "invalid"}).OpenAIRequestMode, ShouldEqual, model.OpenAIRequestModeStream)
	})

	Convey("OpenAI 非流式调用方式会保留", t, func() {
		settings := normalizeAppSettings(model.AppSettings{OpenAIRequestMode: model.OpenAIRequestModeNonStream})

		So(settings.OpenAIRequestMode, ShouldEqual, model.OpenAIRequestModeNonStream)
	})

	Convey("摘要失败重试配置会归一化", t, func() {
		settings := normalizeAppSettings(model.AppSettings{
			SummaryRetryLimit:              -1,
			SummaryRetryBackoffBaseMinutes: 0,
			SummaryRetryBackoffMultiplier:  0,
		})

		So(settings.SummaryRetryLimit, ShouldEqual, 0)
		So(settings.SummaryRetryBackoffBaseMinutes, ShouldEqual, model.DefaultSummaryRetryBackoffBaseMinutes)
		So(settings.SummaryRetryBackoffMultiplier, ShouldEqual, model.DefaultSummaryRetryBackoffMultiplier)
	})
}
