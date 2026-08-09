package sched

import (
	"testing"
	"time"

	"github.com/safwyls/dwcon/internal/store"
)

// mustTime parses a local wall-clock time for test fixtures.
func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	if err != nil {
		t.Fatalf("bad fixture time %q: %v", s, err)
	}
	return ts
}

func sched(days []int, timeOfDay string, warnings []int) *store.RestartSchedule {
	return &store.RestartSchedule{Enabled: true, Days: days, TimeOfDay: timeOfDay, WarningMinutes: warnings}
}

func TestDueEventsFiresRestartAtScheduledMinute(t *testing.T) {
	// 2026-07-20 is a Monday (weekday 1).
	now := mustTime(t, "2026-07-20 05:00:10")
	events := dueEvents(sched([]int{1}, "05:00", []int{15, 5}), now)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1 (the restart): %+v", len(events), events)
	}
	if events[0].Minutes != 0 {
		t.Errorf("expected the restart event, got a %d-minute warning", events[0].Minutes)
	}
}

func TestDueEventsFiresWarningAtLeadTime(t *testing.T) {
	now := mustTime(t, "2026-07-20 04:45:10")
	events := dueEvents(sched([]int{1}, "05:00", []int{15, 5}), now)
	if len(events) != 1 || events[0].Minutes != 15 {
		t.Fatalf("got %+v, want exactly the 15-minute warning", events)
	}
}

func TestDueEventsSkipsUnscheduledDay(t *testing.T) {
	// Tuesday, but only Monday is scheduled.
	now := mustTime(t, "2026-07-21 05:00:10")
	if events := dueEvents(sched([]int{1}, "05:00", nil), now); len(events) != 0 {
		t.Errorf("got %+v, want none on an unscheduled day", events)
	}
}

func TestDueEventsWarningCrossesMidnightIntoUnscheduledDay(t *testing.T) {
	// Restart Monday 00:05; the 15-minute warning lands Sunday 23:50 even
	// though Sunday itself isn't a scheduled day.
	now := mustTime(t, "2026-07-19 23:50:10") // Sunday
	events := dueEvents(sched([]int{1}, "00:05", []int{15}), now)
	if len(events) != 1 || events[0].Minutes != 15 {
		t.Fatalf("got %+v, want the cross-midnight 15-minute warning", events)
	}
}

func TestDueEventsIgnoresStaleEvents(t *testing.T) {
	// Way past the scheduled minute (e.g. process was asleep): nothing may
	// fire, or a 5am restart would run at noon.
	now := mustTime(t, "2026-07-20 12:00:00")
	if events := dueEvents(sched([]int{1}, "05:00", []int{15}), now); len(events) != 0 {
		t.Errorf("got %+v, want none hours after the schedule", events)
	}
}

func TestDueEventsBadTimeOfDay(t *testing.T) {
	now := mustTime(t, "2026-07-20 05:00:10")
	if events := dueEvents(sched([]int{1}, "nonsense", []int{5}), now); len(events) != 0 {
		t.Errorf("got %+v, want none for an unparseable time", events)
	}
}

func TestNextRunFindsUpcomingDay(t *testing.T) {
	// From Monday 06:00, a Mon+Fri 05:00 schedule next runs Friday.
	after := mustTime(t, "2026-07-20 06:00:00")
	next := NextRun(sched([]int{1, 5}, "05:00", nil), after)
	want := mustTime(t, "2026-07-24 05:00:00")
	if !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
}

func TestNextRunLaterSameDay(t *testing.T) {
	after := mustTime(t, "2026-07-20 04:00:00")
	next := NextRun(sched([]int{1}, "05:00", nil), after)
	want := mustTime(t, "2026-07-20 05:00:00")
	if !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
}

func TestNextRunWeekWrap(t *testing.T) {
	// From just after Monday's run, the next Monday-only run is in 7 days.
	after := mustTime(t, "2026-07-20 05:00:01")
	next := NextRun(sched([]int{1}, "05:00", nil), after)
	want := mustTime(t, "2026-07-27 05:00:00")
	if !next.Equal(want) {
		t.Errorf("next = %v, want %v", next, want)
	}
}

func TestNextRunNoDays(t *testing.T) {
	if next := NextRun(sched(nil, "05:00", nil), time.Now()); !next.IsZero() {
		t.Errorf("next = %v, want zero for a schedule with no days", next)
	}
}
