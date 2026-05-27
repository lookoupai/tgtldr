package store

import (
	"context"
	"fmt"
	"time"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MessageRepository struct {
	pool *pgxpool.Pool
}

func (r *MessageRepository) Upsert(ctx context.Context, message model.Message) error {
	_, err := r.pool.Exec(ctx, `
		insert into messages (
			chat_id, telegram_message_id, telegram_sender_id, sender_name, sender_username, sender_is_bot,
			text_content, caption, message_type, media_kind, reply_to_message_id,
			message_time, raw_json
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb)
		on conflict (chat_id, telegram_message_id) do update
		set telegram_sender_id = excluded.telegram_sender_id,
		    sender_name = excluded.sender_name,
		    sender_username = excluded.sender_username,
		    sender_is_bot = excluded.sender_is_bot,
		    text_content = excluded.text_content,
		    caption = excluded.caption,
		    message_type = excluded.message_type,
		    media_kind = excluded.media_kind,
		    reply_to_message_id = excluded.reply_to_message_id,
		    message_time = excluded.message_time,
		    raw_json = excluded.raw_json
	`,
		message.ChatID,
		message.TelegramMessageID,
		message.TelegramSenderID,
		message.SenderName,
		message.SenderUsername,
		message.SenderIsBot,
		message.TextContent,
		message.Caption,
		message.MessageType,
		message.MediaKind,
		message.ReplyToMessageID,
		message.MessageTime,
		message.RawJSON,
	)
	if err != nil {
		return fmt.Errorf("upsert message %d: %w", message.TelegramMessageID, err)
	}
	return nil
}

func (r *MessageRepository) ListForRange(ctx context.Context, chatID int64, start, end time.Time) ([]model.Message, error) {
	rows, err := r.pool.Query(ctx, `
		select id, chat_id, telegram_message_id, telegram_sender_id, sender_name,
		       sender_username, sender_is_bot,
		       text_content, caption, message_type, media_kind, reply_to_message_id,
		       message_time, raw_json::text, created_at
		from messages
		where chat_id = $1 and message_time >= $2 and message_time < $3
		order by message_time asc, telegram_message_id asc
	`, chatID, start, end)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []model.Message
	for rows.Next() {
		var message model.Message
		err := rows.Scan(
			&message.ID,
			&message.ChatID,
			&message.TelegramMessageID,
			&message.TelegramSenderID,
			&message.SenderName,
			&message.SenderUsername,
			&message.SenderIsBot,
			&message.TextContent,
			&message.Caption,
			&message.MessageType,
			&message.MediaKind,
			&message.ReplyToMessageID,
			&message.MessageTime,
			&message.RawJSON,
			&message.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (r *MessageRepository) CountForRange(ctx context.Context, chatID int64, start, end time.Time) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `
		select count(*)
		from messages
		where chat_id = $1 and message_time >= $2 and message_time < $3
	`, chatID, start, end).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count messages: %w", err)
	}
	return count, nil
}

func (r *MessageRepository) LatestSenderUsernames(ctx context.Context, senderIDs []int64) (map[int64]string, error) {
	ids := uniqueMessageSenderIDs(senderIDs)
	if len(ids) == 0 {
		return map[int64]string{}, nil
	}

	rows, err := r.pool.Query(ctx, `
		select distinct on (telegram_sender_id) telegram_sender_id, sender_username
		from messages
		where telegram_sender_id = any($1)
		  and btrim(sender_username) <> ''
		order by telegram_sender_id, message_time desc, id desc
	`, ids)
	if err != nil {
		return nil, fmt.Errorf("query latest sender usernames: %w", err)
	}
	defer rows.Close()

	usernames := make(map[int64]string, len(ids))
	for rows.Next() {
		var senderID int64
		var username string
		if err := rows.Scan(&senderID, &username); err != nil {
			return nil, fmt.Errorf("scan latest sender username: %w", err)
		}
		usernames[senderID] = username
	}
	return usernames, rows.Err()
}

func (r *MessageRepository) LookupByTelegramIDs(ctx context.Context, chatID int64, ids []int) (map[int]model.Message, error) {
	if len(ids) == 0 {
		return map[int]model.Message{}, nil
	}

	rows, err := r.pool.Query(ctx, `
		select id, chat_id, telegram_message_id, telegram_sender_id, sender_name,
		       sender_username, sender_is_bot,
		       text_content, caption, message_type, media_kind, reply_to_message_id,
		       message_time, raw_json::text, created_at
		from messages
		where chat_id = $1 and telegram_message_id = any($2)
	`, chatID, ids)
	if err != nil {
		return nil, fmt.Errorf("lookup messages by telegram ids: %w", err)
	}
	defer rows.Close()

	lookup := make(map[int]model.Message, len(ids))
	for rows.Next() {
		var message model.Message
		err := rows.Scan(
			&message.ID,
			&message.ChatID,
			&message.TelegramMessageID,
			&message.TelegramSenderID,
			&message.SenderName,
			&message.SenderUsername,
			&message.SenderIsBot,
			&message.TextContent,
			&message.Caption,
			&message.MessageType,
			&message.MediaKind,
			&message.ReplyToMessageID,
			&message.MessageTime,
			&message.RawJSON,
			&message.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan lookup message: %w", err)
		}
		lookup[message.TelegramMessageID] = message
	}
	return lookup, rows.Err()
}

func uniqueMessageSenderIDs(values []int64) []int64 {
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
