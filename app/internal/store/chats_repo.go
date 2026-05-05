package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ChatRepository struct {
	pool *pgxpool.Pool
}

func (r *ChatRepository) List(ctx context.Context) ([]model.Chat, error) {
	rows, err := r.pool.Query(ctx, `
		`+chatSelectColumns()+`
		from chats
		order by enabled desc, title asc
	`)
	if err != nil {
		return nil, fmt.Errorf("query chats: %w", err)
	}
	defer rows.Close()

	chats := make([]model.Chat, 0)
	for rows.Next() {
		chat, err := scanChat(rows)
		if err != nil {
			return nil, fmt.Errorf("scan chat: %w", err)
		}
		chats = append(chats, chat)
	}
	return chats, rows.Err()
}

func (r *ChatRepository) ListBotInteractionEnabled(ctx context.Context) ([]model.Chat, error) {
	rows, err := r.pool.Query(ctx, `
		`+chatSelectColumns()+`
		from chats
		where bot_interaction_enabled = true and trim(bot_chat_id) <> ''
		order by id asc
	`)
	if err != nil {
		return nil, fmt.Errorf("query bot interaction chats: %w", err)
	}
	defer rows.Close()

	chats := make([]model.Chat, 0)
	for rows.Next() {
		chat, err := scanChat(rows)
		if err != nil {
			return nil, fmt.Errorf("scan bot interaction chat: %w", err)
		}
		chats = append(chats, chat)
	}
	return chats, rows.Err()
}

func (r *ChatRepository) CountEnabled(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `select count(*) from chats where enabled = true`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count enabled chats: %w", err)
	}
	return count, nil
}

