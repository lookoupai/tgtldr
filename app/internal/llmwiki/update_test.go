package llmwiki

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseWikiUpdateResponse(t *testing.T) {
	Convey("wiki update response should parse fenced JSON", t, func() {
		parsed, err := parseWikiUpdateResponse("```json\n{\"updates\":[{\"path\":\"spaces/general/topics/gpu.md\",\"title\":\"GPU\",\"type\":\"topic\",\"content\":\"# GPU\"}],\"logEntry\":\"updated gpu\"}\n```")

		So(err, ShouldBeNil)
		So(parsed.Updates, ShouldHaveLength, 1)
		So(parsed.Updates[0].Path, ShouldEqual, "spaces/general/topics/gpu.md")
		So(parsed.LogEntry, ShouldEqual, "updated gpu")
	})
}

func TestNormalizePageUpdate(t *testing.T) {
	Convey("wiki page updates should receive frontmatter when absent", t, func() {
		update, err := normalizePageUpdate(pageUpdate{
			Path:          "spaces/general/topics/gpu.md",
			Title:         "GPU",
			Type:          "topic",
			SourceFactIDs: []int64{42},
			Content:       "# GPU\n\nMarket notes.",
		})

		So(err, ShouldBeNil)
		So(update.Content, ShouldStartWith, "---\n")
		So(update.Content, ShouldContainSubstring, "type: topic")
		So(update.Content, ShouldContainSubstring, "source_fact_ids: [42]")
	})
}

func TestApplyWikiUpdateResponse(t *testing.T) {
	Convey("wiki update response should write pages and append log", t, func() {
		root := t.TempDir()
		service := NewService(nil, root, 0)
		So(service.EnsureWorkspace(), ShouldBeNil)

		updated, err := service.applyWikiUpdateResponse(updateResponse{
			Updates: []pageUpdate{
				{
					Path:    "spaces/general/topics/gpu.md",
					Title:   "GPU",
					Type:    "topic",
					Content: "# GPU\n\nMarket notes.",
				},
			},
			LogEntry: "updated gpu",
		})

		So(err, ShouldBeNil)
		So(updated, ShouldEqual, 1)
		content, err := os.ReadFile(filepath.Join(root, "spaces", "general", "topics", "gpu.md"))
		So(err, ShouldBeNil)
		So(string(content), ShouldContainSubstring, "# GPU")
		logContent, err := os.ReadFile(filepath.Join(root, "log.md"))
		So(err, ShouldBeNil)
		So(string(logContent), ShouldContainSubstring, "updated gpu")
	})
}
