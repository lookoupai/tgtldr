package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BotTargetChatRepository struct {
	pool *pgxpool.Pool
}

func (r *BotTargetChatRepository) Upsert(ctx context.Context, candidate model.BotTargetChatCandidate) error {
	if candidate.BotID == 0 || candidate.FromUserID == 0 || strings.TrimSpace(candidate.ChatID) == "" {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
		insert into bot_target_chat_candidates (
			bot_id, chat_id, from_user_id, chat_type, title, username, from_username, message_date, update_id
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		on conflict (bot_id, chat_id, from_user_id) do update
		set chat_type = excluded.chat_type,
		    title = excluded.title,
		    username = excluded.username,
		    from_username = excluded.from_username,
		    message_date = excluded.message_date,
		    update_id = excluded.update_id,
		    updated_at = now()
	`, candidate.BotID,
		strings.TrimSpace(candidate.ChatID),
		candidate.FromUserID,
		strings.TrimSpace(candidate.ChatType),
		strings.TrimSpace(candidate.Title),
		strings.TrimSpace(candidate.Username),
		strings.TrimSpace(candidate.FromUsername),
		candidate.MessageDate,
		candidate.UpdateID,
	)
	if err != nil {
		return fmt.Errorf("upsert bot target chat candidate: %w", err)
	}
	return nil
}

func (r *BotTargetChatRepository) ListByBotAndFromUser(
	ctx context.Context,
	botID int64,
	fromUserID int64,
	limit int,
) ([]model.BotTargetChatCandidate, error) {
	if botID == 0 || fromUserID == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		select bot_id, chat_id, from_user_id, chat_type, title, username, from_username,
		       message_date, update_id, created_at, updated_at
		from bot_target_chat_candidates
		where bot_id = $1 and from_user_id = $2
		order by message_date desc, update_id desc, updated_at desc
		limit $3
	`, botID, fromUserID, limit)
	if err != nil {
		return nil, fmt.Errorf("query bot target chat candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]model.BotTargetChatCandidate, 0)
	for rows.Next() {
		candidate, err := scanBotTargetChatCandidate(rows)
		if err != nil {
			return nil, fmt.Errorf("scan bot target chat candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (r *BotTargetChatRepository) ListByBot(
	ctx context.Context,
	botID int64,
	limit int,
) ([]model.BotTargetChatCandidate, error) {
	if botID == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		select bot_id, chat_id, from_user_id, chat_type, title, username, from_username,
		       message_date, update_id, created_at, updated_at
		from bot_target_chat_candidates
		where bot_id = $1
		order by message_date desc, update_id desc, updated_at desc
		limit $2
	`, botID, limit)
	if err != nil {
		return nil, fmt.Errorf("query bot target chat candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]model.BotTargetChatCandidate, 0)
	for rows.Next() {
		candidate, err := scanBotTargetChatCandidate(rows)
		if err != nil {
			return nil, fmt.Errorf("scan bot target chat candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func scanBotTargetChatCandidate(scanner chatScanner) (model.BotTargetChatCandidate, error) {
	var candidate model.BotTargetChatCandidate
	err := scanner.Scan(
		&candidate.BotID,
		&candidate.ChatID,
		&candidate.FromUserID,
		&candidate.ChatType,
		&candidate.Title,
		&candidate.Username,
		&candidate.FromUsername,
		&candidate.MessageDate,
		&candidate.UpdateID,
		&candidate.CreatedAt,
		&candidate.UpdatedAt,
	)
	if err != nil {
		return model.BotTargetChatCandidate{}, err
	}
	return candidate, nil
}