func (r *ChatRepository) UpsertMany(ctx context.Context, chats []model.Chat) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin chats tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, chat := range chats {
		_, err := tx.Exec(ctx, `
			insert into chats (
				telegram_chat_id, telegram_access_hash, title, username, chat_type,
				enabled, summary_enabled, summary_context, summary_prompt, summary_mode, summary_language, topic_groups,
				summary_time_local, summary_timezone, delivery_mode, model_override,
				keep_bot_messages, filtered_senders, filtered_keywords
			) values ($1, $2, $3, $4, $5, false, false, '', '', 'channel', '', '[]'::jsonb, '09:00', 'Asia/Shanghai', 'dashboard', '', true, '{}', '{}')
			on conflict (telegram_chat_id) do update
			set telegram_access_hash = excluded.telegram_access_hash,
			    title = excluded.title,
			    username = excluded.username,
			    chat_type = excluded.chat_type,
			    updated_at = now()
		`,
			chat.TelegramChatID,
			chat.TelegramAccess,
			chat.Title,
			chat.Username,
			chat.ChatType,
		)
		if err != nil {
			return fmt.Errorf("upsert chat %d: %w", chat.TelegramChatID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit chats tx: %w", err)
	}
	return nil
}

func (r *ChatRepository) Save(ctx context.Context, chat model.Chat) (model.Chat, error) {
	topicGroups, err := marshalTopicGroups(chat.TopicGroups)
	if err != nil {
		return model.Chat{}, err
	}
	saved, err := scanChat(rowScanner{row: r.pool.QueryRow(ctx, `
		update chats
		set enabled = $1,
		    summary_enabled = $2,
		    summary_context = $3,
		    summary_prompt = $4,
		    summary_mode = $5,
		    summary_language = $6,
		    topic_groups = $7::jsonb,
		    summary_time_local = $8,
		    delivery_mode = $9,
		    model_override = $10,
		    keep_bot_messages = $11,
		    filtered_senders = $12,
		    filtered_keywords = $13,
		    bot_chat_id = $14,
		    bot_interaction_enabled = $15,
		    bot_allowed_users = $16,
		    updated_at = now()
		where id = $17
		returning `+chatColumns()+`
	`,
		chat.Enabled,
		chat.SummaryEnabled,
		chat.SummaryContext,
		chat.SummaryPrompt,
		model.NormalizeSummaryMode(chat.SummaryMode),
		normalizeChatSummaryLanguage(chat.SummaryLanguage),
		topicGroups,
		chat.SummaryTimeLocal,
		chat.DeliveryMode,
		chat.ModelOverride,
		chat.KeepBotMessages,
		chat.FilteredSenders,
		chat.FilteredKeywords,
		strings.TrimSpace(chat.BotChatID),
		chat.BotInteraction,
		compactStrings(chat.BotAllowedUsers),
		chat.ID,
	)})
	if err != nil {
		return model.Chat{}, fmt.Errorf("save chat %d: %w", chat.ID, err)
	}
	return saved, nil
}

func (r *ChatRepository) GetByID(ctx context.Context, id int64) (model.Chat, error) {
	chat, err := scanChat(rowScanner{row: r.pool.QueryRow(ctx, `
		`+chatSelectColumns()+`
		from chats
		where id = $1
	`, id)})
	if err != nil {
		return model.Chat{}, fmt.Errorf("get chat %d: %w", id, err)
	}
	return chat, nil
}

func (r *ChatRepository) ListSummaryEnabled(ctx context.Context) ([]model.Chat, error) {
	rows, err := r.pool.Query(ctx, `
		`+chatSelectColumns()+`
		from chats
		where summary_enabled = true
		order by id asc
	`)
	if err != nil {
		return nil, fmt.Errorf("query summary enabled chats: %w", err)
	}
	defer rows.Close()

	out := make([]model.Chat, 0)
	for rows.Next() {
		chat, err := scanChat(rows)
		if err != nil {
			return nil, fmt.Errorf("scan summary enabled chat: %w", err)
		}
		out = append(out, chat)
	}
	return out, rows.Err()
}

func (r *ChatRepository) GetByTelegramID(ctx context.Context, telegramID int64) (model.Chat, error) {
	chat, err := scanChat(rowScanner{row: r.pool.QueryRow(ctx, `
		`+chatSelectColumns()+`
		from chats
		where telegram_chat_id = $1
	`, telegramID)})
	if err != nil {
		return model.Chat{}, fmt.Errorf("get chat by telegram id %d: %w", telegramID, err)
	}
	return chat, nil
}

func (r *ChatRepository) EnsureExists(ctx context.Context, chat model.Chat) (model.Chat, error) {
	if err := r.UpsertMany(ctx, []model.Chat{chat}); err != nil {
		return model.Chat{}, err
	}
	return r.GetByTelegramID(ctx, chat.TelegramChatID)
}

func IsNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

type chatScanner interface {
	Scan(dest ...any) error
}

type rowScanner struct {
	row pgx.Row
}

func (s rowScanner) Scan(dest ...any) error {
	return s.row.Scan(dest...)
}

func chatSelectColumns() string {
	return "select " + chatColumns()
}

func chatColumns() string {
	return `id, telegram_chat_id, telegram_access_hash, title, username, chat_type,
		       enabled, summary_enabled, summary_context, summary_prompt, summary_mode, summary_language, topic_groups::text,
		       summary_time_local, summary_timezone,
		       delivery_mode, model_override, keep_bot_messages, filtered_senders, filtered_keywords,
		       bot_chat_id, bot_interaction_enabled, bot_allowed_users,
		       created_at, updated_at`
}

func scanChat(scanner chatScanner) (model.Chat, error) {
	var chat model.Chat
	var topicGroupsJSON string
	err := scanner.Scan(
		&chat.ID,
		&chat.TelegramChatID,
		&chat.TelegramAccess,
		&chat.Title,
		&chat.Username,
		&chat.ChatType,
		&chat.Enabled,
		&chat.SummaryEnabled,
		&chat.SummaryContext,
		&chat.SummaryPrompt,
		&chat.SummaryMode,
		&chat.SummaryLanguage,
		&topicGroupsJSON,
		&chat.SummaryTimeLocal,
		&chat.SummaryTimezone,
		&chat.DeliveryMode,
		&chat.ModelOverride,
		&chat.KeepBotMessages,
		&chat.FilteredSenders,
		&chat.FilteredKeywords,
		&chat.BotChatID,
		&chat.BotInteraction,
		&chat.BotAllowedUsers,
		&chat.CreatedAt,
		&chat.UpdatedAt,
	)
	if err != nil {
		return model.Chat{}, err
	}
	chat.SummaryMode = model.NormalizeSummaryMode(chat.SummaryMode)
	chat.SummaryLanguage = normalizeChatSummaryLanguage(chat.SummaryLanguage)
	chat.TopicGroups = unmarshalTopicGroups(topicGroupsJSON)
	chat.BotAllowedUsers = compactStrings(chat.BotAllowedUsers)
	return chat, nil
}

func normalizeChatSummaryLanguage(language model.SummaryOutputLanguage) model.SummaryOutputLanguage {
	return model.NormalizeOptionalSummaryOutputLanguage(language)
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func marshalTopicGroups(groups []model.TopicGroup) (string, error) {
	normalized := make([]model.TopicGroup, 0, len(groups))
	for _, group := range groups {
		name := strings.TrimSpace(group.Name)
		if name == "" {
			continue
		}
		normalized = append(normalized, model.TopicGroup{
			Name:        name,
			Description: strings.TrimSpace(group.Description),
		})
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal topic groups: %w", err)
	}
	return string(data), nil
}

func unmarshalTopicGroups(raw string) []model.TopicGroup {
	var groups []model.TopicGroup
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return nil
	}
	normalized := make([]model.TopicGroup, 0, len(groups))
	for _, group := range groups {
		name := strings.TrimSpace(group.Name)
		if name == "" {
			continue
		}
		normalized = append(normalized, model.TopicGroup{
			Name:        name,
			Description: strings.TrimSpace(group.Description),
		})
	}
	return normalized
}
