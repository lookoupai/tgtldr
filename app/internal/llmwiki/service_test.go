package llmwiki

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestParsePage(t *testing.T) {
	Convey("frontmatter should provide wiki page metadata", t, func() {
		updatedAt := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
		page := ParsePage("spaces/general/people/alice.md", `---
type: person
title: Alice
space_id: 7
source_fact_ids: [10, 11, 10, 0]
---

# Ignored Heading

Alice profile.
`, updatedAt)

		So(page.Path, ShouldEqual, "spaces/general/people/alice.md")
		So(page.Title, ShouldEqual, "Alice")
		So(page.PageType, ShouldEqual, "person")
		So(page.SpaceID, ShouldEqual, 7)
		So(page.SourceFactIDs, ShouldResemble, []int64{10, 11})
		So(page.UpdatedAt, ShouldEqual, updatedAt)
		So(page.ContentHash, ShouldNotBeEmpty)
	})

	Convey("heading should be used when title frontmatter is absent", t, func() {
		page := ParsePage("topics/gpu.md", "# GPU Market\n\nNotes.", time.Time{})

		So(page.Title, ShouldEqual, "GPU Market")
		So(page.PageType, ShouldEqual, "page")
	})
}

func TestWorkspaceEnsureAndScan(t *testing.T) {
	Convey("workspace should create default pages and scan markdown files", t, func() {
		root := t.TempDir()
		workspace := NewWorkspace(root)

		So(workspace.Ensure(), ShouldBeNil)
		So(fileExists(filepath.Join(root, "AGENTS.md")), ShouldBeTrue)
		So(fileExists(filepath.Join(root, "index.md")), ShouldBeTrue)
		So(fileExists(filepath.Join(root, "log.md")), ShouldBeTrue)

		pagePath := filepath.Join(root, "spaces", "general", "topics")
		So(os.MkdirAll(pagePath, 0o700), ShouldBeNil)
		So(os.WriteFile(filepath.Join(pagePath, "gpu.md"), []byte(`# GPU Market

Notes.
`), 0o600), ShouldBeNil)
		So(os.WriteFile(filepath.Join(pagePath, "ignore.txt"), []byte("ignored"), 0o600), ShouldBeNil)

		pages, err := workspace.ScanPages()

		So(err, ShouldBeNil)
		So(pages, ShouldHaveLength, 4)
		So(pageTitles(pages), ShouldContain, "GPU Market")
	})
}

func TestWorkspaceSafePath(t *testing.T) {
	Convey("workspace paths must stay inside root", t, func() {
		workspace := NewWorkspace(t.TempDir())

		_, err := workspace.SafePath("../outside.md")
		So(err, ShouldNotBeNil)

		path, err := workspace.SafePath("spaces/general/index.md")
		So(err, ShouldBeNil)
		So(path, ShouldContainSubstring, filepath.Join("spaces", "general", "index.md"))
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func pageTitles(pages []model.LLMWikiPage) []string {
	out := make([]string, 0, len(pages))
	for _, page := range pages {
		out = append(out, page.Title)
	}
	return out
}
