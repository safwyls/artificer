package watchdog

import (
	"testing"
	"time"

	"github.com/safwyls/flametender/internal/dockerctl"
)

var t0 = time.Date(2026, 7, 26, 5, 0, 0, 0, time.UTC)

func exited(code int) *dockerctl.State {
	return &dockerctl.State{Status: "exited", Running: false, ExitCode: code}
}

func running(since time.Time) *dockerctl.State {
	return &dockerctl.State{Status: "running", Running: true, StartedAt: since.Format(time.RFC3339Nano)}
}

func TestCrashTriggersRestart(t *testing.T) {
	st := &serverState{}
	if act := evaluate(st, exited(137), t0); act != actRestart {
		t.Fatalf("act = %v, want restart", act)
	}
	if st.strikes != 1 {
		t.Errorf("strikes = %d, want 1", st.strikes)
	}
}

func TestCleanExitLeftAlone(t *testing.T) {
	st := &serverState{strikes: 2, standDown: true}
	if act := evaluate(st, exited(0), t0); act != actNone {
		t.Fatalf("act = %v, want none for a clean stop", act)
	}
	// A clean stop is human intervention: the slate wipes.
	if st.strikes != 0 || st.standDown {
		t.Errorf("state = %+v, want reset", st)
	}
}

func TestCooldownBetweenAttempts(t *testing.T) {
	st := &serverState{}
	evaluate(st, exited(1), t0)
	// Still down one tick later — inside the cooldown, no second attempt.
	if act := evaluate(st, exited(1), t0.Add(30*time.Second)); act != actNone {
		t.Fatalf("act = %v, want none during cooldown", act)
	}
	if act := evaluate(st, exited(1), t0.Add(cooldown+time.Second)); act != actRestart {
		t.Fatalf("act = %v, want restart after cooldown", act)
	}
}

func TestCrashLoopStandsDownAfterMaxStrikes(t *testing.T) {
	st := &serverState{}
	now := t0
	for i := 0; i < maxStrikes; i++ {
		if act := evaluate(st, exited(139), now); act != actRestart {
			t.Fatalf("attempt %d: act = %v, want restart", i+1, act)
		}
		now = now.Add(cooldown + time.Second)
	}
	if act := evaluate(st, exited(139), now); act != actGiveUp {
		t.Fatalf("act = %v, want give-up after %d strikes", maxStrikes, maxStrikes)
	}
	// Give-up is sticky: another exited observation stays quiet instead of
	// re-announcing.
	if act := evaluate(st, exited(139), now.Add(cooldown+time.Second)); act != actNone {
		t.Fatal("stand-down must be sticky")
	}
}

func TestLongUptimeResetsStrikes(t *testing.T) {
	st := &serverState{strikes: 2, standDown: true}
	// Freshly restarted: not yet healthy, nothing resets.
	evaluate(st, running(t0.Add(-time.Minute)), t0)
	if st.strikes != 2 || !st.standDown {
		t.Errorf("state reset too early: %+v", st)
	}
	// Up for healthyAfter: slate wipes, watchdog re-arms.
	evaluate(st, running(t0.Add(-healthyAfter)), t0)
	if st.strikes != 0 || st.standDown {
		t.Errorf("state = %+v, want reset after sustained uptime", st)
	}
}

func TestTransitionalStatesLeftAlone(t *testing.T) {
	st := &serverState{}
	for _, status := range []string{"restarting", "created", "paused"} {
		cs := &dockerctl.State{Status: status, Running: false, ExitCode: 1}
		if act := evaluate(st, cs, t0); act != actNone {
			t.Errorf("status %q: act = %v, want none", status, act)
		}
	}
}
