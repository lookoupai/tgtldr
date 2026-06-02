package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LLMWikiRepository struct {
	pool *pgxpool.Pool
}

func (r *LLMWikiRepository) CreateRun(ctx context.Context, run model.LLMWikiRun) (model.LLMWikiRun, error) {
	saved, err := scanLLMWikiRun(rowScanner{row: r.pool.QueryRow(ctx, `
		insert into llm_wiki_runs (
			space_id, chat_id, summary_id, range_start, range_end, status,
			updated_page_count, error_message, started_at, finished_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		returning id, space_id, chat_id, summary_id, range_start, range_end, status,
		          updated_page_count, error_message, started_at, finished_at,
		          created_at, updated_at
	`,
		run.SpaceID,
		run.ChatID,
		run.SummaryID,
		run.RangeStart,
		run.RangeEnd,
		run.Status,
		run.UpdatedPageCount,
		strings.TrimSpace(run.ErrorMessage),
		run.StartedAt,
		run.FinishedAt,
	)})
	if err != nil {
		return model.LLMWikiRun{}, fmt.Errorf("create llm wiki run: %w", err)
	}
	return saved, nil
}

func (r *LLMWikiRepository) FinishRun(ctx context.Context, id int64, status model.LLMWikiRunStatus, updatedPageCount int, errorMessage string, finishedAt time.Time) (model.LLMWikiRun, error) {
	saved, err := scanLLMWikiRun(rowScanner{row: r.pool.QueryRow(ctx, `
		update llm_wiki_runs
		set status = $1,
		    updated_page_count = $2,
		    error_message = $3,
		    finished_at = $4,
		    updated_at = now()
		where id = $5
		returning id, space_id, chat_id, summary_id, range_start, range_end, status,
		          updated_page_count, error_message, started_at, finished_at,
		          created_at, updated_at
	`, status, updatedPageCount, strings.TrimSpace(errorMessage), finishedAt, id)})
	if err != nil {
		return model.LLMWikiRun{}, fmt.Errorf("finish llm wiki run %d: %w", id, err)
	}
	return saved, nil
}

type LLMWikiRunFilter struct {
	SpaceID int64
	ChatID  int64
	Limit   int
}

