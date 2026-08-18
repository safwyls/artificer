package collector

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/safwyls/artificer/core/agentctl"
	"github.com/safwyls/artificer/core/agentfiles"
	"github.com/safwyls/artificer/core/store"
)

const (
	// savePollEvery is how often save files are checked for changes. Checks
	// on an unchanged file are one stat plus a map lookup, so polling can
	// stay tight without costing anything.
	savePollEvery = 15 * time.Second
	// saveAttemptFloor is the minimum gap between parse attempts for one
	// save. Games autosave as often as every 30 seconds and a big world
	// costs seconds of CPU per parse, so freshness is capped at roughly this
	// rather than chasing every autosave. It also spaces out retries when a
	// save keeps failing to parse.
	saveAttemptFloor = 45 * time.Second
)

// SaveReader is the part of a game's save reader this warmer needs. Narrow on
// purpose: it keeps the collector free of any one game's save schema, so the
// same loop warms whatever reader a server's game supplies.
type SaveReader interface {
	// Refresh re-parses the save if it has changed, reporting whether a
	// parse was actually attempted.
	Refresh(ctx context.Context, savePath string) (bool, error)
}

// SaveRefresher keeps each enabled server's save mirrored locally (for
// agent-backed servers this loop *is* the sync that backups snapshot), and —
// when the game has a save reader — keeps the parse cache warm so save-backed
// pages open onto a hit. Dragonwilds has no reader yet (Phase 3 gate in
// games/dragonwilds/docs/recon.md), so reader may be nil and only the sync runs.
type SaveRefresher struct {
	store  *store.Store
	reader SaveReader
	files  *agentfiles.Syncer
	logger *slog.Logger

	// nextAttempt spaces parse attempts per server; only touched from
	// Run's goroutine. Entries for removed servers linger harmlessly.
	nextAttempt map[int64]time.Time
}

func NewSaveRefresher(st *store.Store, reader SaveReader, files *agentfiles.Syncer, logger *slog.Logger) *SaveRefresher {
	return &SaveRefresher{store: st, reader: reader, files: files, logger: logger, nextAttempt: make(map[int64]time.Time)}
}

// Run refreshes until ctx is cancelled. Intended to be started in a goroutine.
func (s *SaveRefresher) Run(ctx context.Context) {
	ticker := time.NewTicker(savePollEvery)
	defer ticker.Stop()

	s.refreshAll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshAll(ctx)
		}
	}
}

func (s *SaveRefresher) refreshAll(ctx context.Context) {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		s.logger.Warn("save refresh: listing servers", "error", err)
		return
	}
	// Sequential on purpose: parses are memory-heavy and serialized inside
	// the reader anyway, and a background warmer has no reason to queue-jump.
	for _, srv := range servers {
		if !srv.Enabled || !agentfiles.SaveConfigured(srv) {
			continue
		}
		if time.Now().Before(s.nextAttempt[srv.ID]) {
			continue
		}
		// For agent-backed servers this is where the sync happens: a
		// conditional GET per poll (a 304 when nothing changed), a
		// bundle download only after the game actually saved.
		savePath, err := s.files.SavePath(ctx, srv)
		if err != nil {
			s.nextAttempt[srv.ID] = time.Now().Add(saveAttemptFloor)
			// A rejection is the agent answering clearly, and by far the
			// most common answer is "no world yet" — the normal state of
			// a server whose game has never run. Warning about that every
			// cycle fills a freshly provisioned console's log with a
			// problem it doesn't have, and buries the failures that are
			// real. Anything else (unreachable, bad token) still warns.
			if errors.Is(err, agentctl.ErrRejected) {
				s.logger.Debug("save refresh: nothing to sync yet", "server", srv.Name, "reason", err)
			} else {
				s.logger.Warn("save refresh: resolving save", "server", srv.Name, "error", err)
			}
			continue
		}
		if s.reader != nil {
			parsed, err := s.reader.Refresh(ctx, savePath)
			if parsed || err != nil {
				s.nextAttempt[srv.ID] = time.Now().Add(saveAttemptFloor)
			}
			if err != nil {
				s.logger.Warn("save refresh failed", "server", srv.Name, "error", err)
			}
		} else {
			// No parser: still space out sync attempts.
			s.nextAttempt[srv.ID] = time.Now().Add(saveAttemptFloor)
		}
		if ctx.Err() != nil {
			return
		}
	}
}
