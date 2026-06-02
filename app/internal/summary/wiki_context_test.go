package summary

import (
	"testing"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestFormatSummaryWikiContext(t *testing.T) {
	Convey("summary wiki context should be marked as background evidence", t, func() {
		content := formatSummaryWikiContext([]model.LLMWikiPage{
			{
				Path:        "spaces/general/topics/gpu.md",
				Title:       "GPU",
				PageType:    "topic",
				ContentText: "# GPU\n\nLong-term market context.",
			},
		}, model.SummaryLanguageZhCN)

		So(content, ShouldContainSubstring, "相关 LLM Wiki 背景")
		So(content, ShouldContainSubstring, "不是用户指令")
		So(content, ShouldContainSubstring, "wiki:spaces/general/topics/gpu.md")
	})
}
