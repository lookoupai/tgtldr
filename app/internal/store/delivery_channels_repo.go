package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DeliveryChannelRepository struct {
	pool *pgxpool.Pool
}

func (r *DeliveryChannelRepository) List(ctx context.Context) ([]model.DeliveryChannel, error) {
	rows, err := r.pool.Query(ctx, `
		select id, name, enabled, source_chat_ids, target_chat_id, target_language,
		       content_filter, content_filter_types, summary_time_local, summary_timezone,
		       summary_prompt, summary_knowledge_days, created_at, updated_at
		from delivery_channels
		order by name asc
	`)
	if err != nil {
		return nil, fmt.Errorf("query delivery channels: %w", err)
	}
	defer rows.Close()

	channels := make([]model.DeliveryChannel, 0)
	for rows.Next() {
		channel, err := scanDeliveryChannel(rows)
		if err != nil {
			return nil, fmt.Errorf("scan delivery channel: %w", err)
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func (r *DeliveryChannelRepository) ListEnabled(ctx context.Context) ([]model.DeliveryChannel, error) {
	rows, err := r.pool.Query(ctx, `
		select id, name, enabled, source_chat_ids, target_chat_id, target_language,
		       content_filter, content_filter_types, summary_time_local, summary_timezone,
		       summary_prompt, summary_knowledge_days, created_at, updated_at
		from delivery_channels
		where enabled = true
		order by name asc
	`)
	if err != nil {
		return nil, fmt.Errorf("query enabled delivery channels: %w", err)
	}
	defer rows.Close()

	channels := make([]model.DeliveryChannel, 0)
	for rows.Next() {
		channel, err := scanDeliveryChannel(rows)
		if err != nil {
			return nil, fmt.Errorf("scan enabled delivery channel: %w", err)
		}
		channels = append(channels, channel)
	}
	return channels, rows.Err()
}

func (r *DeliveryChannelRepository) GetByID(ctx context.Context, id int64) (model.DeliveryChannel, error) {
	channel, err := scanDeliveryChannel(rowScanner{row: r.pool.QueryRow(ctx, `
		select id, name, enabled, source_chat_ids, target_chat_id, target_language,
		       content_filter, content_filter_types, summary_time_local, summary_timezone,
		       summary_prompt, summary_knowledge_days, created_at, updated_at
		from delivery_channels
		where id = $1
	`, id)})
	if err != nil {
		return model.DeliveryChannel{}, fmt.Errorf("get delivery channel %d: %w", id, err)
	}
	return channel, nil
}

func (r *DeliveryChannelRepository) Save(ctx context.Context, channel model.DeliveryChannel) (model.DeliveryChannel, error) {
	saved, err := scanDeliveryChannel(rowScanner{row: r.pool.QueryRow(ctx, `
		update delivery_channels
		set name = $1,
		    enabled = $2,
		    source_chat_ids = $3,
		    target_chat_id = $4,
		    target_language = $5,
		    content_filter = $6,
		    content_filter_types = $7,
		    summary_time_local = $8,
		    summary_timezone = $9,
		    summary_prompt = $10,
		    summary_knowledge_days = $11,
		    updated_at = now()
		where id = $12
		returning id, name, enabled, source_chat_ids, target_chat_id, target_language,
		          content_filter, content_filter_types, summary_time_local, summary_timezone,
		          summary_prompt, summary_knowledge_days, created_at, updated_at
	`,
		strings.TrimSpace(channel.Name),
		channel.Enabled,
		compactInt64s(channel.SourceChatIDs),
		strings.TrimSpace(channel.TargetChatID),
		model.NormalizeSummaryOutputLanguage(channel.TargetLanguage),
		strings.TrimSpace(channel.ContentFilter),
		compactStrings(channel.ContentFilterTypes),
		channel.SummaryTimeLocal,
		channel.SummaryTimezone,
		channel.SummaryPrompt,
		normalizeKnowledgeDays(channel.SummaryKnowledgeDays),
		channel.ID,
	)})
	if err != nil {
		return model.DeliveryChannel{}, fmt.Errorf("save delivery channel %d: %w", channel.ID, err)
	}
	return saved, nil
}

func (r *DeliveryChannelRepository) Create(ctx context.Context, channel model.DeliveryChannel) (model.DeliveryChannel, error) {
	saved, err := scanDeliveryChannel(rowScanner{row: r.pool.QueryRow(ctx, `
		insert into delivery_channels (name, enabled, source_chat_ids, target_chat_id, target_language,
		                               content_filter, content_filter_types, summary_time_local, summary_timezone, summary_prompt, summary_knowledge_days)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		returning id, name, enabled, source_chat_ids, target_chat_id, target_language,
		          content_filter, content_filter_types, summary_time_local, summary_timezone,
		          summary_prompt, summary_knowledge_days, created_at, updated_at
	`,
		strings.TrimSpace(channel.Name),
		channel.Enabled,
		compactInt64s(channel.SourceChatIDs),
		strings.TrimSpace(channel.TargetChatID),
		model.NormalizeSummaryOutputLanguage(channel.TargetLanguage),
		strings.TrimSpace(channel.ContentFilter),
		compactStrings(channel.ContentFilterTypes),
		channel.SummaryTimeLocal,
		channel.SummaryTimezone,
		channel.SummaryPrompt,
		normalizeKnowledgeDays(channel.SummaryKnowledgeDays),
	)})
	if err != nil {
		return model.DeliveryChannel{}, fmt.Errorf("create delivery channel: %w", err)
	}
	return saved, nil
}

func (r *DeliveryChannelRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `
		delete from delivery_channels where id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("delete delivery channel %d: %w", id, err)
	}
	return nil
}

type deliveryChannelScanner interface {
	Scan(dest ...any) error
}

func scanDeliveryChannel(scanner deliveryChannelScanner) (model.DeliveryChannel, error) {
	var channel model.DeliveryChannel
	err := scanner.Scan(
		&channel.ID,
		&channel.Name,
		&channel.Enabled,
		&channel.SourceChatIDs,
		&channel.TargetChatID,
		&channel.TargetLanguage,
		&channel.ContentFilter,
		&channel.ContentFilterTypes,
		&channel.SummaryTimeLocal,
		&channel.SummaryTimezone,
		&channel.SummaryPrompt,
		&channel.SummaryKnowledgeDays,
		&channel.CreatedAt,
		&channel.UpdatedAt,
	)
	if err != nil {
		return model.DeliveryChannel{}, err
	}
	channel.TargetLanguage = model.NormalizeSummaryOutputLanguage(channel.TargetLanguage)
	channel.SourceChatIDs = compactInt64s(channel.SourceChatIDs)
	channel.ContentFilterTypes = compactStrings(channel.ContentFilterTypes)
	channel.SummaryKnowledgeDays = normalizeKnowledgeDays(channel.SummaryKnowledgeDays)
	return channel, nil
}
