package main

// The labelling rules, tested where they are cheapest to get wrong.
//
// These are not tests of custody — the engine owns that. They are tests
// of the redesign's one structural promise: that a world's chip, its
// group and its primary action all come from a single custodyOf result,
// and of the formatting rules that decide what a row says.

import (
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/safwyls/artificer/companion"
)

// A headless Fyne app, because anything that measures text needs a
// driver to resolve the font with — fyne.MeasureText dereferences the
// current app, and wrapLines and ellipsize both measure.
func TestMain(m *testing.M) {
	test.NewApp()
	os.Exit(m.Run())
}

func world(id int64, name string) *companion.World {
	w := &companion.World{}
	w.World.ID = id
	w.World.Name = name
	return w
}

func held(w *companion.World, who string, sess int64, expires time.Time, claimable bool) *companion.World {
	w.Holder = &companion.Holder{
		SessionID: sess, Username: who, ExpiresAt: expires, Claimable: claimable,
	}
	return w
}

func TestCustodyOfAndGroup(t *testing.T) {
	now := time.Now()
	soon := now.Add(2 * time.Hour)
	later := now.Add(40 * time.Hour)

	cases := []struct {
		name  string
		link  companion.WorldLink
		world *companion.World
		me    string
		conf  bool
		state custody
		group group
	}{
		{"no world, configured", companion.WorldLink{WorldID: 1}, nil, "me", true, custodyGone, groupGone},
		{"no world, not configured", companion.WorldLink{WorldID: 1}, nil, "me", false, custodyFree, groupFree},
		{"unheld", companion.WorldLink{WorldID: 1}, world(1, "A"), "me", true, custodyFree, groupFree},
		{
			"mine, same session",
			companion.WorldLink{WorldID: 1, SessionID: 7},
			held(world(1, "A"), "me", 7, later, false), "me", true, custodyMine, groupMine,
		},
		{
			"mine, another machine",
			companion.WorldLink{WorldID: 1, SessionID: 9},
			held(world(1, "A"), "me", 7, later, false), "me", true, custodyFetching, groupMine,
		},
		{
			"someone else",
			companion.WorldLink{WorldID: 1},
			held(world(1, "A"), "rook", 3, soon, false), "me", true, custodyHeld, groupHeld,
		},
		{
			"someone else, lapsed",
			companion.WorldLink{WorldID: 1},
			held(world(1, "A"), "rook", 3, now.Add(-time.Hour), true), "me", true, custodyExpired, groupHeld,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := custodyOf(c.link, c.world, c.me, c.conf)
			if got.State != c.state {
				t.Errorf("state = %q, want %q", got.State, c.state)
			}
			if got.group() != c.group {
				t.Errorf("group = %d, want %d", got.group(), c.group)
			}
		})
	}
}

// The chip must exist for every state custodyOf can produce. A state
// with no chip is a row that says nothing about its own custody, which
// is the failure the single-source rule exists to prevent.
func TestEveryCustodyStateHasAChip(t *testing.T) {
	for _, s := range []custody{
		custodyFree, custodyMine, custodyFetching, custodyHeld, custodyExpired, custodyGone,
	} {
		if custodyChip(custodyInfo{State: s}) == nil {
			t.Errorf("no chip for %q", s)
		}
	}
}

func TestHoldPressureOnlyInsideThreeHours(t *testing.T) {
	now := time.Now()
	if c := (custodyInfo{ExpiresAt: now.Add(40 * time.Hour)}); c.urgent(now) {
		t.Error("a hold with 40h left is not pressure")
	}
	if c := (custodyInfo{ExpiresAt: now.Add(2 * time.Hour)}); !c.urgent(now) {
		t.Error("a hold with 2h left is pressure")
	}
	// A hold that has already lapsed shows no countdown: there is
	// nothing left to count, and "0m left" is noise on a row whose chip
	// already says the hold expired.
	if c := (custodyInfo{ExpiresAt: now.Add(-time.Minute)}); c.urgent(now) {
		t.Error("a lapsed hold is not a countdown")
	}
	// No expiry at all must not read as urgent.
	if c := (custodyInfo{}); c.urgent(now) {
		t.Error("no expiry is not a countdown")
	}
}

