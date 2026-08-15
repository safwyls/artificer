package eslog

import (
	"testing"
	"time"
)

// Lines are verbatim from the 2026-08-15 capture (build b466cef15) with
// synthetic SteamIDs and names substituted in. Note what the real format
// does *not* have: no timestamp or level prefix, lowercase "peer", the id
// on the join line itself, and a name on a separate `[server] Player`
// line a few lines later.
const (
	lineReady   = `[Session] 'HostOnline' (up)!`
	lineAccept1 = `[online] Session accepted with peer (steamid:76561190000000001)`
	lineJoin1   = `[online] Added peer 0(1) (steamid:76561190000000001)`
	lineAuth1   = `[online] Client '76561190000000001' authenticated by steam`
	lineHandle1 = `[server] Machine '1': Player '0(0)' logged in`
	lineName1   = `[server] Player 'Ember' logged in with Permissions:`
	lineJoin2   = `[online] Added peer 1(2) (steamid:76561190000000002)`
	lineName2   = `[server] Player 'Wren' logged in with Permissions:`
	lineDisc1   = `[online] Disconnecting peer 0(1)`
	lineLeave1  = `[online] Removed peer 0(1)`
	lineRemove1 = `[server] Remove Player 'Ember'`
	lineSaving  = `[server] Start Saving`
	lineSaved   = `[server] Saved`
	lineNoise   = `[Server][Water] Added Water Dispenser: id: [ 3380609024 ] pos: [ 8576, 1713, 14898 ]`
	lineSession = `m#1(129): up 60 (84), down 19 (30), remote 61 (78), limit 453, lost 0, ping 11 ms, OperatingNormally`
)

func startAt() time.Time { return time.Date(2026, 8, 15, 21, 6, 34, 0, time.UTC) }

// The whole point: a real join sequence produces one named session. The
// previous rules table matched none of these lines, and the dashboard
// showed an empty roster on a server with someone playing on it.
func TestRealJoinSequenceProducesANamedSession(t *testing.T) {
	tr := NewTracker(RulesV2)
	tr.Update(startAt(), []string{
		lineNoise, lineReady, lineSession,
		lineAccept1, lineJoin1, lineAuth1, lineHandle1, lineName1,
	})

	got := tr.Sessions()
	if len(got) != 1 {
		t.Fatalf("sessions = %+v, want 1", got)
	}
	if got[0].SteamID != "76561190000000001" {
		t.Errorf("steam id = %q", got[0].SteamID)
	}
	if got[0].Name != "Ember" {
		t.Errorf("name = %q, want the login line's name", got[0].Name)
	}
	if got[0].Peer != 0 || got[0].Machine != 1 {
		t.Errorf("peer/machine = %d/%d, want 0/1", got[0].Peer, got[0].Machine)
	}
	if !tr.Ready() {
		t.Error("the HostOnline line should mark the server ready")
	}
}

// The handle line quotes a player *handle* — `Player '0(0)'` — and must
// never be mistaken for a name, or the roster labels someone "0(0)".
func TestPlayerHandleLineIsNotAName(t *testing.T) {
	tr := NewTracker(RulesV2)
	tr.Update(startAt(), []string{lineJoin1, lineHandle1})

	got := tr.Sessions()
	if len(got) != 1 {
		t.Fatalf("sessions = %+v", got)
	}
	if got[0].Name != "" {
		t.Errorf("name = %q, want it left empty until the real login line", got[0].Name)
	}
}

func TestLeaveClosesTheSession(t *testing.T) {
	tr := NewTracker(RulesV2)
	tr.Update(startAt(), []string{lineJoin1, lineName1})
	// The disconnect emits several lines; only the peer removal closes it.
	tr.Update(startAt(), []string{lineJoin1, lineName1, lineSaving, lineSaved, lineRemove1, lineDisc1, lineLeave1})

	if got := tr.Sessions(); len(got) != 0 {
		t.Fatalf("after leave: %+v, want none", got)
	}
	if at, inFlight := tr.LastSave(); at.IsZero() || inFlight {
		t.Errorf("save state = %v/%v, want a finished save", at, inFlight)
	}
}

// Two players, two names, in join order — the names carry no peer index,
// so this is the property that keeps them on the right rows.
func TestNamesAttachInJoinOrder(t *testing.T) {
	tr := NewTracker(RulesV2)
	tr.Update(startAt(), []string{lineJoin1, lineName1, lineJoin2, lineName2})

	got := tr.Sessions()
	if len(got) != 2 {
		t.Fatalf("sessions = %+v", got)
	}
	if got[0].Name != "Ember" || got[1].Name != "Wren" {
		t.Errorf("names landed wrong: %+v", got)
	}
}

