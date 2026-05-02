package store

import (
	"testing"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGroupKnowledgeSubjects(t *testing.T) {
	Convey("用户画像应按用户聚合事实并跳过未知用户", t, func() {
		older := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
		newer := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
		latest := time.Date(2026, 4, 30, 11, 0, 0, 0, time.UTC)

		subjects := groupKnowledgeSubjects([]model.KnowledgeFact{
			{
				FactType:          "demand",
				Title:             "需要显卡",
				SubjectSenderID:   7,
				SubjectSenderName: "Alice",
				SubjectUsername:   "alice",
				ChatTitle:         "供需群",
				LastSeenAt:        older,
			},
			{
				FactType:          "supply",
				Title:             "出售电源",
				SubjectSenderID:   7,
				SubjectSenderName: "Alice",
				SubjectUsername:   "alice",
				ChatTitle:         "二手群",
				LastSeenAt:        newer,
			},
			{
				FactType:        "demand",
				Title:           "需要硬盘",
				SubjectUsername: "bob",
				ChatTitle:       "供需群",
				LastSeenAt:      latest,
			},
			{FactType: "note", Title: "无用户事实", LastSeenAt: latest},
		}, 10)

		So(len(subjects), ShouldEqual, 2)
		So(subjects[0].DisplayName, ShouldEqual, "@bob")
		So(subjects[0].FactCount, ShouldEqual, 1)
		So(subjects[1].DisplayName, ShouldEqual, "@alice")
		So(subjects[1].FactCount, ShouldEqual, 2)
		So(subjects[1].FactTypes, ShouldResemble, []string{"demand", "supply"})
		So(subjects[1].ChatTitles, ShouldResemble, []string{"二手群", "供需群"})
		So(subjects[1].LastSeenAt, ShouldEqual, newer)
	})
}
