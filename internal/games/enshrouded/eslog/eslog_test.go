package eslog

import (
	"testing"
	"time"
)

// Lines are shaped exactly like the community captures the recon doc
// quotes (docs/enshrouded-recon.md, "Logs"): a level-letter timestamp
// prefix, a component tag, then the message. The ids are synthetic.
const (
	lineReady    = `[I 00:00:14,325] [Session] 'HostOnline' (up)!`
	lineAccept1  = `[I 00:03:01,100] [online] Session accepted with peer ( id 76561198000000001 ).`
	lineAdded0   = `[I 00:03:01,220] [online] Added Peer #0.`
	lineAccept2  = `[I 00:05:44,000] [online] Session accepted with peer ( id 76561198000000002 ).`
	lineAdded1   = `[I 00:05:44,180] [online] Added Peer #1.`
	lineFailed0  = `[I 00:41:02,900] [online] Session failed for peer #0 with error 4.`
	lineRemoved0 = `[I 00:41:03,010] [online] Removed Peer #0.`
	lineSave     = `[I 00:10:00,000] [server] Start Saving`
	lineNoise    = `[I 00:00:09,001] [OnlineProviderSteam] 'Initialize' (up)!`
)

func startAt() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) }

func TestJoinAndLeaveDeriveSessions(t *testing.T) {
	tr := NewTracker(RulesV1)
	tr.Update(startAt(), []string{lineNoise, lineReady, lineAccept1, lineAdded0, lineAccept2, lineAdded1})

	got := tr.Sessions()
	if len(got) != 2 {
		t.Fatalf("sessions = %v, want 2", got)
	}
	if got[0].Peer != 0 || got[0].SteamID != "76561198000000001" {
		t.Errorf("peer 0 = %+v", got[0])
	}
	if got[1].Peer != 1 || got[1].SteamID != "76561198000000002" {
		t.Errorf("peer 1 = %+v", got[1])
	}
	if !tr.Ready() {
		t.Error("the HostOnline line should mark the server ready")
	}

	// The disconnect emits a session-failed line before the removal; only
	// the removal closes the session.
	tr.Update(startAt(), []string{lineNoise, lineReady, lineAccept1, lineAdded0, lineAccept2, lineAdded1, lineFailed0, lineRemoved0})
	got = tr.Sessions()
	if len(got) != 1 || got[0].Peer != 1 {
		t.Errorf("after peer 0 left: %v, want only peer 1", got)
	}
}

// The tail is the agent's whole ring, re-sent every poll; the anchor keeps
// replays from double-counting, and a reused peer number after a leave is
// a fresh session, not the old one back.
func TestIncrementalTailAndPeerNumberReuse(t *testing.T) {
	tr := NewTracker(RulesV1)
	tr.Update(startAt(), []string{lineReady, lineAccept1, lineAdded0})
	tr.Update(startAt(), []string{lineReady, lineAccept1, lineAdded0, lineRemoved0})
	if got := tr.Sessions(); len(got) != 0 {
		t.Fatalf("after leave: %v, want none", got)
	}
	// A new player lands on the reused peer #0.
	tr.Update(startAt(), []string{lineReady, lineAccept1, lineAdded0, lineRemoved0, lineAccept2, lineAdded0})
	got := tr.Sessions()
	if len(got) != 1 || got[0].SteamID != "76561198000000002" {
		t.Errorf("reused peer number = %v, want the second player's id", got)
	}
}

// A changed process start time means a new server lifetime: every prior
// session is over whether or not a leave line said so.
func TestRestartResetsState(t *testing.T) {
	tr := NewTracker(RulesV1)
	tr.Update(startAt(), []string{lineReady, lineAccept1, lineAdded0, lineSave})
	if len(tr.Sessions()) != 1 || tr.LastSaveStart().IsZero() {
		t.Fatal("setup failed")
	}
	tr.Update(startAt().Add(time.Hour), []string{lineNoise})
	if got := tr.Sessions(); len(got) != 0 {
		t.Errorf("a restart kept sessions alive: %v", got)
	}
	if tr.Ready() {
		t.Error("a restart kept the ready flag")
	}
	if !tr.LastSaveStart().IsZero() {
		t.Error("a restart kept the save timestamp")
	}
}

// A tail that starts mid-join (the accepted line scrolled out) still
// produces a session — with no id, which the client renders by peer
// number rather than dropping the player.
func TestAddedWithoutAcceptedStillTracks(t *testing.T) {
	tr := NewTracker(RulesV1)
	tr.Update(startAt(), []string{lineAdded0})
	got := tr.Sessions()
	if len(got) != 1 || got[0].SteamID != "" || got[0].Peer != 0 {
		t.Errorf("sessions = %v, want an id-less peer 0", got)
	}
}

// Interleaved simultaneous joins: accepted ids queue FIFO and each Added
// consumes one, which is the order the server emits them in.
func TestSimultaneousJoinsPairInOrder(t *testing.T) {
	tr := NewTracker(RulesV1)
	tr.Update(startAt(), []string{lineAccept1, lineAccept2, lineAdded0, lineAdded1})
	got := tr.Sessions()
	if len(got) != 2 {
		t.Fatalf("sessions = %v", got)
	}
	if got[0].SteamID != "76561198000000001" || got[1].SteamID != "76561198000000002" {
		t.Errorf("pairing out of order: %v", got)
	}
}

func TestSaveStartIsObserved(t *testing.T) {
	tr := NewTracker(RulesV1)
	tr.Update(startAt(), []string{lineSave})
	if tr.LastSaveStart().IsZero() {
		t.Error("the Start Saving line was not observed")
	}
}