func (r *LLMWikiRepository) ListRuns(ctx context.Context, filter LLMWikiRunFilter) ([]model.LLMWikiRun, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	query := `
		select id, space_id, chat_id, summary_id, range_start, range_end, status,
		       updated_page_count, error_message, started_at, finished_at,
		       created_at, updated_at
		from llm_wiki_runs
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
		return nil, fmt.Errorf("query llm wiki runs: %w", err)
	}
	defer rows.Close()

	items := make([]model.LLMWikiRun, 0)
	for rows.Next() {
		run, err := scanLLMWikiRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan llm wiki run: %w", err)
		}
		items = append(items, run)
	}
	return items, rows.Err()
}

type LLMWikiPageFilter struct {
	Query    string
	SpaceID  int64
	PageType string
	Page     int
	PageSize int
}

func (r *LLMWikiRepository) ReindexPages(ctx context.Context, pages []model.LLMWikiPage) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin llm wiki reindex: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `delete from llm_wiki_pages`); err != nil {
		return fmt.Errorf("clear llm wiki pages: %w", err)
	}
	for _, page := range pages {
		if err := insertLLMWikiPage(ctx, tx, normalizeLLMWikiPage(page)); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit llm wiki reindex: %w", err)
	}
	return nil
}

func insertLLMWikiPage(ctx context.Context, tx pgx.Tx, page model.LLMWikiPage) error {
	refs, err := json.Marshal(page.SourceMessageRefs)
	if err != nil {
		return fmt.Errorf("marshal llm wiki source refs: %w", err)
	}
	_, err = tx.Exec(ctx, `
		insert into llm_wiki_pages (
			space_id, path, title, page_type, content_hash, content_text,
			source_fact_ids, source_message_refs, updated_at, indexed_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, now())
	`,
		page.SpaceID,
		page.Path,
		page.Title,
		page.PageType,
		page.ContentHash,
		page.ContentText,
		page.SourceFactIDs,
		refs,
		page.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert llm wiki page %s: %w", page.Path, err)
	}
	return nil
}

func (r *LLMWikiRepository) SearchPages(ctx context.Context, filter LLMWikiPageFilter) (model.LLMWikiPageListResponse, error) {
	normalized := normalizeLLMWikiPageFilter(filter)
	terms := searchTerms(normalized.Query)
	whereClause, args := buildLLMWikiPageWhereClause(normalized, terms)

	var total int
	if err := r.pool.QueryRow(ctx, `select count(*) from llm_wiki_pages`+whereClause, args...).Scan(&total); err != nil {
		return model.LLMWikiPageListResponse{}, fmt.Errorf("count llm wiki pages: %w", err)
	}

	offset := (normalized.Page - 1) * normalized.PageSize
	argsWithPagination := append(args, normalized.PageSize, offset)
	rows, err := r.pool.Query(ctx, `
		select id, space_id, path, title, page_type, content_hash, content_text,
		       source_fact_ids, source_message_refs::text, created_at, updated_at, indexed_at
		from llm_wiki_pages
	`+whereClause+`
		order by updated_at desc, id desc
		limit $`+fmt.Sprint(len(args)+1)+` offset $`+fmt.Sprint(len(args)+2),
		argsWithPagination...,
	)
	if err != nil {
		return model.LLMWikiPageListResponse{}, fmt.Errorf("query llm wiki pages: %w", err)
	}
	defer rows.Close()

	items := make([]model.LLMWikiPage, 0)
	for rows.Next() {
		page, err := scanLLMWikiPage(rows)
		if err != nil {
			return model.LLMWikiPageListResponse{}, fmt.Errorf("scan llm wiki page: %w", err)
		}
		items = append(items, page)
	}
	if err := rows.Err(); err != nil {
		return model.LLMWikiPageListResponse{}, fmt.Errorf("iterate llm wiki pages: %w", err)
	}
	return model.LLMWikiPageListResponse{
		Items:    items,
		Total:    total,
		Page:     normalized.Page,
		PageSize: normalized.PageSize,
	}, nil
}

func (r *LLMWikiRepository) GetPageByID(ctx context.Context, id int64) (model.LLMWikiPage, error) {
	page, err := scanLLMWikiPage(rowScanner{row: r.pool.QueryRow(ctx, `
		select id, space_id, path, title, page_type, content_hash, content_text,
		       source_fact_ids, source_message_refs::text, created_at, updated_at, indexed_at
		from llm_wiki_pages
		where id = $1
	`, id)})
	if err != nil {
		return model.LLMWikiPage{}, fmt.Errorf("get llm wiki page %d: %w", id, err)
	}
	return page, nil
}

func normalizeLLMWikiPageFilter(filter LLMWikiPageFilter) LLMWikiPageFilter {
	filter.Query = strings.TrimSpace(filter.Query)
	filter.PageType = strings.TrimSpace(filter.PageType)
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}
	return filter
}

func buildLLMWikiPageWhereClause(filter LLMWikiPageFilter, terms []string) (string, []any) {
	clauses := make([]string, 0, len(terms)+2)
	args := make([]any, 0, len(terms)+2)
	if filter.SpaceID > 0 {
		args = append(args, filter.SpaceID)
		clauses = append(clauses, fmt.Sprintf("space_id = $%d", len(args)))
	}
	if filter.PageType != "" {
		args = append(args, filter.PageType)
		clauses = append(clauses, fmt.Sprintf("page_type = $%d", len(args)))
	}
	for _, term := range terms {
		args = append(args, "%"+term+"%")
		index := len(args)
		clauses = append(clauses, fmt.Sprintf(
			"(title ilike $%d or path ilike $%d or content_text ilike $%d)",
			index,
			index,
			index,
		))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " where " + strings.Join(clauses, " and "), args
}

func normalizeLLMWikiPage(page model.LLMWikiPage) model.LLMWikiPage {
	page.Path = strings.TrimSpace(page.Path)
	page.Title = strings.TrimSpace(page.Title)
	page.PageType = strings.TrimSpace(page.PageType)
	page.ContentHash = strings.TrimSpace(page.ContentHash)
	if page.PageType == "" {
		page.PageType = "page"
	}
	if page.Title == "" {
		page.Title = page.Path
	}
	page.SourceFactIDs = compactInt64s(page.SourceFactIDs)
	return page
}

func scanLLMWikiPage(scanner chatScanner) (model.LLMWikiPage, error) {
	var page model.LLMWikiPage
	var refsJSON string
	if err := scanner.Scan(
		&page.ID,
		&page.SpaceID,
		&page.Path,
		&page.Title,
		&page.PageType,
		&page.ContentHash,
		&page.ContentText,
		&page.SourceFactIDs,
		&refsJSON,
		&page.CreatedAt,
		&page.UpdatedAt,
		&page.IndexedAt,
	); err != nil {
		return model.LLMWikiPage{}, err
	}
	if strings.TrimSpace(refsJSON) != "" {
		if err := json.Unmarshal([]byte(refsJSON), &page.SourceMessageRefs); err != nil {
			return model.LLMWikiPage{}, fmt.Errorf("decode llm wiki source refs: %w", err)
		}
	}
	return page, nil
}

func scanLLMWikiRun(scanner chatScanner) (model.LLMWikiRun, error) {
	var run model.LLMWikiRun
	if err := scanner.Scan(
		&run.ID,
		&run.SpaceID,
		&run.ChatID,
		&run.SummaryID,
		&run.RangeStart,
		&run.RangeEnd,
		&run.Status,
		&run.UpdatedPageCount,
		&run.ErrorMessage,
		&run.StartedAt,
		&run.FinishedAt,
		&run.CreatedAt,
		&run.UpdatedAt,
	); err != nil {
		return model.LLMWikiRun{}, err
	}
	return run, nil
}
