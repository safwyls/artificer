package store

import (
	"context"
	"sort"
	"time"
)

// Pending ban edits — the console's intent for a list the running game also
// owns.
//
// enshrouded_server.json's `bannedAccounts` has two writers: this console
// and the game, whose in-game kick/ban UI maintains the same array. The
// game keeps it in memory while it runs and writes it out on the way down,
// so an edit written to the file mid-session is reverted at the next stop.
// A real deployment demonstrated exactly that on 2026-08-16.
//
// The answer is not to fight for the file but to wait for it: an edit made
// while the server is up is recorded here and applied immediately before
// the next start, when the game is holding nothing.
//
// A row lives until the *file* agrees with it, not until it has been
// written. That is deliberate, and it is what lets the console answer the
// question it previously couldn't: if a row was applied to a stopped
// server's config and the file still disagrees afterwards, the game
// overwrote it, and no amount of file-writing will ever make bans stick on
// this build.

// Ban actions.
const (
	PendingBan  = "ban"
	PendingLift = "lift"
)

// PendingBanEdit is one queued edit.
type PendingBanEdit struct {
	AccountID string `json:"id"`
	Action    string `json:"action"`
	// Applied reports that this edit was written into the config while the
	// game was stopped. Still being here means the game did not keep it.
	Applied bool `json:"applied"`
}

// PendingBans returns the queued edits for a server, ordered by account so
// display and tests are stable.
func (s *Store) PendingBans(ctx context.Context, serverID int64) ([]PendingBanEdit, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT account_id, action, applied_at IS NOT NULL FROM pending_bans WHERE server_id = ?`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PendingBanEdit{}
	for rows.Next() {
		var e PendingBanEdit
		if err := rows.Scan(&e.AccountID, &e.Action, &e.Applied); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AccountID < out[j].AccountID })
	return out, nil
}

// QueueBanEdit records one edit, replacing whatever was queued for that
// account. Queueing the opposite of what is already queued cancels the
// pair outright rather than leaving two edits that undo each other — a ban
// added and then lifted before either reached the game never happened.
func (s *Store) QueueBanEdit(ctx context.Context, serverID int64, accountID, action string) error {
	var existing string
	err := s.db.QueryRowContext(ctx,
		`SELECT action FROM pending_bans WHERE server_id = ? AND account_id = ?`,
		serverID, accountID).Scan(&existing)
	if err == nil && existing != action {
		_, derr := s.db.ExecContext(ctx,
			`DELETE FROM pending_bans WHERE server_id = ? AND account_id = ?`, serverID, accountID)
		return derr
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO pending_bans (server_id, account_id, action) VALUES (?, ?, ?)
		ON CONFLICT (server_id, account_id) DO UPDATE SET action = excluded.action, applied_at = NULL`,
		serverID, accountID, action)
	return err
}

// MarkPendingBansApplied stamps every queued edit for a server as written
// into the config. Called after the config was written with the game
// stopped — the only write the game cannot immediately undo.
func (s *Store) MarkPendingBansApplied(ctx context.Context, serverID int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE pending_bans SET applied_at = ? WHERE server_id = ?`,
		at.UTC().Format(time.RFC3339), serverID)
	return err
}

// ReconcilePendingBans drops the queued edits the file now agrees with:
// a ban whose account is in the list, a lift whose account is not. It
// returns the edits still outstanding.
//
// This is the whole retirement path, and it only runs while the game is
// up. That condition is the point. With the game stopped, the file
// agreeing proves only that the console wrote it — which is exactly what
// the game then overwrites. With the game up, the file agreeing means the
// game loaded the change and kept it, which is the confirmation worth
// retiring a row for. Retiring on our own write instead would delete the
// evidence before the restart that tests it.
func (s *Store) ReconcilePendingBans(ctx context.Context, serverID int64, banned []string, running bool) ([]PendingBanEdit, error) {
	pending, err := s.PendingBans(ctx, serverID)
	if err != nil {
		return nil, err
	}
	if len(pending) == 0 || !running {
		return pending, nil
	}
	inFile := make(map[string]bool, len(banned))
	for _, id := range banned {
		inFile[id] = true
	}
	out := make([]PendingBanEdit, 0, len(pending))
	for _, e := range pending {
		satisfied := (e.Action == PendingBan) == inFile[e.AccountID]
		if !satisfied {
			out = append(out, e)
			continue
		}
		if _, derr := s.db.ExecContext(ctx,
			`DELETE FROM pending_bans WHERE server_id = ? AND account_id = ?`,
			serverID, e.AccountID); derr != nil {
			return nil, derr
		}
	}
	return out, nil
}

// ClearPendingBans drops every queued edit for a server.
func (s *Store) ClearPendingBans(ctx context.Context, serverID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM pending_bans WHERE server_id = ?`, serverID)
	return err
}
