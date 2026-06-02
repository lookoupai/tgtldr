package store

import (
	"strings"
	"testing"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestBuildLLMWikiPageWhereClause(t *testing.T) {
	Convey("wiki page search should use AND semantics across terms", t, func() {
		filter := normalizeLLMWikiPageFilter(LLMWikiPageFilter{
			Query:    "alice gpu",
			SpaceID:  7,
			PageType: "person",
			Page:     -1,
			PageSize: 200,
		})
		whereClause, args := buildLLMWikiPageWhereClause(filter, searchTerms(filter.Query))

		So(filter.Page, ShouldEqual, 1)
		So(filter.PageSize, ShouldEqual, 100)
		So(whereClause, ShouldContainSubstring, "space_id = $1")
		So(whereClause, ShouldContainSubstring, "page_type = $2")
		So(strings.Count(whereClause, "content_text ilike"), ShouldEqual, 2)
		So(args, ShouldResemble, []any{int64(7), "person", "%alice%", "%gpu%"})
	})
}

func TestNormalizeLLMWikiPage(t *testing.T) {
	Convey("wiki page normalization should fill defaults and compact source facts", t, func() {
		page := normalizeLLMWikiPage(model.LLMWikiPage{
			Path:          " spaces/general/people/alice.md ",
			SourceFactIDs: []int64{3, 2, 3, 0},
		})

		So(page.Path, ShouldEqual, "spaces/general/people/alice.md")
		So(page.Title, ShouldEqual, "spaces/general/people/alice.md")
		So(page.PageType, ShouldEqual, "page")
		So(page.SourceFactIDs, ShouldResemble, []int64{3, 2})
	})
}
