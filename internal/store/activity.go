package store

import (
	"context"
	"time"
)

// PlayerEvent is one observed join or leave, at the collector's sampling
// granularity (~30s).
type PlayerEvent struct {
	ID       int64     `json:"id"`
	ServerID int64     `json:"-"`
	TS       time.Time `json:"ts"`
	UserID   string    `json:"userId"`
	Name     string    `json:"name"`
	Event    string    `json:"event"`
}

func (s *Store) InsertPlayerEvent(ctx context.Context, serverID int64, at time.Time, userID, name, event string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO player_events (server_id, ts, user_id, name, event) VALUES (?, ?, ?, ?, ?)`,
		serverID, at.UTC().Format(time.RFC3339), userID, name, event)
	return err
}

// ListPlayerEvents returns events since the cutoff, newest first.
func (s *Store) ListPlayerEvents(ctx context.Context, serverID int64, since time.Time) ([]PlayerEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, server_id, ts, user_id, name, event FROM player_events
		WHERE server_id = ? AND ts >= ? ORDER BY ts DESC, id DESC`,
		serverID, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PlayerEvent{}
	for rows.Next() {
		var (
			e  PlayerEvent
			ts string
		)
		if err := rows.Scan(&e.ID, &e.ServerID, &ts, &e.UserID, &e.Name, &e.Event); err != nil {
			return nil, err
		}
		e.TS, _ = time.Parse(time.RFC3339, ts)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) PrunePlayerEvents(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM player_events WHERE ts < ?`, before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// AuditEntry is one management action taken through Palcon.
type AuditEntry struct {
	ID       int64     `json:"id"`
	TS       time.Time `json:"ts"`
	Username string    `json:"username"`
	ServerID int64     `json:"-"`
	Action   string    `json:"action"`
	Detail   string    `json:"detail"`
}

func (s *Store) InsertAudit(ctx context.Context, serverID int64, username, action, detail string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audit_log (ts, username, server_id, action, detail) VALUES (?, ?, ?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), username, serverID, action, detail)
	return err
}

func (s *Store) ListAudit(ctx context.Context, serverID int64, limit int) ([]AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, ts, username, server_id, action, detail FROM audit_log
		WHERE server_id = ? ORDER BY ts DESC, id DESC LIMIT ?`, serverID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []AuditEntry{}
	for rows.Next() {
		var (
			e  AuditEntry
			ts string
		)
		if err := rows.Scan(&e.ID, &ts, &e.Username, &e.ServerID, &e.Action, &e.Detail); err != nil {
			return nil, err
		}
		e.TS, _ = time.Parse(time.RFC3339, ts)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) PruneAudit(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM audit_log WHERE ts < ?`, before.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
