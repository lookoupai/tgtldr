package bot

import (
	"testing"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFormatTelegramHTML(t *testing.T) {
	Convey("格式化 Markdown 为 Telegram HTML", t, func() {
		input := stringsJoin(
			"## 今日主要结论",
			"",
			"- **节点 A** 表现稳定",
			"- 查看 [文档](https://example.com)",
			"",
			"```",
			"line 1",
			"line 2",
			"```",
			"",
			"`inline` 代码",
		)

		output := formatTelegramHTML(input)

		So(output, ShouldContainSubstring, "<b>【今日主要结论】</b>")
		So(output, ShouldContainSubstring, "• <b>节点 A</b> 表现稳定")
		So(output, ShouldContainSubstring, `<a href="https://example.com">文档</a>`)
		So(output, ShouldContainSubstring, "<pre>line 1\nline 2</pre>")
		So(output, ShouldContainSubstring, "<code>inline</code> 代码")
	})

	Convey("三级标题保持简洁粗体", t, func() {
		output := formatTelegramHTML("### 分话题总结")
		So(output, ShouldEqual, "<b>分话题总结</b>")
	})

	Convey("超长消息会自动拆成多段", t, func() {
		body := stringsJoin(
			"## 今日主要结论",
			"",
			"- **很长的摘要** "+repeatText("机场稳定性讨论。", 500),
			"",
			"## 分话题总结",
			"",
			"- "+repeatText("第二段内容。", 500),
		)

		parts := formatTelegramMessages(body, model.LanguageZhCN)

		So(len(parts), ShouldBeGreaterThan, 1)
		for _, part := range parts {
			So(telegramVisibleLength(part) <= telegramMessageVisibleLimit, ShouldBeTrue)
			So(part, ShouldNotContainSubstring, telegramTruncationNotice(model.LanguageZhCN))
		}
		So(parts[0], ShouldContainSubstring, "<b>【今日主要结论】</b>")
		So(parts[len(parts)-1], ShouldContainSubstring, "第二段内容。")
	})

	Convey("短消息保持单段", t, func() {
		parts := formatTelegramMessages("## 今日主要结论\n\n- 一切正常", model.LanguageZhCN)

		So(parts, ShouldHaveLength, 1)
		So(parts[0], ShouldContainSubstring, "<b>【今日主要结论】</b>")
	})

	Convey("拆分时会闭合并重开跨段标签", t, func() {
		formatted := formatTelegramHTML("**" + repeatText("粗体内容", 80) + "**")
		parts := splitTelegramHTML(formatted, 30)

		So(len(parts), ShouldBeGreaterThan, 1)
		for _, part := range parts {
			So(telegramVisibleLength(part) <= 30, ShouldBeTrue)
			So(part, ShouldStartWith, "<b>")
			So(part, ShouldEndWith, "</b>")
		}
	})
}

func stringsJoin(lines ...string) string {
	result := ""
	for index, line := range lines {
		if index > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

func repeatText(text string, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		result += text
	}
	return result
}
