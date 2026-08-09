package dwlog

import (
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