func TestHeadMetaRendersOnlyWhatExists(t *testing.T) {
	now := time.Date(2026, 8, 22, 18, 30, 0, 0, time.Local)

	if got := headMeta(nil, now); got != "" {
		t.Errorf("no world should render nothing, got %q", got)
	}

	// A world the vault knows but nobody has pushed a save to has a
	// name and nothing else. It must render nothing rather than "v0".
	bare := world(1, "A")
	if got := headMeta(bare, now); got != "" {
		t.Errorf("no head should render nothing, got %q", got)
	}

	full := world(1, "A")
	v := int64(41)
	full.World.HeadVersion = &v
	full.Head = &struct {
		ID        int64     `json:"id"`
		Bytes     int64     `json:"bytes"`
		CreatedAt time.Time `json:"createdAt"`
	}{ID: 3, Bytes: 1932735283, CreatedAt: now.Add(-28 * time.Minute)}
	if got, want := headMeta(full, now), "v41 · 1.8 GB · 28m ago"; got != want {
		t.Errorf("headMeta = %q, want %q", got, want)
	}

	// Version but no head row at all: the version still shows.
	partial := world(1, "A")
	partial.World.HeadVersion = &v
	if got, want := headMeta(partial, now), "v41"; got != want {
		t.Errorf("headMeta = %q, want %q", got, want)
	}
}

func TestFmtDuration(t *testing.T) {
	for _, c := range []struct {
		in   time.Duration
		want string
	}{
		{47*time.Hour + 30*time.Minute, "47h 30m"},
		{2*time.Hour + 12*time.Minute, "2h 12m"},
		{3 * time.Hour, "3h"},
		{45 * time.Minute, "45m"},
		{30 * time.Second, "under a minute"},
	} {
		if got := fmtDuration(c.in); got != c.want {
			t.Errorf("fmtDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestOfflineIsDerivedFromAFailedPoll(t *testing.T) {
	var st companion.State
	if offline(st) {
		t.Error("an unconfigured companion is not offline, it is unconfigured")
	}
	st.Sync.Configured = true
	if offline(st) {
		t.Error("a configured companion with no error is online")
	}
	st.Sync.LastError = "dial tcp: no route to host"
	if !offline(st) {
		t.Error("a configured companion whose last poll failed is offline")
	}
}

// The offline custody line is the one piece of copy that must not go
// stale: it is the reassurance that the hold survives the outage.
func TestOfflineHoldCopyStands(t *testing.T) {
	now := time.Now()
	c := custodyInfo{State: custodyMine, Holder: "me", ExpiresAt: now.Add(41 * time.Hour)}
	got := custodyLine(c, companion.WorldLink{WorldID: 1}, true, now)
	if want := "the hold stands while you are offline — 41h left on the hold"; got != want {
		t.Errorf("custodyLine = %q, want %q", got, want)
	}
}

// A world with no expiry from the service must not produce a dangling
// "— left on the hold".
func TestCustodyLineWithoutExpiry(t *testing.T) {
	now := time.Now()
	c := custodyInfo{State: custodyMine}
	if got, want := custodyLine(c, companion.WorldLink{WorldID: 1}, false, now), "yours"; got != want {
		t.Errorf("custodyLine = %q, want %q", got, want)
	}
}

func TestWrapLinesKeepsTheWholeName(t *testing.T) {
	// Wide enough for anything: one line, unchanged.
	got := wrapLines("RuneScape: Dragonwilds", 10000, 2, szMicro, fyne.TextStyle{})
	if len(got) != 1 || got[0] != "RuneScape: Dragonwilds" {
		t.Errorf("wrapLines = %q, want the whole name on one line", got)
	}
	// Narrow: it must break rather than truncate, and must not exceed
	// the line budget.
	got = wrapLines("RuneScape: Dragonwilds", 60, 2, szMicro, fyne.TextStyle{})
	if len(got) == 0 || len(got) > 2 {
		t.Errorf("wrapLines gave %d lines, want 1..2", len(got))
	}
	if wrapLines("", 100, 2, szMicro, fyne.TextStyle{}) != nil {
		t.Error("an empty name is no lines at all")
	}
}
