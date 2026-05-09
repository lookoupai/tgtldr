package store

import (
	"context"
	"fmt"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DeliveryChannelRunRepository struct {
	pool *pgxpool.Pool
}

func (r *DeliveryChannelRunRepository) GetByChannelAndDate(ctx context.Context, channelID int64, date string) (model.DeliveryChannelRun, error) {
	run, err := scanDeliveryChannelRun(rowScanner{row: r.pool.QueryRow(ctx, `
		select id, channel_id, summary_date::text, status, content, model,
		       generated_at, delivered_at, delivery_error, error_message, created_at, updated_at
		from delivery_channel_runs
		where channel_id = $1 and summary_date = $2::date
	`, channelID, date)})
	if err != nil {
		return model.DeliveryChannelRun{}, fmt.Errorf("get delivery channel run %d on %s: %w", channelID, date, err)
	}
	return run, nil
}

func (r *DeliveryChannelRunRepository) TrySetRunning(ctx context.Context, channelID int64, date string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		insert into delivery_channel_runs (channel_id, summary_date, status)
		values ($1, $2::date, 'running')
		on conflict (channel_id, summary_date) do update
		set status = 'running',
		    delivery_error = '',
		    error_message = '',
		    updated_at = now()
		where delivery_channel_runs.delivered_at is null
	`, channelID, date)
	if err != nil {
		return false, fmt.Errorf("set delivery channel run running: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *DeliveryChannelRunRepository) MarkDelivered(ctx context.Context, channelID int64, date string, content string, modelName string, generatedAt time.Time, deliveredAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		insert into delivery_channel_runs (
			channel_id, summary_date, status, content, model, generated_at, delivered_at, delivery_error, error_message
		)
		values ($1, $2::date, 'succeeded', $3, $4, $5, $6, '', '')
		on conflict (channel_id, summary_date) do update
		set status = 'succeeded',
		    content = excluded.content,
		    model = excluded.model,
		    generated_at = excluded.generated_at,
		    delivered_at = excluded.delivered_at,
		    delivery_error = '',
		    error_message = '',
		    updated_at = now()
	`, channelID, date, content, modelName, generatedAt, deliveredAt)
	if err != nil {
		return fmt.Errorf("mark delivery channel run delivered: %w", err)
	}
	return nil
}

func (r *DeliveryChannelRunRepository) MarkFailed(ctx context.Context, channelID int64, date string, message string) error {
	_, err := r.pool.Exec(ctx, `
		insert into delivery_channel_runs (channel_id, summary_date, status, delivery_error, error_message)
		values ($1, $2::date, 'failed', $3, $3)
		on conflict (channel_id, summary_date) do update
		set status = 'failed',
		    delivered_at = null,
		    delivery_error = excluded.delivery_error,
		    error_message = excluded.error_message,
		    updated_at = now()
	`, channelID, date, message)
	if err != nil {
		return fmt.Errorf("mark delivery channel run failed: %w", err)
	}
	return nil
}

type deliveryChannelRunScanner interface {
	Scan(dest ...any) error
}

func scanDeliveryChannelRun(scanner deliveryChannelRunScanner) (model.DeliveryChannelRun, error) {
	var run model.DeliveryChannelRun
	err := scanner.Scan(
		&run.ID,
		&run.ChannelID,
		&run.SummaryDate,
		&run.Status,
		&run.Content,
		&run.Model,
		&run.GeneratedAt,
		&run.DeliveredAt,
		&run.DeliveryError,
		&run.ErrorMessage,
		&run.CreatedAt,
		&run.UpdatedAt,
	)
	if err != nil {
		return model.DeliveryChannelRun{}, err
	}
	return run, nil
}
