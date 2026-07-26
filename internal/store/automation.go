package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RestartSchedule is one recurring restart rule for a server: "on these
// weekdays, at this local time, with these warning broadcasts first".
type RestartSchedule struct {
	ID       int64
	ServerID int64
	Enabled  bool
	// Days holds Go time.Weekday values (Sunday = 0), sorted ascending.
	Days []int
	// TimeOfDay is 'HH:MM' local wall-clock time.
	TimeOfDay string
	// WarningMinutes are broadcast lead times, sorted descending.
	WarningMinutes []int
	LastRunAt      *time.Time
}

func encodeInts(vals []int) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ",")
}

func decodeInts(s string) []int {
	if s == "" {
		return nil
	}
	var out []int
	for _, part := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// lastRunTime parses the stored timestamp; a row that has never run keeps a
// nil pointer rather than a zero time so the API can serialize it as null.
func lastRunTime(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s.String)
	if err != nil {
		return nil
	}
	return &t
}

const scheduleColumns = `id, server_id, enabled, days, time_of_day, warning_minutes, last_run_at`

func scanSchedule(scan func(dest ...any) error) (*RestartSchedule, error) {
	var (
		sc          RestartSchedule
		enabled     int
		days, warns string
		lastRun     sql.NullString
	)
	if err := scan(&sc.ID, &sc.ServerID, &enabled, &days, &sc.TimeOfDay, &warns, &lastRun); err != nil {
		return nil, err
	}
	sc.Enabled = enabled != 0
	sc.Days = decodeInts(days)
	sc.WarningMinutes = decodeInts(warns)
	sc.LastRunAt = lastRunTime(lastRun)
	return &sc, nil
}

func (s *Store) querySchedules(ctx context.Context, query string, args ...any) ([]*RestartSchedule, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*RestartSchedule
	for rows.Next() {
		sc, err := scanSchedule(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (s *Store) ListRestartSchedules(ctx context.Context, serverID int64) ([]*RestartSchedule, error) {
	return s.querySchedules(ctx, `SELECT `+scheduleColumns+` FROM restart_schedules WHERE server_id = ? ORDER BY time_of_day, id`, serverID)
}

// ListAllRestartSchedules returns every schedule across servers, for the
// scheduler's periodic sweep.
func (s *Store) ListAllRestartSchedules(ctx context.Context) ([]*RestartSchedule, error) {
	return s.querySchedules(ctx, `SELECT `+scheduleColumns+` FROM restart_schedules ORDER BY server_id, time_of_day, id`)
}

func (s *Store) GetRestartSchedule(ctx context.Context, id int64) (*RestartSchedule, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+scheduleColumns+` FROM restart_schedules WHERE id = ?`, id)
	sc, err := scanSchedule(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return sc, nil
}

func (s *Store) CreateRestartSchedule(ctx context.Context, sc *RestartSchedule) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO restart_schedules (server_id, enabled, days, time_of_day, warning_minutes)
		VALUES (?, ?, ?, ?, ?)`,
		sc.ServerID, boolToInt(sc.Enabled), encodeInts(sc.Days), sc.TimeOfDay, encodeInts(sc.WarningMinutes))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) UpdateRestartSchedule(ctx context.Context, sc *RestartSchedule) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE restart_schedules SET enabled = ?, days = ?, time_of_day = ?, warning_minutes = ?
		WHERE id = ? AND server_id = ?`,
		boolToInt(sc.Enabled), encodeInts(sc.Days), sc.TimeOfDay, encodeInts(sc.WarningMinutes), sc.ID, sc.ServerID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) DeleteRestartSchedule(ctx context.Context, serverID, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM restart_schedules WHERE id = ? AND server_id = ?`, id, serverID)
	return err
}

func (s *Store) MarkRestartScheduleRun(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE restart_schedules SET last_run_at = ? WHERE id = ?`,
		at.UTC().Format(time.RFC3339), id)
	return err
}

// DiscordWebhook is a server's Discord notification config. WebhookURL is
// decrypted here and must never be serialized to the API — clients only ever
// learn whether one is configured.
type DiscordWebhook struct {
	ServerID   int64
	WebhookURL string
	Enabled    bool
	OnStatus   bool
	OnPlayers  bool
	OnRestarts bool
}

func (s *Store) GetDiscordWebhook(ctx context.Context, serverID int64) (*DiscordWebhook, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT server_id, webhook_url_enc, enabled, on_status, on_players, on_restarts
		FROM discord_webhooks WHERE server_id = ?`, serverID)
	var (
		w                                       DiscordWebhook
		urlEnc                                  string
		enabled, onStatus, onPlayers, onRestart int
	)
	err := row.Scan(&w.ServerID, &urlEnc, &enabled, &onStatus, &onPlayers, &onRestart)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	w.WebhookURL, err = s.box.Decrypt(urlEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypting webhook url: %w", err)
	}
	w.Enabled = enabled != 0
	w.OnStatus = onStatus != 0
	w.OnPlayers = onPlayers != 0
	w.OnRestarts = onRestart != 0
	return &w, nil
}

// SetDiscordWebhook upserts a server's webhook config. An empty WebhookURL
// keeps the stored one (mirroring how server password updates work), so
// toggles can be changed without re-pasting the secret.
func (s *Store) SetDiscordWebhook(ctx context.Context, w *DiscordWebhook) error {
	if w.WebhookURL == "" {
		existing, err := s.GetDiscordWebhook(ctx, w.ServerID)
		if err != nil {
			return fmt.Errorf("no webhook URL provided and none stored: %w", err)
		}
		w.WebhookURL = existing.WebhookURL
	}
	urlEnc, err := s.box.Encrypt(w.WebhookURL)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO discord_webhooks (server_id, webhook_url_enc, enabled, on_status, on_players, on_restarts)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(server_id) DO UPDATE SET
			webhook_url_enc = excluded.webhook_url_enc,
			enabled = excluded.enabled,
			on_status = excluded.on_status,
			on_players = excluded.on_players,
			on_restarts = excluded.on_restarts`,
		w.ServerID, urlEnc, boolToInt(w.Enabled), boolToInt(w.OnStatus), boolToInt(w.OnPlayers), boolToInt(w.OnRestarts))
	return err
}

func (s *Store) DeleteDiscordWebhook(ctx context.Context, serverID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM discord_webhooks WHERE server_id = ?`, serverID)
	return err
}
