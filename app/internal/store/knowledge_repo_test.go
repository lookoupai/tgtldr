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

func TestMergeKnowledgeFacts(t *testing.T) {
	Convey("重复知识事实应合并证据、时间和置信度", t, func() {
		older := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
		newer := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
		laterExpiry := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
		earlierExpiry := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)

		merged := mergeKnowledgeFacts(
			model.KnowledgeFact{
				ID:                10,
				SubjectSenderName: "Alice",
				SubjectUsername:   "alice",
				Confidence:        0.9,
				Status:            model.KnowledgeFactStatusActive,
				SourceMessageIDs:  []int{5, 3},
				FirstSeenAt:       older,
				LastSeenAt:        older,
				ExpiresAt:         &laterExpiry,
			},
			model.KnowledgeFact{
				FactType:         "demand",
				Title:            "需要显卡",
				DataJSON:         `{"item":"显卡"}`,
				Confidence:       0.7,
				Status:           model.KnowledgeFactStatusActive,
				SourceMessageIDs: []int{3, 8},
				FirstSeenAt:      newer,
				LastSeenAt:       newer,
				ExpiresAt:        &earlierExpiry,
			},
		)

		So(merged.ID, ShouldEqual, 10)
		So(merged.Confidence, ShouldEqual, 0.9)
		So(merged.SubjectSenderName, ShouldEqual, "Alice")
		So(merged.SubjectUsername, ShouldEqual, "alice")
		So(merged.SourceMessageIDs, ShouldResemble, []int{3, 5, 8})
		So(merged.FirstSeenAt, ShouldEqual, older)
		So(merged.LastSeenAt, ShouldEqual, newer)
		So(*merged.ExpiresAt, ShouldEqual, laterExpiry)
	})

	Convey("已忽略的事实合并后仍保持 dismissed", t, func() {
		merged := mergeKnowledgeFacts(
			model.KnowledgeFact{
				ID:     11,
				Status: model.KnowledgeFactStatusDismissed,
			},
			model.KnowledgeFact{
				Status: model.KnowledgeFactStatusActive,
			},
		)

		So(merged.Status, ShouldEqual, model.KnowledgeFactStatusDismissed)
	})
}

func TestNormalizeKnowledgeFactForUpsert(t *testing.T) {
	Convey("写入前应清理知识事实字段并压缩消息 ID", t, func() {
		seenAt := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)

		normalized := normalizeKnowledgeFactForUpsert(model.KnowledgeFact{
			FactType:          " demand ",
			Title:             " 需要显卡 ",
			SubjectSenderName: " Alice ",
			SubjectUsername:   " alice ",
			SourceMessageIDs:  []int{3, 0, 2, 3},
			LastSeenAt:        seenAt,
		})

		So(normalized.FactType, ShouldEqual, "demand")
		So(normalized.Title, ShouldEqual, "需要显卡")
		So(normalized.SubjectSenderName, ShouldEqual, "Alice")
		So(normalized.SubjectUsername, ShouldEqual, "alice")
		So(normalized.DataJSON, ShouldEqual, "{}")
		So(normalized.Status, ShouldEqual, model.KnowledgeFactStatusActive)
		So(normalized.SourceMessageIDs, ShouldResemble, []int{2, 3})
		So(normalized.FirstSeenAt, ShouldEqual, seenAt)
	})
}
