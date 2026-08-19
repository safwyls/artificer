package dwlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Synthetic lines built from the two verified v0 markers (recon doc, "Logs").
// The bracketed prefixes imitate UE's timestamp/frame framing; the markers
// are the only part the parser relies on. Replace with real captures when a
// corpus lands.
const (
	joinVex    = "[2026.08.09-12.00.01:000][  3]LogNet: Join succeeded: Vexmarrow"
	joinBram   = "[2026.08.09-12.05.42:114][881]LogNet: Join succeeded: Bramblejaw"
	joinKae    = "[2026.08.09-12.07.03:551][942]LogNet: Join succeeded: Kaelith"
	leaveBram  = "[2026.08.09-13.01.09:207][412]LogDominionPlayerController: ClientRequestDisconnect Bramblejaw"
	leaveBlank = "[2026.08.09-13.02.00:001][444]LogDominionPlayerController: ClientRequestDisconnect"
	noise      = "[2026.08.09-12.00.02:000][  4]LogWorld: Bringing World /Game/Maps/Ashenfall up for play"
)

func names(t *Tracker) []string {
	var out []string
	for _, s := range t.Sessions() {
		out = append(out, s.Name)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var t0 = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)

func TestJoinLeaveAndNoise(t *testing.T) {
	tr := NewTracker(RulesV0)
	tr.Update(t0, []string{joinVex, noise, joinBram, joinKae, leaveBram})
	if got := names(tr); !equal(got, []string{"Kaelith", "Vexmarrow"}) {
		t.Fatalf("sessions = %v", got)
	}
}

func TestLeaveAttributionByContainedName(t *testing.T) {
	tr := NewTracker(RulesV0)
	// "Bram" must not claim Bramblejaw's disconnect line.
	joinBramShort := "[x]LogNet: Join succeeded: Bram"
	tr.Update(t0, []string{joinBramShort, joinBram, leaveBram})
	if got := names(tr); !equal(got, []string{"Bram"}) {
		t.Fatalf("sessions = %v", got)
	}
}

func TestUnattributableLeaveClosesNothing(t *testing.T) {
	tr := NewTracker(RulesV0)
	tr.Update(t0, []string{joinVex, joinKae, leaveBlank + " for controller 7"})
	// The line names nobody tracked (suffix defeats the marker-only shape's
	// attribution), so both stay — the documented phantom-over-fabrication
	// trade.
	if got := names(tr); len(got) != 2 {
		t.Fatalf("sessions = %v, want both retained", got)
	}
}

func TestIncrementalMergeByAnchor(t *testing.T) {
	tr := NewTracker(RulesV0)
	tr.Update(t0, []string{joinVex, noise})
	// Second poll: ring advanced, overlap includes previous last line.
	tr.Update(t0, []string{joinVex, noise, joinKae})
	tr.Update(t0, []string{noise, joinKae, leaveBlank + " Kaelith"})
	if got := names(tr); !equal(got, []string{"Vexmarrow"}) {
		t.Fatalf("sessions = %v", got)
	}
}

func TestAnchorLostReprocessesIdempotently(t *testing.T) {
	tr := NewTracker(RulesV0)
	tr.Update(t0, []string{joinVex})
	// Ring rolled completely; previous last line is gone. Reprocessing the
	// new tail must keep Vexmarrow (join not re-seen, no leave seen) and add
	// the new join exactly once.
	tr.Update(t0, []string{joinKae, noise})
	if got := names(tr); !equal(got, []string{"Kaelith", "Vexmarrow"}) {
		t.Fatalf("sessions = %v", got)
	}
}

func TestRestartResetsState(t *testing.T) {
	tr := NewTracker(RulesV0)
	tr.Update(t0, []string{joinVex, joinKae})
	tr.Update(t0.Add(time.Hour), []string{joinBram})
	if got := names(tr); !equal(got, []string{"Bramblejaw"}) {
		t.Fatalf("sessions after restart = %v", got)
	}
}

func TestJoinIsIdempotentAndKeepsFirstSeenAt(t *testing.T) {
	tr := NewTracker(RulesV0)
	tr.Update(t0, []string{joinVex})
	first := tr.Sessions()[0].SeenAt
	tr.Update(t0, []string{joinVex, noise, joinVex})
	if got := tr.Sessions(); len(got) != 1 || !got[0].SeenAt.Equal(first) {
		t.Fatalf("sessions = %+v, want one with original SeenAt", got)
	}
}

// The corpus is real server output (testdata/server-lifecycle.log, captured
// 2026-08-09). These assertions are what stop the parser from drifting away
// from the shapes the game actually emits — the synthetic lines above only
// prove the state machine, not the vocabulary.
func TestRulesV0AgainstRealCapture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "server-lifecycle.log"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")

	var saves []string
	for _, line := range lines {
		if slot, ok := RulesV0.Save(line); ok {
			saves = append(saves, slot)
		}
		// Nothing in an idle-server capture may look like a session event:
		// a false join would invent a player nobody can see.
		if name, ok := RulesV0.Join(line); ok {
			t.Errorf("join matched a non-join line: %q -> %q", line, name)
		}
		if _, ok := RulesV0.Leave(line); ok {
			t.Errorf("leave matched a non-leave line: %q", line)
		}
	}
	if len(saves) == 0 {
		t.Fatal("no save lines recognised in the real capture")
	}
	for _, slot := range saves {
		if slot != "World-75058" {
			t.Errorf("save slot = %q, want the world name from the capture", slot)
		}
	}
}

