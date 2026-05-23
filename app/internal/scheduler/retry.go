package scheduler

import (
	"context"
	"math"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
)

func (s *Service) scheduleNextRetry(ctx context.Context, result model.Summary) error {
	settings, err := s.store.Settings.Get(ctx)
	if err != nil {
		return err
	}

	item, err := s.store.Summaries.GetByChatAndDate(ctx, result.ChatID, result.SummaryDate)
	if err != nil {
		return err
	}
	if !canScheduleSummaryRetry(settings, item) {
		return nil
	}

	return s.store.Summaries.ScheduleRetry(ctx, result.ChatID, result.SummaryDate, s.clock.Now().Add(summaryRetryDelay(settings, item.RetryCount)))
}

func shouldRetrySummary(settings model.AppSettings, item model.Summary, now time.Time) bool {
	if item.Status != model.SummaryStatusFailed {
		return false
	}
	if !canScheduleSummaryRetry(settings, item) {
		return false
	}
	return item.NextRetryAt != nil && !item.NextRetryAt.After(now)
}

func canScheduleSummaryRetry(settings model.AppSettings, item model.Summary) bool {
	limit := settings.SummaryRetryLimit
	if limit < 0 {
		limit = 0
	}
	return limit > 0 && item.RetryCount < limit
}

func summaryRetryDelay(settings model.AppSettings, retryCount int) time.Duration {
	baseMinutes := settings.SummaryRetryBackoffBaseMinutes
	if baseMinutes <= 0 {
		baseMinutes = model.DefaultSummaryRetryBackoffBaseMinutes
	}
	multiplier := settings.SummaryRetryBackoffMultiplier
	if multiplier < 1 {
		multiplier = model.DefaultSummaryRetryBackoffMultiplier
	}
	if retryCount < 0 {
		retryCount = 0
	}

	minutes := float64(baseMinutes) * math.Pow(multiplier, float64(retryCount))
	return time.Duration(minutes * float64(time.Minute))
}
