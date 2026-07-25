package collector

import (
	"context"
	"log/slog"
	"time"

	"github.com/safwyls/palcon/internal/palsave"
	"github.com/safwyls/palcon/internal/store"
)

const (
	// savePollEvery is how often save files are checked for changes. Checks
	// on an unchanged file are one stat plus a map lookup, so polling can
	// stay tight without costing anything.
	savePollEvery = 15 * time.Second
	// saveAttemptFloor is the minimum gap between parse attempts for one
	// save. Palworld can autosave every 30 seconds and a big world costs
	// seconds of CPU per parse, so freshness is capped at roughly this
	// rather than chasing every autosave. It also spaces out retries when a
	// save keeps failing to parse.
	saveAttemptFloor = 45 * time.Second
)

// SaveRefresher keeps the shared save-parse cache warm by re-parsing each
// enabled server's save shortly after the game writes it, so the pals and
// calculator pages open onto a cache hit instead of a multi-second parse.
// It also warms every save once at startup, which covers restarts.
type SaveRefresher struct {
	store  *store.Store
	reader *palsave.Reader
	logger *slog.Logger

	// nextAttempt spaces parse attempts per save path; only touched from
	// Run's goroutine. Entries for removed servers linger harmlessly.
	nextAttempt map[string]time.Time
}

func NewSaveRefresher(st *store.Store, reader *palsave.Reader, logger *slog.Logger) *SaveRefresher {
	return &SaveRefresher{store: st, reader: reader, logger: logger, nextAttempt: make(map[string]time.Time)}
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
		if !srv.Enabled || srv.SavePath == "" {
			continue
		}
		if time.Now().Before(s.nextAttempt[srv.SavePath]) {
			continue
		}
		parsed, err := s.reader.Refresh(ctx, srv.SavePath)
		if parsed || err != nil {
			s.nextAttempt[srv.SavePath] = time.Now().Add(saveAttemptFloor)
		}
		if err != nil {
			s.logger.Warn("save refresh failed", "server", srv.Name, "error", err)
		}
		if ctx.Err() != nil {
			return
		}
	}
}