// Interleaved joins: both Added lines before either name line. The names
// still land in join order, which is the order the server emits them.
func TestInterleavedJoinsKeepNameOrder(t *testing.T) {
	tr := NewTracker(RulesV2)
	tr.Update(startAt(), []string{lineJoin1, lineJoin2, lineName1, lineName2})

	got := tr.Sessions()
	if len(got) != 2 || got[0].Name != "Ember" || got[1].Name != "Wren" {
		t.Errorf("sessions = %+v", got)
	}
}

// A player who leaves before their name line arrives must not hand their
// pending slot to the next player's name.
func TestNameSkipsAPeerThatAlreadyLeft(t *testing.T) {
	tr := NewTracker(RulesV2)
	tr.Update(startAt(), []string{lineJoin1, lineLeave1, lineJoin2, lineName2})

	got := tr.Sessions()
	if len(got) != 1 {
		t.Fatalf("sessions = %+v", got)
	}
	if got[0].SteamID != "76561190000000002" || got[0].Name != "Wren" {
		t.Errorf("the surviving session = %+v, want Wren's own name", got[0])
	}
}

// The tail is the agent's whole ring, re-sent every poll; the anchor keeps
// replays from double-counting, and a reused peer index after a leave is a
// fresh session, not the old one back.
func TestIncrementalTailAndPeerIndexReuse(t *testing.T) {
	tr := NewTracker(RulesV2)
	tr.Update(startAt(), []string{lineJoin1, lineName1})
	tr.Update(startAt(), []string{lineJoin1, lineName1, lineLeave1})
	if got := tr.Sessions(); len(got) != 0 {
		t.Fatalf("after leave: %+v", got)
	}
	// A different player lands on the reused peer index 0.
	tr.Update(startAt(), []string{
		lineJoin1, lineName1, lineLeave1,
		`[online] Added peer 0(3) (steamid:76561190000000003)`,
		`[server] Player 'Rook' logged in with Permissions:`,
	})
	got := tr.Sessions()
	if len(got) != 1 || got[0].SteamID != "76561190000000003" || got[0].Name != "Rook" {
		t.Errorf("reused peer index = %+v, want the third player", got)
	}
}

// A changed process start time means a new server lifetime: every prior
// session is over whether or not a leave line said so.
func TestRestartResetsState(t *testing.T) {
	tr := NewTracker(RulesV2)
	tr.Update(startAt(), []string{lineReady, lineJoin1, lineName1, lineSaving, lineSaved})
	if len(tr.Sessions()) != 1 {
		t.Fatal("setup failed")
	}
	tr.Update(startAt().Add(time.Hour), []string{lineNoise})

	if got := tr.Sessions(); len(got) != 0 {
		t.Errorf("a restart kept sessions alive: %+v", got)
	}
	if tr.Ready() {
		t.Error("a restart kept the ready flag")
	}
	if at, _ := tr.LastSave(); !at.IsZero() {
		t.Error("a restart kept the save timestamp")
	}
}

// A save in flight is distinguishable from a finished one — the game
// brackets the write, and a snapshot taken mid-write is the one thing
// worth knowing about.
func TestSaveInFlightIsVisible(t *testing.T) {
	tr := NewTracker(RulesV2)
	tr.Update(startAt(), []string{lineSaving})
	at, inFlight := tr.LastSave()
	if !inFlight || !at.IsZero() {
		t.Errorf("mid-save state = %v/%v, want in-flight and no completion", at, inFlight)
	}
	tr.Update(startAt(), []string{lineSaving, lineSaved})
	if at, inFlight = tr.LastSave(); inFlight || at.IsZero() {
		t.Errorf("after Saved = %v/%v", at, inFlight)
	}
}

// The boot log is thousands of lines of world setup and a session table
// every ten seconds. None of it may move the roster.
func TestNoiseIsIgnored(t *testing.T) {
	tr := NewTracker(RulesV2)
	tr.Update(startAt(), []string{
		lineNoise, lineSession,
		`[online] Begin auth session with peer 0(1)`,
		`[session] Add remote machine index 1 (id: 2337126944).`,
		`[server] Remove Entity for Player 'Ember'`,
		`[ecss] Stats: Upd:3,600 (16.7ms) Time:60,020ms`,
	})
	if got := tr.Sessions(); len(got) != 0 {
		t.Errorf("noise produced sessions: %+v", got)
	}
}
