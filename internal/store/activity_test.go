package store

import (
	"context"
	"testing"
	"time"
)

func TestPlayerEventsRoundTripAndPrune(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := context.Background()
	serverID := testServerID(t, st)

	now := time.Now().UTC().Truncate(time.Second)
	old := now.Add(-100 * 24 * time.Hour)
	if err := st.InsertPlayerEvent(ctx, serverID, old, "steam_1", "Kyoshi", "join"); err != nil {
		t.Fatalf("insert old: %v", err)
	}
	if err := st.InsertPlayerEvent(ctx, serverID, now, "steam_1", "Kyoshi", "leave"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	events, err := st.ListPlayerEvents(ctx, serverID, now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(events) != 1 || events[0].Event != "leave" || events[0].Name != "Kyoshi" || !events[0].TS.Equal(now) {
		t.Fatalf("events = %+v, want just the recent leave", events)
	}

	n, err := st.PrunePlayerEvents(ctx, now.Add(-90*24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("prune: n=%d err=%v, want 1 pruned", n, err)
	}
}

// Open sessions are how a restart finds the joins a previous run never got
// to close, so "most recent event is a join" has to survive rejoins.
func TestOpenSessionsAndWatchHeartbeat(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := context.Background()
	serverID := testServerID(t, st)

	now := time.Now().UTC().Truncate(time.Second)
	insert := func(offset time.Duration, uid, name, event string) {
		t.Helper()
		if err := st.InsertPlayerEvent(ctx, serverID, now.Add(offset), uid, name, event); err != nil {
			t.Fatalf("insert %s: %v", event, err)
		}
	}
	// Kyoshi left and came back: still open, at the *second* join.
	insert(-4*time.Hour, "steam_1", "Kyoshi", "join")
	insert(-3*time.Hour, "steam_1", "Kyoshi", "leave")
	insert(-2*time.Hour, "steam_1", "Kyoshi", "join")
	// Rushi logged off for good.
	insert(-3*time.Hour, "steam_2", "Rushi", "join")
	insert(-time.Hour, "steam_2", "Rushi", "leave")

	open, err := st.OpenSessions(ctx, serverID)
	if err != nil {
		t.Fatalf("open sessions: %v", err)
	}
	if len(open) != 1 || open[0].UserID != "steam_1" || !open[0].Since.Equal(now.Add(-2*time.Hour)) {
		t.Fatalf("open = %+v, want only Kyoshi since the rejoin", open)
	}

	if ts, err := st.LastWatch(ctx, serverID); err != nil || !ts.IsZero() {
		t.Fatalf("LastWatch before any probe = %v (err %v), want zero", ts, err)
	}
	for _, at := range []time.Time{now.Add(-time.Minute), now} {
		if err := st.TouchWatch(ctx, serverID, at); err != nil {
			t.Fatalf("touch: %v", err)
		}
	}
	ts, err := st.LastWatch(ctx, serverID)
	if err != nil || !ts.Equal(now) {
		t.Fatalf("LastWatch = %v (err %v), want the latest probe %v", ts, err, now)
	}
}

func TestAuditSurvivesServerDeletion(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := context.Background()
	serverID := testServerID(t, st)

	if err := st.InsertAudit(ctx, serverID, "admin", "power-restart", "palworld"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := st.DeleteServer(ctx, serverID); err != nil {
		t.Fatalf("delete server: %v", err)
	}

	entries, err := st.ListAudit(ctx, serverID, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].Action != "power-restart" || entries[0].Username != "admin" {
		t.Fatalf("entries = %+v, want the surviving power-restart row", entries)
	}
}
