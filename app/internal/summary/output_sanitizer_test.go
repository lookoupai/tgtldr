package summary

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSanitizeSummaryOutput(t *testing.T) {
	Convey("移除前置思考内容并保留正式摘要", t, func() {
		content := "Thinking...\n> 分析输入\n> 整理结构\n\n## 今日主要结论\n- 保留正式摘要"

		got := sanitizeSummaryOutput(content)

		So(got, ShouldEqual, "## 今日主要结论\n- 保留正式摘要")
	})

	Convey("移除标题前的前言并清理内部锚点", t, func() {
		content := "下面给出摘要：\n\n## Key Takeaways\n- [m001] keep this point"

		got := sanitizeSummaryOutput(content)

		So(got, ShouldEqual, "## Key Takeaways\n-  keep this point")
	})

	Convey("没有标准标题时保留原始正文", t, func() {
		content := "- 直接输出结论"

		got := sanitizeSummaryOutput(content)

		So(got, ShouldEqual, "- 直接输出结论")
	})
}
