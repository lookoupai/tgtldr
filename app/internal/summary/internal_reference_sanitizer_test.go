package summary

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSanitizeSummaryInternalReferences(t *testing.T) {
	Convey("删除正文里的内部消息锚点并保留其余文本", t, func() {
		content := "- [m007] 提供多平台 API 号码\n- [m001][m018] 强调墨西哥卡无需等待"

		got := sanitizeSummaryInternalReferences(content)

		So(got, ShouldEqual, "-  提供多平台 API 号码\n-  强调墨西哥卡无需等待")
	})

	Convey("保留 Markdown 用户链接，不误删真实用户引用", t, func() {
		content := "- [Cheap Ads](tg://user?id=8018409353) 提供账号\n- [m019] 推广 @Z555RBOT"

		got := sanitizeSummaryInternalReferences(content)

		So(got, ShouldEqual, "- [Cheap Ads](tg://user?id=8018409353) 提供账号\n-  推广 @Z555RBOT")
	})

	Convey("删除 ref 和 msg 形式的内部锚点", t, func() {
		content := "- [ref001] 回复了上文\n- [msg:123] 原消息不可见"

		got := sanitizeSummaryInternalReferences(content)

		So(got, ShouldEqual, "-  回复了上文\n-  原消息不可见")
	})
}