// Every UE line in the capture must survive the tracker untouched: the
// tracker's contract is that unknown lines are noise, not errors.
func TestRealCaptureLeavesNoPhantomPlayers(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "server-lifecycle.log"))
	if err != nil {
		t.Fatal(err)
	}
	tr := NewTracker(RulesV0)
	tr.Update(t0, strings.Split(string(data), "\n"))
	if got := tr.Sessions(); len(got) != 0 {
		t.Errorf("idle-server capture produced %d session(s): %+v", len(got), got)
	}
	at, slot := tr.LastSave()
	if at.IsZero() || slot != "World-75058" {
		t.Errorf("LastSave = (%v, %q), want the capture's save", at, slot)
	}
}

// TestRulesV1AgainstPlayerCapture replays the real player-session capture
// (shapes real, ids and names synthetic — see the file header comment in
// dwlog.go). The full replay must end empty; stopping before the leave
// lines must show exactly one session carrying the real id.
func TestRulesV1AgainstPlayerCapture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "player-session.log"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")

	// Find where the disconnect sequence starts.
	leaveAt := -1
	for i, line := range lines {
		if strings.Contains(line, "ClientRequestDisconnect") {
			leaveAt = i
			break
		}
	}
	if leaveAt < 0 {
		t.Fatal("capture has no disconnect line")
	}

	tr := NewTracker(RulesV1)
	tr.Update(t0, lines[:leaveAt])
	sessions := tr.Sessions()
	if len(sessions) != 1 {
		t.Fatalf("mid-session: %d session(s): %+v", len(sessions), sessions)
	}
	if sessions[0].Name != "Bramblejaw" || sessions[0].ID != "0123456789abcdef0123456789abcdef" {
		t.Errorf("session = %+v, want the capture's id and name", sessions[0])
	}
	at, slot := tr.LastSave()
	if at.IsZero() || slot != "World-75058" {
		t.Errorf("LastSave = (%v, %q)", at, slot)
	}

	tr2 := NewTracker(RulesV1)
	tr2.Update(t0, lines)
	if got := tr2.Sessions(); len(got) != 0 {
		t.Errorf("full replay left %d session(s): %+v", len(got), got)
	}
}

// TestRulesV1JoinNotDoubled: the capture emits both "Player ADDED" and
// "Join succeeded" for one join, 1 ms apart. v1 must produce one session,
// keyed by id, not one per pattern.
func TestRulesV1JoinNotDoubled(t *testing.T) {
	tr := NewTracker(RulesV1)
	tr.Update(t0, []string{
		"[x][5]LogDomMatcherSession: Player ADDED to session [aaaa000000000000000000000000aaaa]-[Vexmarrow]",
		"[x][5]LogNet: Join succeeded: Vexmarrow",
	})
	sessions := tr.Sessions()
	if len(sessions) != 1 || sessions[0].ID != "aaaa000000000000000000000000aaaa" {
		t.Fatalf("sessions = %+v, want exactly one keyed by id", sessions)
	}
}

// TestRulesV1DisconnectAloneCloses: a crash path might emit the
// ClientRequestDisconnect shape without the Removed line. Its Account id
// (platform-tagged "XP:<id>") must land on the same key the ADDED line
// opened.
func TestRulesV1DisconnectAloneCloses(t *testing.T) {
	tr := NewTracker(RulesV1)
	tr.Update(t0, []string{
		"[x][5]LogDomMatcherSession: Player ADDED to session [aaaa000000000000000000000000aaaa]-[Vexmarrow]",
		"[x][9]LogDominionPlayerController: ClientRequestDisconnect : DisconnectMe : PlayerStateSave result[true] - state saved for Account[XP:aaaa000000000000000000000000aaaa] Character Name[Vexmarrow] Guid[DCG:BBBB000000000000000000000000BBBB] Type[0]",
	})
	if got := tr.Sessions(); len(got) != 0 {
		t.Fatalf("sessions = %+v, want none after disconnect", got)
	}
}

// TestCharacterNamesFromDisconnect pins the guid → name harvest: the
// disconnect line pairs the character guid (the id the world save's
// transform records use) with the display name, and that knowledge must
// survive a server restart — identity is not liveness.
func TestCharacterNamesFromDisconnect(t *testing.T) {
	const disconnect = "[2026.08.09-21.46.39:311][131]LogDominionPlayerController: ClientRequestDisconnect : DisconnectMe : PlayerStateSave result[true] - state saved for Account[XP:0123456789abcdef0123456789abcdef] Character Name[Bramblejaw] Guid[DCG:0123456789abcdef0123456789ABCDEF] Type[0]"

	tr := NewTracker(RulesV1)
	start := time.Now()
	tr.Update(start, []string{disconnect})

	names := tr.CharacterNames()
	// Uppercased on the way in: the transform records render uppercase.
	if got := names["0123456789ABCDEF0123456789ABCDEF"]; got != "Bramblejaw" {
		t.Fatalf("CharacterNames = %v", names)
	}

	// A restart clears sessions but keeps the learned identities.
	tr.Update(start.Add(time.Hour), []string{noise})
	if len(tr.Sessions()) != 0 {
		t.Errorf("sessions survived a restart: %v", tr.Sessions())
	}
	if got := tr.CharacterNames()["0123456789ABCDEF0123456789ABCDEF"]; got != "Bramblejaw" {
		t.Error("character identity lost on restart")
	}

	// The returned map is a copy, not a window into the tracker.
	names["0123456789ABCDEF0123456789ABCDEF"] = "clobbered"
	if got := tr.CharacterNames()["0123456789ABCDEF0123456789ABCDEF"]; got != "Bramblejaw" {
		t.Error("CharacterNames leaked its internal map")
	}
}
