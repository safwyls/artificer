// Package banqueue applies the console's queued ban edits to
// enshrouded_server.json in the one window where nothing else owns that
// file: after the game has stopped and before it starts again.
//
// The problem it exists for: `bannedAccounts` has two writers. The console
// edits it, and the running game — whose in-game kick/ban UI maintains the
// same array — holds it in memory and writes it back out when it stops. A
// ban written to the file mid-session is therefore reverted at the next
// stop, which a real deployment demonstrated on 2026-08-16.
//
// So the console stops trying to win that race. An edit made while the
// server is up is recorded as intent (internal/store/bans.go) and written
// here, on the way into a start. Both restart paths that this console
// drives — the power handlers and the scheduler — call Apply, which is
// why this is a package rather than a method on either of them.
package banqueue

import (
	"context"
	"log/slog"
	"time"

	"github.com/safwyls/artificer/core/agentfiles"
	"github.com/safwyls/artificer/core/store"
	"github.com/safwyls/artificer/games/enshrouded/esconfig"
)

type Queue struct {
	store  *store.Store
	files  *agentfiles.Syncer
	logger *slog.Logger
}

func New(st *store.Store, files *agentfiles.Syncer, logger *slog.Logger) *Queue {
	return &Queue{store: st, files: files, logger: logger}
}

// Pending reports whether anything is queued for a server. The power
// paths ask first so they only take the slower two-step restart — stop,
// apply, start, rather than one restart call — when there is a reason to.
func (q *Queue) Pending(ctx context.Context, srv *store.Server) bool {
	if q == nil {
		return false
	}
	pending, err := q.store.PendingBans(ctx, srv.ID)
	return err == nil && len(pending) > 0
}

// Apply writes the queued edits into the config and stamps them applied,
// so a later read can tell "not written yet" from "written, and the game
// overwrote it".
//
// Errors are logged, never returned: a bookkeeping failure must not block
// a server from starting. The edits stay queued and are retried at the
// next start.
func (q *Queue) Apply(ctx context.Context, srv *store.Server) {
	if q == nil {
		return
	}
	pending, err := q.store.PendingBans(ctx, srv.ID)
	if err != nil {
		q.logger.Error("pending bans: reading the queue failed", "server", srv.ID, "error", err)
		return
	}
	if len(pending) == 0 {
		return
	}
	path, viaAgent, err := q.files.ConfigPath(ctx, srv)
	if err != nil {
		q.logger.Error("pending bans: no config to apply them to", "server", srv.ID, "error", err)
		return
	}
	res, err := esconfig.ReadBans(path)
	if err != nil {
		q.logger.Error("pending bans: reading the ban list failed", "server", srv.ID, "error", err)
		return
	}

	bans := res.Bans
	for _, e := range pending {
		bans = Apply1(bans, e)
	}
	if err := esconfig.WriteBans(path, bans); err != nil {
		q.logger.Error("pending bans: writing the ban list failed", "server", srv.ID, "error", err)
		return
	}
	if viaAgent {
		if err := q.files.PushConfig(ctx, srv, path); err != nil {
			q.logger.Error("pending bans: pushing the config failed", "server", srv.ID, "error", err)
			return
		}
	}
	if err := q.store.MarkPendingBansApplied(ctx, srv.ID, time.Now()); err != nil {
		q.logger.Error("pending bans: marking them applied failed", "server", srv.ID, "error", err)
	}
	q.logger.Info("applied queued ban edits before start", "server", srv.ID, "edits", len(pending))
}

// Apply1 turns one edit into a list change. Exported and separate so the
// queued path and any direct path can't drift on what "ban" and "lift"
// mean, and so it can be tested without a store.
func Apply1(bans []esconfig.Ban, e store.PendingBanEdit) []esconfig.Ban {
	idx := -1
	for i, b := range bans {
		if b.ID == e.AccountID {
			idx = i
			break
		}
	}
	if e.Action == store.PendingLift {
		if idx < 0 {
			return bans
		}
		return append(bans[:idx:idx], bans[idx+1:]...)
	}
	if idx >= 0 {
		return bans
	}
	return append(bans, esconfig.Ban{Index: -1, ID: e.AccountID})
}
