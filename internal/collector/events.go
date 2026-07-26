package collector

import (
	"context"
	"time"

	"github.com/safwyls/palcon/internal/palworld"
	"github.com/safwyls/palcon/internal/store"
)

// downAfter is how many consecutive failed probes declare a server down.
// At the sampling Interval that's ~1 minute of silence — long enough to
// ride out one dropped request, short enough to still be news.
const downAfter = 2

// serverState is the per-server memory behind the activity log and Discord
// notifications: enough to tell a transition from steady state, and last
// tick's player list to diff joins/leaves against.
type serverState struct {
	// primed is false until the first successful probe; the first
	// observation establishes a baseline instead of logging everyone
	// online as having just "joined".
	primed bool
	fails  int
	down   bool
	// downSince timestamps the failure that began the outage, for the
	// "back online after X" message.
	downSince time.Time
	// players maps UserID → display name as of the last successful probe.
	players map[string]string
}

// watch probes the server's player list each tick and turns changes into
// player_events rows (always) and Discord notifications (gated inside the
// notifier by each server's webhook config).
func (c *Collector) watch(ctx context.Context, srv *store.Server, client palworld.Client) {
	players, err := client.Players(ctx)
	now := time.Now()

	c.mu.Lock()
	st := c.watchState[srv.ID]
	if st == nil {
		st = &serverState{}
		c.watchState[srv.ID] = st
	}

	if err != nil {
		st.fails++
		if !st.down && st.fails == downAfter {
			st.down = true
			st.downSince = now.Add(-time.Duration(downAfter-1) * Interval)
			// Everyone we knew about was just disconnected; closing their
			// sessions at the outage start keeps playtime honest.
			disconnected := st.players
			st.players = nil
			c.mu.Unlock()
			for uid, name := range disconnected {
				c.recordEvent(ctx, srv.ID, st.downSince, uid, name, "leave")
			}
			if c.notifier != nil {
				c.notifier.ServerDown(ctx, srv)
			}
			return
		}
		c.mu.Unlock()
		return
	}

	wasDown := st.down
	downSince := st.downSince
	st.fails = 0
	st.down = false

	current := make(map[string]string, len(players))
	for _, p := range players {
		// UserID is the stable identity (PlayerUID changes format across
		// transports); names are display-only.
		current[p.UserID] = p.Name
	}
	prev := st.players
	primed := st.primed
	st.players = current
	st.primed = true
	c.mu.Unlock()

	if c.notifier != nil && wasDown {
		c.notifier.ServerUp(ctx, srv, time.Since(downSince))
	}
	// First-ever observation: baseline only. Whoever was already on when
	// palcon started has no known join time, and inventing one would skew
	// every session that follows.
	if !primed {
		return
	}

	for uid, name := range current {
		if _, ok := prev[uid]; !ok {
			c.recordEvent(ctx, srv.ID, now, uid, name, "join")
			// After downtime the whole list reads as joins — real ones,
			// since their sessions were closed when the server went down —
			// but Discord shouldn't announce a restart as player churn.
			if c.notifier != nil && !wasDown {
				c.notifier.PlayerJoined(ctx, srv, name, len(current))
			}
		}
	}
	for uid, name := range prev {
		if _, ok := current[uid]; !ok {
			c.recordEvent(ctx, srv.ID, now, uid, name, "leave")
			if c.notifier != nil && !wasDown {
				c.notifier.PlayerLeft(ctx, srv, name, len(current))
			}
		}
	}
}

func (c *Collector) recordEvent(ctx context.Context, serverID int64, at time.Time, userID, name, event string) {
	if err := c.store.InsertPlayerEvent(ctx, serverID, at, userID, name, event); err != nil {
		c.logger.Error("collector: recording player event", "server", serverID, "event", event, "error", err)
	}
}
