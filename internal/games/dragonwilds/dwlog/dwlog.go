// Package dwlog derives who is on a Dragonwilds server from its log stream.
//
// Dragonwilds has no RCON, no HTTP API and no query protocol; the log tail
// the palagent supervisor captures is the only live source of player state.
// This package is a small state machine over that tail: join and leave lines
// open and close sessions, everything else is noise by design.
//
// Rules are versioned tables (see RulesV0) so a game patch that changes the
// log format breaks one table, not the package: write the next table from a
// fresh capture, leave the old one for older installs. The v0 markers come
// from docs/dragonwilds-recon.md — two patterns verified in production by
// community tooling, with the full line shapes (ids, timestamps) still
// unverified. Consequences v0 lives with, until a committed corpus upgrades
// it: players are keyed by name (no id appears in a verified line), and a
// leave line is attributed by searching it for a tracked name — one that
// names nobody we know closes nothing rather than guessing.
//
// The tracker never accumulates across process lifetimes: a changed process
// start time resets it, because the server loads a world fresh and every
// prior session is over whether or not a leave line said so.
package dwlog

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Rules is one version of the log vocabulary. Matchers work on raw lines and
// return ok=false for lines that are not theirs; no rule failure is ever
// fatal to the tracker.
type Rules struct {
	// Version names the capture this table was written against.
	Version string
	// Join returns the joining player's name.
	Join func(line string) (name string, ok bool)
	// Leave reports that a line is a disconnect. The name is best-effort:
	// empty means the line doesn't identify the player in a form this table
	// understands, and the tracker will try to attribute it to a tracked
	// name appearing in the line.
	Leave func(line string) (name string, ok bool)
	// Save reports that the world was written to disk, returning the slot
	// (world) name. Unlike join/leave this one is verified against a real
	// capture — see testdata/server-lifecycle.log.
	Save func(line string) (slot string, ok bool)
}

// v0 markers, verified only as substrings (recon doc, "Logs"):
//
//	LogNet: Join succeeded: <name>
//	LogDominionPlayerController: ClientRequestDisconnect
const (
	joinMarkerV0  = "LogNet: Join succeeded:"
	leaveMarkerV0 = "LogDominionPlayerController: ClientRequestDisconnect"
	// saveMarkerV0 is verified: the server emits it on every autosave and
	// on world creation. The slot name follows in parentheses.
	saveMarkerV0 = "Save completed SUCCESSFULLY (slot: "
)

// RulesV0 is written against game 0.12.1.4 community captures, not a corpus
// of our own. The join name is everything after the marker; the leave line
// is assumed not to carry a name in a known position.
var RulesV0 = Rules{
	Version: "v0-community-0.12",
	Join: func(line string) (string, bool) {
		i := strings.Index(line, joinMarkerV0)
		if i < 0 {
			return "", false
		}
		name := strings.TrimSpace(line[i+len(joinMarkerV0):])
		if name == "" {
			return "", false
		}
		return name, true
	},
	Leave: func(line string) (string, bool) {
		if !strings.Contains(line, leaveMarkerV0) {
			return "", false
		}
		return "", true
	},
	Save: func(line string) (string, bool) {
		i := strings.Index(line, saveMarkerV0)
		if i < 0 {
			return "", false
		}
		rest := line[i+len(saveMarkerV0):]
		end := strings.IndexByte(rest, ')')
		if end < 0 {
			return "", false
		}
		return strings.TrimSpace(rest[:end]), true
	},
}

// Session is one tracked player. Name doubles as the identity until a
// verified log line yields a real player id — the collector needs *some*
// stable key per player, and on a 6-player friends server names are unique
// in practice. SeenAt is when this tracker first saw the join line, which is
// poll-time coarse; the log's own timestamps are not parsed in v0.
type Session struct {
	Name   string
	SeenAt time.Time
}

// Tracker holds the derived player state for one server process.
type Tracker struct {
	rules Rules

	mu        sync.Mutex
	startedAt time.Time
	prevLast  string // last line of the previous tail, the merge anchor
	sessions  map[string]Session
	// lastSave is when this tracker last saw the server report a completed
	// save, and which world slot it named. Zero until one is observed —
	// the game does not save on shutdown, so "when did it last save" is
	// the only honest answer to "how much would a restart cost".
	lastSave     time.Time
	lastSaveSlot string
}

// NewTracker builds a tracker over the given rules table.
func NewTracker(rules Rules) *Tracker {
	return &Tracker{rules: rules, sessions: map[string]Session{}}
}

// Update feeds the tracker the current log tail. startedAt is the supervised
// process's start time: when it changes, state resets before any line is
// read, because these lines belong to a new server lifetime.
//
// tail is the agent's whole retained ring, not a delta. New lines are found
// by anchoring on the last line of the previous update; if the anchor has
// scrolled out (a busy log between polls), the whole tail is reprocessed
// instead — joins and leaves are idempotent set operations, so reprocessing
// old lines is safe, and only events that scrolled past unseen are lost.
func (t *Tracker) Update(startedAt time.Time, tail []string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !startedAt.Equal(t.startedAt) {
		t.startedAt = startedAt
		t.prevLast = ""
		t.sessions = map[string]Session{}
		t.lastSave, t.lastSaveSlot = time.Time{}, ""
	}

	lines := tail
	if t.prevLast != "" {
		// Search from the end: with duplicate lines the latest occurrence is
		// the safest anchor (never replays more than necessary).
		for i := len(tail) - 1; i >= 0; i-- {
			if tail[i] == t.prevLast {
				lines = tail[i+1:]
				break
			}
		}
	}
	now := time.Now()
	for _, line := range lines {
		t.apply(line, now)
	}
	if len(tail) > 0 {
		t.prevLast = tail[len(tail)-1]
	}
}

func (t *Tracker) apply(line string, now time.Time) {
	if name, ok := t.rules.Join(line); ok {
		if _, tracked := t.sessions[name]; !tracked {
			t.sessions[name] = Session{Name: name, SeenAt: now}
		}
		return
	}
	if slot, ok := t.rules.Save(line); ok {
		t.lastSave, t.lastSaveSlot = now, slot
		return
	}
	if name, ok := t.rules.Leave(line); ok {
		if name == "" {
			name = t.attribute(line)
		}
		if name != "" {
			delete(t.sessions, name)
		}
		// An unattributable leave closes nothing: a phantom entry in the
		// player list is a visible, recoverable error (it clears on the next
		// restart), while removing the wrong player fabricates a leave event
		// downstream in the collector.
		return
	}
}

// attribute finds the tracked name a leave line mentions, longest name first
// so "Bram" never claims a line about "Bramblejaw".
func (t *Tracker) attribute(line string) string {
	names := make([]string, 0, len(t.sessions))
	for name := range t.sessions {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, name := range names {
		if strings.Contains(line, name) {
			return name
		}
	}
	return ""
}

// LastSave reports when the server last confirmed a completed world save
// during this process lifetime, and the slot it wrote. A zero time means
// none has been seen yet — not that none happened, since the tail only
// reaches back so far.
func (t *Tracker) LastSave() (time.Time, string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastSave, t.lastSaveSlot
}

// Sessions returns the current players, sorted by name for stable output.
func (t *Tracker) Sessions() []Session {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Session, 0, len(t.sessions))
	for _, s := range t.sessions {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
