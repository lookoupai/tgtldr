package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type KnowledgeSpaceRepository struct {
	pool *pgxpool.Pool
}

func (r *KnowledgeSpaceRepository) List(ctx context.Context) ([]model.KnowledgeSpace, error) {
	rows, err := r.pool.Query(ctx, `
		select id, name, description, enabled, chat_ids, schema_json::text,
		       extract_prompt, summary_prompt, confidence_threshold, retention_days,
		       include_in_summary, created_at, updated_at
		from knowledge_spaces
		order by enabled desc, name asc
	`)
	if err != nil {
		return nil, fmt.Errorf("query knowledge spaces: %w", err)
	}
	defer rows.Close()

	items := make([]model.KnowledgeSpace, 0)
	for rows.Next() {
		item, err := scanKnowledgeSpace(rows)
		if err != nil {
			return nil, fmt.Errorf("scan knowledge space: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *KnowledgeSpaceRepository) GetByID(ctx context.Context, id int64) (model.KnowledgeSpace, error) {
	item, err := scanKnowledgeSpace(rowScanner{row: r.pool.QueryRow(ctx, `
		select id, name, description, enabled, chat_ids, schema_json::text,
		       extract_prompt, summary_prompt, confidence_threshold, retention_days,
		       include_in_summary, created_at, updated_at
		from knowledge_spaces
		where id = $1
	`, id)})
	if err != nil {
		return model.KnowledgeSpace{}, fmt.Errorf("get knowledge space %d: %w", id, err)
	}
	return item, nil
}

func (r *KnowledgeSpaceRepository) Create(ctx context.Context, item model.KnowledgeSpace) (model.KnowledgeSpace, error) {
	normalized, err := normalizeKnowledgeSpace(item)
	if err != nil {
		return model.KnowledgeSpace{}, err
	}
	saved, err := scanKnowledgeSpace(rowScanner{row: r.pool.QueryRow(ctx, `
		insert into knowledge_spaces (
			name, description, enabled, chat_ids, schema_json, extract_prompt,
			summary_prompt, confidence_threshold, retention_days, include_in_summary
		) values ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10)
		returning id, name, description, enabled, chat_ids, schema_json::text,
		          extract_prompt, summary_prompt, confidence_threshold, retention_days,
		          include_in_summary, created_at, updated_at
	`,
		normalized.Name,
		normalized.Description,
		normalized.Enabled,
		normalized.ChatIDs,
		normalized.SchemaJSON,
		normalized.ExtractPrompt,
		normalized.SummaryPrompt,
		normalized.ConfidenceThreshold,
		normalized.RetentionDays,
		normalized.IncludeInSummary,
	)})
	if err != nil {
		return model.KnowledgeSpace{}, fmt.Errorf("create knowledge space: %w", err)
	}
	return saved, nil
}

func (r *KnowledgeSpaceRepository) Save(ctx context.Context, item model.KnowledgeSpace) (model.KnowledgeSpace, error) {
	normalized, err := normalizeKnowledgeSpace(item)
	if err != nil {
		return model.KnowledgeSpace{}, err
	}
	saved, err := scanKnowledgeSpace(rowScanner{row: r.pool.QueryRow(ctx, `
		update knowledge_spaces
		set name = $1,
		    description = $2,
		    enabled = $3,
		    chat_ids = $4,
		    schema_json = $5::jsonb,
		    extract_prompt = $6,
		    summary_prompt = $7,
		    confidence_threshold = $8,
		    retention_days = $9,
		    include_in_summary = $10,
		    updated_at = now()
		where id = $11
		returning id, name, description, enabled, chat_ids, schema_json::text,
		          extract_prompt, summary_prompt, confidence_threshold, retention_days,
		          include_in_summary, created_at, updated_at
	`,
		normalized.Name,
		normalized.Description,
		normalized.Enabled,
		normalized.ChatIDs,
		normalized.SchemaJSON,
		normalized.ExtractPrompt,
		normalized.SummaryPrompt,
		normalized.ConfidenceThreshold,
		normalized.RetentionDays,
		normalized.IncludeInSummary,
		normalized.ID,
	)})
	if err != nil {
		return model.KnowledgeSpace{}, fmt.Errorf("save knowledge space %d: %w", item.ID, err)
	}
	return saved, nil
}

type KnowledgeFactRepository struct {
	pool *pgxpool.Pool
}

type KnowledgeFactFilter struct {
	SpaceID  int64
	ChatID   int64
	Status   model.KnowledgeFactStatus
	FactType string
	Query    string
	Limit    int
}

type KnowledgeSubjectFilter struct {
	SpaceID  int64
	ChatID   int64
	FactType string
	Query    string
	Limit    int
}

func (r *KnowledgeFactRepository) List(ctx context.Context, filter KnowledgeFactFilter) ([]model.KnowledgeFact, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	query := `
		select f.id, f.space_id, f.chat_id, coalesce(c.title, ''), f.fact_type, f.title,
		       f.data_json::text, f.subject_sender_id, f.subject_sender_name,
		       f.subject_username, f.confidence, f.status, f.source_message_ids,
		       f.first_seen_at, f.last_seen_at, f.expires_at, f.created_at, f.updated_at
		from knowledge_facts f
		left join chats c on c.id = f.chat_id
		where 1 = 1
	`
	args := make([]any, 0, 8)
	if filter.SpaceID > 0 {
		args = append(args, filter.SpaceID)
		query += fmt.Sprintf(" and f.space_id = $%d", len(args))
	}
	if filter.ChatID > 0 {
		args = append(args, filter.ChatID)
		query += fmt.Sprintf(" and f.chat_id = $%d", len(args))
	}
	if strings.TrimSpace(string(filter.Status)) != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(" and f.status = $%d", len(args))
	}
	if factType := strings.TrimSpace(filter.FactType); factType != "" {
		args = append(args, factType)
		query += fmt.Sprintf(" and lower(f.fact_type) = lower($%d)", len(args))
	}
	for _, term := range searchTerms(strings.TrimSpace(filter.Query)) {
		args = append(args, "%"+term+"%")
		index := len(args)
		query += fmt.Sprintf(
			" and (f.title ilike $%d or f.fact_type ilike $%d or f.data_json::text ilike $%d or f.subject_sender_name ilike $%d or f.subject_username ilike $%d or c.title ilike $%d)",
			index,
			index,
			index,
			index,
			index,
			index,
		)
	}
	args = append(args, limit)
	query += fmt.Sprintf(" order by f.updated_at desc limit $%d", len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query knowledge facts: %w", err)
	}
	defer rows.Close()

	items := make([]model.KnowledgeFact, 0)
	for rows.Next() {
		item, err := scanKnowledgeFact(rows)
		if err != nil {
			return nil, fmt.Errorf("scan knowledge fact: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *KnowledgeFactRepository) Create(ctx context.Context, fact model.KnowledgeFact) (model.KnowledgeFact, error) {
	if fact.FirstSeenAt.IsZero() {
		fact.FirstSeenAt = time.Now()
	}
	if fact.LastSeenAt.IsZero() {
		fact.LastSeenAt = fact.FirstSeenAt
	}
	normalized := normalizeKnowledgeFactForUpsert(fact)
	if !json.Valid([]byte(normalized.DataJSON)) {
		return model.KnowledgeFact{}, fmt.Errorf("knowledge fact data must be valid JSON")
	}
	saved, err := scanKnowledgeFact(rowScanner{row: r.pool.QueryRow(ctx, `
		with inserted as (
			insert into knowledge_facts (
				space_id, chat_id, fact_type, title, data_json, subject_sender_id,
				subject_sender_name, subject_username, confidence, status,
				source_message_ids, first_seen_at, last_seen_at, expires_at
			) values ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			returning *
		)
		select f.id, f.space_id, f.chat_id, coalesce(c.title, ''), f.fact_type, f.title,
		       f.data_json::text, f.subject_sender_id, f.subject_sender_name,
		       f.subject_username, f.confidence, f.status, f.source_message_ids,
		       f.first_seen_at, f.last_seen_at, f.expires_at, f.created_at, f.updated_at
		from inserted f
		left join chats c on c.id = f.chat_id
	`,
		normalized.SpaceID,
		normalized.ChatID,
		normalized.FactType,
		normalized.Title,
		normalized.DataJSON,
		normalized.SubjectSenderID,
		normalized.SubjectSenderName,
		normalized.SubjectUsername,
		normalized.Confidence,
		normalized.Status,
		normalized.SourceMessageIDs,
		normalized.FirstSeenAt,
		normalized.LastSeenAt,
		normalized.ExpiresAt,
	)})
	if err != nil {
		return model.KnowledgeFact{}, fmt.Errorf("create knowledge fact: %w", err)
	}
	return saved, nil
}

func (r *KnowledgeFactRepository) ListSubjects(ctx context.Context, filter KnowledgeSubjectFilter) ([]model.KnowledgeSubject, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	facts, err := r.List(ctx, KnowledgeFactFilter{
		SpaceID:  filter.SpaceID,
		ChatID:   filter.ChatID,
		Status:   model.KnowledgeFactStatusActive,
		FactType: filter.FactType,
		Query:    filter.Query,
		Limit:    200,
	})
	if err != nil {
		return nil, err
	}

	return groupKnowledgeSubjects(facts, limit), nil
}

func groupKnowledgeSubjects(facts []model.KnowledgeFact, limit int) []model.KnowledgeSubject {
	type accumulator struct {
		subject model.KnowledgeSubject
		typeSet map[string]struct{}
		chatSet map[string]struct{}
	}

	byKey := make(map[string]*accumulator)
	for _, fact := range facts {
		key := knowledgeSubjectKey(fact)
		if key == "" {
			continue
		}
		current, ok := byKey[key]
		if !ok {
			current = &accumulator{
				subject: model.KnowledgeSubject{
					Key:               key,
					DisplayName:       knowledgeSubjectDisplayName(fact),
					SubjectSenderID:   fact.SubjectSenderID,
					SubjectSenderName: fact.SubjectSenderName,
					SubjectUsername:   fact.SubjectUsername,
					LastSeenAt:        fact.LastSeenAt,
				},
				typeSet: make(map[string]struct{}),
				chatSet: make(map[string]struct{}),
			}
			byKey[key] = current
		}

		current.subject.FactCount++
		current.subject.Facts = append(current.subject.Facts, fact)
		if fact.LastSeenAt.After(current.subject.LastSeenAt) {
			current.subject.LastSeenAt = fact.LastSeenAt
		}
		if fact.FactType != "" {
			current.typeSet[fact.FactType] = struct{}{}
		}
		if fact.ChatTitle != "" {
			current.chatSet[fact.ChatTitle] = struct{}{}
		}
	}

	items := make([]model.KnowledgeSubject, 0, len(byKey))
	for _, current := range byKey {
		current.subject.FactTypes = sortedMapKeys(current.typeSet)
		current.subject.ChatTitles = sortedMapKeys(current.chatSet)
		items = append(items, current.subject)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].LastSeenAt.Equal(items[j].LastSeenAt) {
			if items[i].FactCount == items[j].FactCount {
				return strings.ToLower(items[i].DisplayName) < strings.ToLower(items[j].DisplayName)
			}
			return items[i].FactCount > items[j].FactCount
		}
		return items[i].LastSeenAt.After(items[j].LastSeenAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (r *KnowledgeFactRepository) GetByID(ctx context.Context, id int64) (model.KnowledgeFact, error) {
	item, err := scanKnowledgeFact(rowScanner{row: r.pool.QueryRow(ctx, `
		select f.id, f.space_id, f.chat_id, coalesce(c.title, ''), f.fact_type, f.title,
		       f.data_json::text, f.subject_sender_id, f.subject_sender_name,
		       f.subject_username, f.confidence, f.status, f.source_message_ids,
		       f.first_seen_at, f.last_seen_at, f.expires_at, f.created_at, f.updated_at
		from knowledge_facts f
		left join chats c on c.id = f.chat_id
		where f.id = $1
	`, id)})
	if err != nil {
		return model.KnowledgeFact{}, fmt.Errorf("get knowledge fact %d: %w", id, err)
	}
	return item, nil
}

func (r *KnowledgeFactRepository) UpdateStatus(ctx context.Context, id int64, status model.KnowledgeFactStatus) (model.KnowledgeFact, error) {
	item, err := scanKnowledgeFact(rowScanner{row: r.pool.QueryRow(ctx, `
		with updated as (
			update knowledge_facts
			set status = $1,
			    updated_at = now()
			where id = $2
			returning *
		)
		select f.id, f.space_id, f.chat_id, coalesce(c.title, ''), f.fact_type, f.title,
		       f.data_json::text, f.subject_sender_id, f.subject_sender_name,
		       f.subject_username, f.confidence, f.status, f.source_message_ids,
		       f.first_seen_at, f.last_seen_at, f.expires_at, f.created_at, f.updated_at
		from updated f
		left join chats c on c.id = f.chat_id
	`, status, id)})
	if err != nil {
		return model.KnowledgeFact{}, fmt.Errorf("update knowledge fact %d status: %w", id, err)
	}
	return item, nil
}

func (r *KnowledgeFactRepository) ExpireDue(ctx context.Context, now time.Time) error {
	_, err := r.pool.Exec(ctx, `
		update knowledge_facts
		set status = $1,
		    updated_at = now()
		where status = $2
		  and expires_at is not null
		  and expires_at <= $3
	`, model.KnowledgeFactStatusExpired, model.KnowledgeFactStatusActive, now)
	if err != nil {
		return fmt.Errorf("expire due knowledge facts: %w", err)
	}
	return nil
}

func (r *KnowledgeFactRepository) ListForSummary(ctx context.Context, chatID int64, now time.Time) ([]model.KnowledgeFact, error) {
	rows, err := r.pool.Query(ctx, `
		select f.id, f.space_id, s.name, f.chat_id, coalesce(c.title, ''), f.fact_type, f.title,
		       f.data_json::text, f.subject_sender_id, f.subject_sender_name,
		       f.subject_username, f.confidence, f.status, f.source_message_ids,
		       f.first_seen_at, f.last_seen_at, f.expires_at, f.created_at, f.updated_at
		from knowledge_facts f
		join knowledge_spaces s on s.id = f.space_id
		left join chats c on c.id = f.chat_id
		where f.chat_id = $1
		  and f.status = $2
		  and (f.expires_at is null or f.expires_at > $3)
		  and s.enabled = true
		  and s.include_in_summary = true
		  and (cardinality(s.chat_ids) = 0 or $1 = any(s.chat_ids))
		order by lower(s.name) asc, lower(f.fact_type) asc, f.confidence desc, f.last_seen_at desc, f.id desc
	`, chatID, model.KnowledgeFactStatusActive, now)
	if err != nil {
		return nil, fmt.Errorf("query summary knowledge facts: %w", err)
	}
	defer rows.Close()

	items := make([]model.KnowledgeFact, 0)
	for rows.Next() {
		item, err := scanKnowledgeFactWithSpace(rows)
		if err != nil {
			return nil, fmt.Errorf("scan summary knowledge fact: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type KnowledgeRunRepository struct {
	pool *pgxpool.Pool
}

type KnowledgeRunFilter struct {
	SpaceID int64
	ChatID  int64
	Limit   int
}

func (r *KnowledgeFactRepository) UpsertMany(ctx context.Context, facts []model.KnowledgeFact) error {
	if len(facts) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin knowledge facts tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, fact := range facts {
		normalized := normalizeKnowledgeFactForUpsert(fact)
		if !json.Valid([]byte(normalized.DataJSON)) {
			return fmt.Errorf("knowledge fact data must be valid JSON")
		}

		existing, err := findMergeableKnowledgeFact(ctx, tx, normalized)
		if err != nil {
			return err
		}
		if existing.ID > 0 {
			merged := mergeKnowledgeFacts(existing, normalized)
			if err := updateKnowledgeFact(ctx, tx, merged); err != nil {
				return err
			}
			continue
		}

		_, err = tx.Exec(ctx, `
			insert into knowledge_facts (
				space_id, chat_id, fact_type, title, data_json, subject_sender_id,
				subject_sender_name, subject_username, confidence, status,
				source_message_ids, first_seen_at, last_seen_at, expires_at
			) values ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			on conflict (space_id, chat_id, fact_type, title, subject_sender_id, source_message_ids)
			do update set
				data_json = excluded.data_json,
				subject_sender_name = excluded.subject_sender_name,
				subject_username = excluded.subject_username,
				confidence = excluded.confidence,
				status = case
					when knowledge_facts.status = 'dismissed' then knowledge_facts.status
					else excluded.status
				end,
				last_seen_at = excluded.last_seen_at,
				expires_at = excluded.expires_at,
				updated_at = now()
		`,
			normalized.SpaceID,
			normalized.ChatID,
			normalized.FactType,
			normalized.Title,
			normalized.DataJSON,
			normalized.SubjectSenderID,
			normalized.SubjectSenderName,
			normalized.SubjectUsername,
			normalized.Confidence,
			normalized.Status,
			normalized.SourceMessageIDs,
			normalized.FirstSeenAt,
			normalized.LastSeenAt,
			normalized.ExpiresAt,
		)
		if err != nil {
			return fmt.Errorf("upsert knowledge fact: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit knowledge facts tx: %w", err)
	}
	return nil
}

func findMergeableKnowledgeFact(ctx context.Context, tx pgx.Tx, fact model.KnowledgeFact) (model.KnowledgeFact, error) {
	var existing model.KnowledgeFact
	var sourceMessageIDs []int32
	err := tx.QueryRow(ctx, `
		select id, source_message_ids, first_seen_at, last_seen_at, expires_at,
		       confidence, status, subject_sender_name, subject_username
		from knowledge_facts
		where space_id = $1
		  and chat_id = $2
		  and lower(fact_type) = lower($3)
		  and lower(title) = lower($4)
		  and subject_sender_id = $5
		order by case
			when status = 'active' then 0
			when status = 'dismissed' then 1
			else 2
		end, updated_at desc, id desc
		limit 1
		for update
	`,
		fact.SpaceID,
		fact.ChatID,
		fact.FactType,
		fact.Title,
		fact.SubjectSenderID,
	).Scan(
		&existing.ID,
		&sourceMessageIDs,
		&existing.FirstSeenAt,
		&existing.LastSeenAt,
		&existing.ExpiresAt,
		&existing.Confidence,
		&existing.Status,
		&existing.SubjectSenderName,
		&existing.SubjectUsername,
	)
	if err == pgx.ErrNoRows {
		return model.KnowledgeFact{}, nil
	}
	if err != nil {
		return model.KnowledgeFact{}, fmt.Errorf("find mergeable knowledge fact: %w", err)
	}
	existing.SourceMessageIDs = int32sToInts(sourceMessageIDs)
	return existing, nil
}

func updateKnowledgeFact(ctx context.Context, tx pgx.Tx, fact model.KnowledgeFact) error {
	_, err := tx.Exec(ctx, `
		update knowledge_facts
		set data_json = $2::jsonb,
		    subject_sender_name = $3,
		    subject_username = $4,
		    confidence = $5,
		    status = case
		        when status = 'dismissed' then status
		        else $6
		    end,
		    source_message_ids = $7,
		    first_seen_at = $8,
		    last_seen_at = $9,
		    expires_at = $10,
		    updated_at = now()
		where id = $1
	`,
		fact.ID,
		fact.DataJSON,
		fact.SubjectSenderName,
		fact.SubjectUsername,
		fact.Confidence,
		fact.Status,
		fact.SourceMessageIDs,
		fact.FirstSeenAt,
		fact.LastSeenAt,
		fact.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("merge knowledge fact: %w", err)
	}
	return nil
}

func normalizeKnowledgeFactForUpsert(fact model.KnowledgeFact) model.KnowledgeFact {
	fact.FactType = strings.TrimSpace(fact.FactType)
	fact.Title = strings.TrimSpace(fact.Title)
	fact.SubjectSenderName = strings.TrimSpace(fact.SubjectSenderName)
	fact.SubjectUsername = strings.TrimSpace(fact.SubjectUsername)
	if fact.DataJSON = strings.TrimSpace(fact.DataJSON); fact.DataJSON == "" {
		fact.DataJSON = "{}"
	}
	if fact.Status == "" {
		fact.Status = model.KnowledgeFactStatusActive
	}
	if fact.FirstSeenAt.IsZero() {
		fact.FirstSeenAt = fact.LastSeenAt
	}
	if fact.LastSeenAt.IsZero() {
		fact.LastSeenAt = fact.FirstSeenAt
	}
	fact.SourceMessageIDs = compactInts(fact.SourceMessageIDs)
	return fact
}

func mergeKnowledgeFacts(existing model.KnowledgeFact, incoming model.KnowledgeFact) model.KnowledgeFact {
	merged := incoming
	merged.ID = existing.ID
	merged.SourceMessageIDs = mergeInts(existing.SourceMessageIDs, incoming.SourceMessageIDs)
	merged.FirstSeenAt = earlierTime(existing.FirstSeenAt, incoming.FirstSeenAt)
	merged.LastSeenAt = laterTime(existing.LastSeenAt, incoming.LastSeenAt)
	if existing.Confidence > incoming.Confidence {
		merged.Confidence = existing.Confidence
	}
	if strings.TrimSpace(merged.SubjectSenderName) == "" {
		merged.SubjectSenderName = existing.SubjectSenderName
	}
	if strings.TrimSpace(merged.SubjectUsername) == "" {
		merged.SubjectUsername = existing.SubjectUsername
	}
	if existing.Status == model.KnowledgeFactStatusDismissed {
		merged.Status = model.KnowledgeFactStatusDismissed
	}
	merged.ExpiresAt = laterOptionalTime(existing.ExpiresAt, incoming.ExpiresAt)
	return merged
}

func earlierTime(a time.Time, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() || a.Before(b) {
		return a
	}
	return b
}

func laterTime(a time.Time, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() || a.After(b) {
		return a
	}
	return b
}

func laterOptionalTime(a *time.Time, b *time.Time) *time.Time {
	if a == nil || b == nil {
		return nil
	}
	if a.After(*b) {
		return a
	}
	return b
}

func (r *KnowledgeRunRepository) Create(ctx context.Context, run model.KnowledgeRun) (model.KnowledgeRun, error) {
	saved, err := scanKnowledgeRun(rowScanner{row: r.pool.QueryRow(ctx, `
		insert into knowledge_runs (
			space_id, chat_id, range_start, range_end, status, input_message_count,
			extracted_count, error_message, started_at, finished_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		returning id, space_id, chat_id, range_start, range_end, status,
		          input_message_count, extracted_count, error_message,
		          started_at, finished_at, created_at, updated_at
	`,
		run.SpaceID,
		run.ChatID,
		run.RangeStart,
		run.RangeEnd,
		run.Status,
		run.InputMessageCount,
		run.ExtractedCount,
		run.ErrorMessage,
		run.StartedAt,
		run.FinishedAt,
	)})
	if err != nil {
		return model.KnowledgeRun{}, fmt.Errorf("create knowledge run: %w", err)
	}
	return saved, nil
}

func (r *KnowledgeRunRepository) List(ctx context.Context, filter KnowledgeRunFilter) ([]model.KnowledgeRun, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := `
		select id, space_id, chat_id, range_start, range_end, status,
		       input_message_count, extracted_count, error_message,
		       started_at, finished_at, created_at, updated_at
		from knowledge_runs
		where 1 = 1
	`
	args := make([]any, 0, 3)
	if filter.SpaceID > 0 {
		args = append(args, filter.SpaceID)
		query += fmt.Sprintf(" and space_id = $%d", len(args))
	}
	if filter.ChatID > 0 {
		args = append(args, filter.ChatID)
		query += fmt.Sprintf(" and chat_id = $%d", len(args))
	}
	args = append(args, limit)
	query += fmt.Sprintf(" order by created_at desc, id desc limit $%d", len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query knowledge runs: %w", err)
	}
	defer rows.Close()

	items := make([]model.KnowledgeRun, 0)
	for rows.Next() {
		run, err := scanKnowledgeRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan knowledge run: %w", err)
		}
		items = append(items, run)
	}
	return items, rows.Err()
}

func (r *KnowledgeRunRepository) Finish(ctx context.Context, id int64, status model.KnowledgeRunStatus, inputCount int, extractedCount int, errorMessage string, finishedAt time.Time) (model.KnowledgeRun, error) {
	saved, err := scanKnowledgeRun(rowScanner{row: r.pool.QueryRow(ctx, `
		update knowledge_runs
		set status = $1,
		    input_message_count = $2,
		    extracted_count = $3,
		    error_message = $4,
		    finished_at = $5,
		    updated_at = now()
		where id = $6
		returning id, space_id, chat_id, range_start, range_end, status,
		          input_message_count, extracted_count, error_message,
		          started_at, finished_at, created_at, updated_at
	`, status, inputCount, extractedCount, strings.TrimSpace(errorMessage), finishedAt, id)})
	if err != nil {
		return model.KnowledgeRun{}, fmt.Errorf("finish knowledge run %d: %w", id, err)
	}
	return saved, nil
}

func scanKnowledgeSpace(scanner chatScanner) (model.KnowledgeSpace, error) {
	var item model.KnowledgeSpace
	err := scanner.Scan(
		&item.ID,
		&item.Name,
		&item.Description,
		&item.Enabled,
		&item.ChatIDs,
		&item.SchemaJSON,
		&item.ExtractPrompt,
		&item.SummaryPrompt,
		&item.ConfidenceThreshold,
		&item.RetentionDays,
		&item.IncludeInSummary,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return model.KnowledgeSpace{}, err
	}
	item, err = normalizeKnowledgeSpace(item)
	if err != nil {
		return model.KnowledgeSpace{}, err
	}
	return item, nil
}

func scanKnowledgeRun(scanner chatScanner) (model.KnowledgeRun, error) {
	var run model.KnowledgeRun
	err := scanner.Scan(
		&run.ID,
		&run.SpaceID,
		&run.ChatID,
		&run.RangeStart,
		&run.RangeEnd,
		&run.Status,
		&run.InputMessageCount,
		&run.ExtractedCount,
		&run.ErrorMessage,
		&run.StartedAt,
		&run.FinishedAt,
		&run.CreatedAt,
		&run.UpdatedAt,
	)
	if err != nil {
		return model.KnowledgeRun{}, err
	}
	return run, nil
}

func scanKnowledgeFact(scanner chatScanner) (model.KnowledgeFact, error) {
	var item model.KnowledgeFact
	var sourceMessageIDs []int32
	err := scanner.Scan(
		&item.ID,
		&item.SpaceID,
		&item.ChatID,
		&item.ChatTitle,
		&item.FactType,
		&item.Title,
		&item.DataJSON,
		&item.SubjectSenderID,
		&item.SubjectSenderName,
		&item.SubjectUsername,
		&item.Confidence,
		&item.Status,
		&sourceMessageIDs,
		&item.FirstSeenAt,
		&item.LastSeenAt,
		&item.ExpiresAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return model.KnowledgeFact{}, err
	}
	item.SourceMessageIDs = int32sToInts(sourceMessageIDs)
	return item, nil
}

func scanKnowledgeFactWithSpace(scanner chatScanner) (model.KnowledgeFact, error) {
	var item model.KnowledgeFact
	var sourceMessageIDs []int32
	err := scanner.Scan(
		&item.ID,
		&item.SpaceID,
		&item.SpaceName,
		&item.ChatID,
		&item.ChatTitle,
		&item.FactType,
		&item.Title,
		&item.DataJSON,
		&item.SubjectSenderID,
		&item.SubjectSenderName,
		&item.SubjectUsername,
		&item.Confidence,
		&item.Status,
		&sourceMessageIDs,
		&item.FirstSeenAt,
		&item.LastSeenAt,
		&item.ExpiresAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return model.KnowledgeFact{}, err
	}
	item.SourceMessageIDs = int32sToInts(sourceMessageIDs)
	return item, nil
}

func normalizeKnowledgeSpace(item model.KnowledgeSpace) (model.KnowledgeSpace, error) {
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	item.ExtractPrompt = strings.TrimSpace(item.ExtractPrompt)
	item.SummaryPrompt = strings.TrimSpace(item.SummaryPrompt)
	if item.SchemaJSON = strings.TrimSpace(item.SchemaJSON); item.SchemaJSON == "" {
		item.SchemaJSON = "{}"
	}
	if !json.Valid([]byte(item.SchemaJSON)) {
		return model.KnowledgeSpace{}, fmt.Errorf("schema_json must be valid JSON")
	}
	if item.ConfidenceThreshold <= 0 {
		item.ConfidenceThreshold = 0.75
	}
	if item.ConfidenceThreshold > 1 {
		item.ConfidenceThreshold = 1
	}
	if item.RetentionDays <= 0 {
		item.RetentionDays = 30
	}
	item.ChatIDs = compactInt64s(item.ChatIDs)
	return item, nil
}

func knowledgeSubjectKey(fact model.KnowledgeFact) string {
	if fact.SubjectSenderID > 0 {
		return fmt.Sprintf("id:%d", fact.SubjectSenderID)
	}
	if username := strings.TrimSpace(fact.SubjectUsername); username != "" {
		return "username:" + strings.ToLower(username)
	}
	if name := strings.TrimSpace(fact.SubjectSenderName); name != "" {
		return "name:" + strings.ToLower(name)
	}
	return ""
}

func knowledgeSubjectDisplayName(fact model.KnowledgeFact) string {
	if username := strings.TrimSpace(fact.SubjectUsername); username != "" {
		return "@" + username
	}
	if name := strings.TrimSpace(fact.SubjectSenderName); name != "" {
		return name
	}
	if fact.SubjectSenderID > 0 {
		return fmt.Sprintf("%d", fact.SubjectSenderID)
	}
	return ""
}

func sortedMapKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func compactInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func compactInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	out := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func mergeInts(a []int, b []int) []int {
	values := make([]int, 0, len(a)+len(b))
	values = append(values, a...)
	values = append(values, b...)
	return compactInts(values)
}

func int32sToInts(values []int32) []int {
	out := make([]int, 0, len(values))
	for _, value := range values {
		out = append(out, int(value))
	}
	return out
}
