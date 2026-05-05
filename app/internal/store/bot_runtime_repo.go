package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/frederic/tgtldr/app/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BotRuntimeRepository struct {
	pool *pgxpool.Pool
}

func (r *BotRuntimeRepository) Get(ctx context.Context) (model.BotRuntimeState, error) {
	state, err := scanBotRuntimeState(rowScanner{row: r.pool.QueryRow(ctx, `
		select id, bot_username, last_poll_at, last_update_at, last_handled_at, last_error, updated_at
		from bot_runtime_state
		where id = 1
	`)})
	if err != nil {
		return model.BotRuntimeState{}, fmt.Errorf("get bot runtime state: %w", err)
	}
	return state, nil
}

func (r *BotRuntimeRepository) MarkPoll(ctx context.Context, username string, hasUpdates bool) error {
	updateColumn := ""
	if hasUpdates {
		updateColumn = ", last_update_at = now()"
	}
	_, err := r.pool.Exec(ctx, `
		insert into bot_runtime_state (id, bot_username, last_poll_at, last_error, updated_at)
		values (1, $1, now(), '', now())
		on conflict (id) do update
		set bot_username = excluded.bot_username,
		    last_poll_at = now(),
		    last_error = ''`+updateColumn+`,
		    updated_at = now()
	`, strings.TrimSpace(username))
	if err != nil {
		return fmt.Errorf("mark bot poll: %w", err)
	}
	return nil
}

func (r *BotRuntimeRepository) MarkHandled(ctx context.Context, username string) error {
	_, err := r.pool.Exec(ctx, `
		insert into bot_runtime_state (id, bot_username, last_handled_at, last_error, updated_at)
		values (1, $1, now(), '', now())
		on conflict (id) do update
		set bot_username = excluded.bot_username,
		    last_handled_at = now(),
		    last_error = '',
		    updated_at = now()
	`, strings.TrimSpace(username))
	if err != nil {
		return fmt.Errorf("mark bot handled: %w", err)
	}
	return nil
}

func (r *BotRuntimeRepository) MarkError(ctx context.Context, username string, err error) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return r.MarkErrorText(ctx, username, message)
}

func (r *BotRuntimeRepository) MarkErrorText(ctx context.Context, username string, message string) error {
	_, err := r.pool.Exec(ctx, `
		insert into bot_runtime_state (id, bot_username, last_error, updated_at)
		values (1, $1, $2, now())
		on conflict (id) do update
		set bot_username = excluded.bot_username,
		    last_error = excluded.last_error,
		    updated_at = now()
	`, strings.TrimSpace(username), strings.TrimSpace(message))
	if err != nil {
		return fmt.Errorf("mark bot error: %w", err)
	}
	return nil
}

func scanBotRuntimeState(scanner chatScanner) (model.BotRuntimeState, error) {
	var state model.BotRuntimeState
	err := scanner.Scan(
		&state.ID,
		&state.BotUsername,
		&state.LastPollAt,
		&state.LastUpdateAt,
		&state.LastHandledAt,
		&state.LastError,
		&state.UpdatedAt,
	)
	if err != nil {
		return model.BotRuntimeState{}, err
	}
	return state, nil
}
