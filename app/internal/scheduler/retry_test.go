package scheduler

import (
	"testing"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	. "github.com/smartystreets/goconvey/convey"
)

func TestShouldRetrySummary(t *testing.T) {
	Convey("只有失败、未超限且到达重试时间的摘要才自动重试", t, func() {
		now := time.Date(2026, time.May, 23, 10, 0, 0, 0, time.UTC)
		past := now.Add(-time.Minute)
		future := now.Add(time.Minute)
		settings := model.AppSettings{SummaryRetryLimit: 2}

		So(shouldRetrySummary(settings, model.Summary{
			Status:      model.SummaryStatusFailed,
			RetryCount:  1,
			NextRetryAt: &past,
		}, now), ShouldBeTrue)

		So(shouldRetrySummary(settings, model.Summary{
			Status:      model.SummaryStatusFailed,
			RetryCount:  1,
			NextRetryAt: &future,
		}, now), ShouldBeFalse)

		So(shouldRetrySummary(settings, model.Summary{
			Status:      model.SummaryStatusFailed,
			RetryCount:  2,
			NextRetryAt: &past,
		}, now), ShouldBeFalse)

		So(shouldRetrySummary(model.AppSettings{SummaryRetryLimit: 0}, model.Summary{
			Status:      model.SummaryStatusFailed,
			RetryCount:  0,
			NextRetryAt: &past,
		}, now), ShouldBeFalse)

		So(shouldRetrySummary(settings, model.Summary{
			Status:      model.SummaryStatusSucceeded,
			RetryCount:  0,
			NextRetryAt: &past,
		}, now), ShouldBeFalse)
	})
}

func TestSummaryRetryDelay(t *testing.T) {
	Convey("重试间隔按基础分钟和倍率退避", t, func() {
		settings := model.AppSettings{
			SummaryRetryBackoffBaseMinutes: 2,
			SummaryRetryBackoffMultiplier:  3,
		}

		So(summaryRetryDelay(settings, 0), ShouldEqual, 2*time.Minute)
		So(summaryRetryDelay(settings, 1), ShouldEqual, 6*time.Minute)
		So(summaryRetryDelay(settings, 2), ShouldEqual, 18*time.Minute)
	})

	Convey("无效退避配置使用默认值", t, func() {
		So(summaryRetryDelay(model.AppSettings{}, 1), ShouldEqual, 3*time.Minute)
	})
}
