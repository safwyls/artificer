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
