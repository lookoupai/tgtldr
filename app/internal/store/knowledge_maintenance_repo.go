package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type KnowledgeMaintenanceEventRepository struct {
	pool *pgxpool.Pool
}

type KnowledgeMaintenanceEventFilter struct {
	FactID  int64
	SpaceID int64
	ChatID  int64
	Limit   int
}

func (r *KnowledgeMaintenanceEventRepository) Create(ctx context.Context, event model.KnowledgeMaintenanceEvent) (model.KnowledgeMaintenanceEvent, error) {
	normalized := normalizeKnowledgeMaintenanceEvent(event)
	saved, err := scanKnowledgeMaintenanceEvent(rowScanner{row: r.pool.QueryRow(ctx, `
		with inserted as (
			insert into knowledge_maintenance_events (
				fact_id, space_id, chat_id, action, source, reason,
				operator_text, matched_query, previous_status, next_status
			) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			returning *
		)
		select e.id, coalesce(e.fact_id, 0), coalesce(f.title, ''), e.space_id,
		       coalesce(s.name, ''), e.chat_id, coalesce(c.title, ''), e.action,
		       e.source, e.reason, e.operator_text, e.matched_query,
		       e.previous_status, e.next_status, e.created_at
		from inserted e
		left join knowledge_facts f on f.id = e.fact_id
		left join knowledge_spaces s on s.id = e.space_id
		left join chats c on c.id = e.chat_id
	`,
		nullablePositiveInt64(normalized.FactID),
		normalized.SpaceID,
		normalized.ChatID,
		normalized.Action,
		normalized.Source,
		normalized.Reason,
		normalized.OperatorText,
		normalized.MatchedQuery,
		normalized.PreviousStatus,
		normalized.NextStatus,
	)})
	if err != nil {
		return model.KnowledgeMaintenanceEvent{}, fmt.Errorf("create knowledge maintenance event: %w", err)
	}
	return saved, nil
}

func (r *KnowledgeMaintenanceEventRepository) List(ctx context.Context, filter KnowledgeMaintenanceEventFilter) ([]model.KnowledgeMaintenanceEvent, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := `
		select e.id, coalesce(e.fact_id, 0), coalesce(f.title, ''), e.space_id,
		       coalesce(s.name, ''), e.chat_id, coalesce(c.title, ''), e.action,
		       e.source, e.reason, e.operator_text, e.matched_query,
		       e.previous_status, e.next_status, e.created_at
		from knowledge_maintenance_events e
		left join knowledge_facts f on f.id = e.fact_id
		left join knowledge_spaces s on s.id = e.space_id
		left join chats c on c.id = e.chat_id
		where 1 = 1
	`
	args := make([]any, 0, 4)
	if filter.FactID > 0 {
		args = append(args, filter.FactID)
		query += fmt.Sprintf(" and e.fact_id = $%d", len(args))
	}
	if filter.SpaceID > 0 {
		args = append(args, filter.SpaceID)
		query += fmt.Sprintf(" and e.space_id = $%d", len(args))
	}
	if filter.ChatID > 0 {
		args = append(args, filter.ChatID)
		query += fmt.Sprintf(" and e.chat_id = $%d", len(args))
	}
	args = append(args, limit)
	query += fmt.Sprintf(" order by e.created_at desc, e.id desc limit $%d", len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query knowledge maintenance events: %w", err)
	}
	defer rows.Close()

	items := make([]model.KnowledgeMaintenanceEvent, 0)
	for rows.Next() {
		item, err := scanKnowledgeMaintenanceEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan knowledge maintenance event: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanKnowledgeMaintenanceEvent(scanner chatScanner) (model.KnowledgeMaintenanceEvent, error) {
	var item model.KnowledgeMaintenanceEvent
	err := scanner.Scan(
		&item.ID,
		&item.FactID,
		&item.FactTitle,
		&item.SpaceID,
		&item.SpaceName,
		&item.ChatID,
		&item.ChatTitle,
		&item.Action,
		&item.Source,
		&item.Reason,
		&item.OperatorText,
		&item.MatchedQuery,
		&item.PreviousStatus,
		&item.NextStatus,
		&item.CreatedAt,
	)
	if err != nil {
		return model.KnowledgeMaintenanceEvent{}, err
	}
	return item, nil
}

func normalizeKnowledgeMaintenanceEvent(event model.KnowledgeMaintenanceEvent) model.KnowledgeMaintenanceEvent {
	event.Action = strings.TrimSpace(event.Action)
	event.Source = strings.TrimSpace(event.Source)
	event.Reason = strings.TrimSpace(event.Reason)
	event.OperatorText = strings.TrimSpace(event.OperatorText)
	event.MatchedQuery = strings.TrimSpace(event.MatchedQuery)
	if event.Source == "" {
		event.Source = "manual"
	}
	return event
}

func nullablePositiveInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}
