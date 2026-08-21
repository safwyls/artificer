package store

import (
	"context"
	"time"
)

// Per-server state a game module keeps across restarts.
//
// Core provides the shelf and stays out of what sits on it: the game
// namespaces its rows with a scope of its own choosing and decides the
// value's shape. See core/db/migrations/0024_game_state.sql for what this
// is and is not for.

// GameStateEntry is one stored fact, with the instant it was true.
type GameStateEntry struct {
	Key       string
	Value     []byte
	UpdatedAt time.Time
}

// PutGameState stores or replaces one entry. updatedAt is when the fact
// was true, which is rarely now — a character sheet read from a save is
// as old as that save.
func (s *Store) PutGameState(ctx context.Context, serverID int64, scope, key string, value []byte, updatedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO game_state (server_id, scope, key, value, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (server_id, scope, key)
		DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		serverID, scope, key, string(value), updatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

// ListGameState returns a server's entries in one scope, newest first, so
// a caller applying a cap keeps the freshest.
func (s *Store) ListGameState(ctx context.Context, serverID int64, scope string) ([]GameStateEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT key, value, updated_at FROM game_state
		WHERE server_id = ? AND scope = ?
		ORDER BY updated_at DESC`, serverID, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GameStateEntry
	for rows.Next() {
		var (
			e     GameStateEntry
			value string
			at    string
		)
		if err := rows.Scan(&e.Key, &value, &at); err != nil {
			return nil, err
		}
		e.Value = []byte(value)
		// A row whose stamp cannot be read is still worth its value; it
		// simply sorts as the oldest possible.
		e.UpdatedAt, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteGameState removes one entry; deleting what is not there is fine.
func (s *Store) DeleteGameState(ctx context.Context, serverID int64, scope, key string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM game_state WHERE server_id = ? AND scope = ? AND key = ?`, serverID, scope, key)
	return err
}
