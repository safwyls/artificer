// Package watchdog revives crashed game servers: when a watched container
// is found exited with a non-zero code, it's restarted, with strikes and a
// cooldown so a server that crashes on boot doesn't get bounced forever.
//
// Deliberately container-only: a running-but-unresponsive game is left
// alone, because "unresponsive" is indistinguishable from a long save, a
// steam update or a heavy autosave from out here, and restarting a healthy
// server is worse than not restarting a wedged one.
//
// Clean exits (code 0) are never revived — Palcon's own stop flow asks the
// game to exit cleanly precisely so an intentional stop reads as one. The
// caveat, surfaced in the UI: stopping the container behind Palcon's back
// (docker stop straight to a game that ignores SIGTERM ends in SIGKILL,
// exit 137) looks exactly like a crash and will be revived.
package watchdog

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/safwyls/wildskeeper/internal/dockerctl"
	"github.com/safwyls/wildskeeper/internal/notify"
	"github.com/safwyls/wildskeeper/internal/store"
)

const (
	// tick matches the collector's cadence: a crash is noticed within ~30s.
	tick = 30 * time.Second
	// cooldown after a restart attempt before the next is considered, so
	// the watchdog observes the outcome instead of machine-gunning starts.
	cooldown = 5 * time.Minute
	// healthyAfter is how long a container must stay up to clear its
	// strikes — surviving boot isn't health, surviving a while is.
	healthyAfter = 10 * time.Minute
	// maxStrikes is how many consecutive revivals are attempted before
	// standing down; a server that crashes three times in a row needs a
	// human, not a fourth restart.
	maxStrikes = 3
)

type action int

const (
	actNone action = iota
	actRestart
	actGiveUp
)

// serverState is the per-server crash-loop memory.
type serverState struct {
	strikes     int
	lastAttempt time.Time
	standDown   bool
}

// evaluate decides what to do about a container in the given state, and
// updates the loop-protection bookkeeping. Pure of I/O for testability.
func evaluate(st *serverState, cs *dockerctl.State, now time.Time) action {
	if cs.Running {
		started, err := time.Parse(time.RFC3339Nano, cs.StartedAt)
		if err == nil && now.Sub(started) >= healthyAfter {
			st.strikes = 0
			st.standDown = false
		}
		return actNone
	}
	// Only a finished container is a candidate — "restarting" and
	// "created" are transitions docker is already handling.
	if cs.Status != "exited" && cs.Status != "dead" {
		return actNone
	}
	if cs.ExitCode == 0 {
		// A clean exit is someone's decision. It also marks human
		// intervention, so the slate is wiped.
		st.strikes = 0
		st.standDown = false
		return actNone
	}
	if st.standDown {
		return actNone
	}
	if !st.lastAttempt.IsZero() && now.Sub(st.lastAttempt) < cooldown {
		return actNone
	}
	if st.strikes >= maxStrikes {
		st.standDown = true
		return actGiveUp
	}
	st.strikes++
	st.lastAttempt = now
	return actRestart
}

type Watchdog struct {
	store    *store.Store
	docker   *dockerctl.Client
	notifier *notify.Notifier
	logger   *slog.Logger

	mu    sync.Mutex
	state map[int64]*serverState
}

func New(st *store.Store, docker *dockerctl.Client, notifier *notify.Notifier, logger *slog.Logger) *Watchdog {
	return &Watchdog{store: st, docker: docker, notifier: notifier, logger: logger, state: make(map[int64]*serverState)}
}

// Run sweeps until ctx is cancelled. Intended to be started in a goroutine,
// and only when docker control is configured.
func (w *Watchdog) Run(ctx context.Context) {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.sweep(ctx)
		}
	}
}

func (w *Watchdog) sweep(ctx context.Context) {
	servers, err := w.store.ListServers(ctx)
	if err != nil {
		w.logger.Error("watchdog: listing servers", "error", err)
		return
	}
	for _, srv := range servers {
		if !srv.Enabled || !srv.Watchdog || srv.ContainerName == "" {
			continue
		}
		w.check(ctx, srv)
	}
}

func (w *Watchdog) check(ctx context.Context, srv *store.Server) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	cs, err := w.docker.Inspect(ctx, srv.ContainerName)
	if err != nil {
		// An unreachable docker endpoint is not a crashed game server.
		w.logger.Warn("watchdog: inspect failed", "server", srv.ID, "container", srv.ContainerName, "error", err)
		return
	}

	w.mu.Lock()
	st := w.state[srv.ID]
	if st == nil {
		st = &serverState{}
		w.state[srv.ID] = st
	}
	act := evaluate(st, cs, time.Now())
	strikes := st.strikes
	w.mu.Unlock()

	switch act {
	case actRestart:
		w.logger.Info("watchdog: restarting crashed server",
			"server", srv.ID, "container", srv.ContainerName, "exitCode", cs.ExitCode, "attempt", strikes)
		w.notifier.WatchdogRestarted(ctx, srv, cs.ExitCode, strikes)
		if err := w.docker.Start(ctx, srv.ContainerName); err != nil {
			w.logger.Error("watchdog: restart failed", "server", srv.ID, "container", srv.ContainerName, "error", err)
			return
		}
		detail := fmt.Sprintf("exit code %d, attempt %d", cs.ExitCode, strikes)
		if err := w.store.InsertAudit(ctx, srv.ID, "watchdog", "watchdog-restart", detail); err != nil {
			w.logger.Error("watchdog: recording audit entry", "server", srv.ID, "error", err)
		}
	case actGiveUp:
		w.logger.Error("watchdog: standing down after repeated crashes",
			"server", srv.ID, "container", srv.ContainerName, "attempts", strikes)
		w.notifier.WatchdogGaveUp(ctx, srv, strikes)
		if err := w.store.InsertAudit(ctx, srv.ID, "watchdog", "watchdog-standdown",
			fmt.Sprintf("after %d attempts", strikes)); err != nil {
			w.logger.Error("watchdog: recording audit entry", "server", srv.ID, "error", err)
		}
	}
}
