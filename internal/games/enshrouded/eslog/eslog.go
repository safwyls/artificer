// Package eslog derives who is on an Enshrouded server from its log
// stream.
//
// Enshrouded has no RCON and no HTTP API; until the A2S query lands
// (roadmap Phase 2), the log tail the flameagent supervisor captures is
// the only live source of player state. This package is a small state
// machine over that tail: peer-add and peer-remove lines open and close
// sessions, everything else is noise by design.
//
// The line vocabulary (docs/enshrouded-recon.md, "Logs") — verbatim
// community captures, not yet re-verified against a server of our own:
//
//	[Session] 'HostOnline' (up)!                                   server ready
//	[online] Session accepted with peer ( id 76561198083236349 ).  join, SteamID64
//	[online] Added Peer #0.                                        join complete
//	[online] Session failed for peer #0 with error 4.              disconnect (ignored)
//	[online] Removed Peer #0.                                      leave
//	[server] Start Saving                                          world save began
//
// Names never appear in the log — the SteamID64 is the whole identity.
// The id arrives on the "Session accepted" line and the peer number on
// the "Added Peer" line that follows, so accepted ids queue FIFO and each
// Added Peer consumes one; simultaneous joins interleave in that order.
//
// Rules are a versioned table (RulesV1) exactly as in the sibling
// consoles: a game patch that changes the format breaks one table, not
// the package — write the next table from a fresh capture and leave this
// one for older installs.
package eslog

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Event is one peer transition extracted from a log line.
type Event struct {
	// Peer is the server's own peer number, reused after leaves.
	Peer int
	// SteamID is the SteamID64 when the line carries one.
	SteamID string
}

// Rules is one version of the log vocabulary. Matchers work on raw lines
// and return ok=false for lines that are not theirs; no rule failure is
// ever fatal to the tracker.
type Rules struct {
	// Version names the capture this table was written against.
	Version string
	// Accepted extracts the SteamID64 from a session-accepted line.
	Accepted func(line string) (string, bool)
	// Added extracts the peer number from an added-peer line.
	Added func(line string) (int, bool)
	// Removed extracts the peer number from a removed-peer line.
	Removed func(line string) (int, bool)
	// Ready reports the server-online line.
	Ready func(line string) bool
	// SaveStart reports that the server began writing the world.
	SaveStart func(line string) bool
}

var (
	acceptedRe = regexp.MustCompile(`\[online\] Session accepted with peer \( id (\d{7,20}) \)`)
	addedRe    = regexp.MustCompile(`\[online\] Added Peer #(\d+)\.`)
	removedRe  = regexp.MustCompile(`\[online\] Removed Peer #(\d+)\.`)
)

const (
	readyMarker = `[Session] 'HostOnline' (up)!`
	saveMarker  = `[server] Start Saving`
)

// RulesV1 is written against community captures of 2024-2025 builds (the
// jsknnr issue tracker's user-posted logs). The recon doc records where
// each line came from.
var RulesV1 = Rules{
	Version: "v1-community-0.9",
	Accepted: func(line string) (string, bool) {
		m := acceptedRe.FindStringSubmatch(line)
		if m == nil {
			return "", false
		}
		return m[1], true
	},
	Added: func(line string) (int, bool) {
		return peerNum(addedRe, line)
	},
	Removed: func(line string) (int, bool) {
		return peerNum(removedRe, line)
	},
	Ready: func(line string) bool {
		return strings.Contains(line, readyMarker)
	},
	SaveStart: func(line string) bool {
		return strings.Contains(line, saveMarker)
	},
}

func peerNum(re *regexp.Regexp, line string) (int, bool) {
	m := re.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

// Session is one tracked player. SteamID may be empty when the Added Peer
// line arrived without a preceding accepted id (a tail that started
// mid-join); the peer number then doubles as the identity for the rest of
// the session.
type Session struct {
	Peer    int
	SteamID string
	SeenAt  time.Time
}

// Tracker holds the derived player state for one server process.
type Tracker struct {
	rules Rules

	mu        sync.Mutex
	startedAt time.Time
	prevLast  string // last line of the previous tail, the merge anchor
	// pending are accepted SteamIDs awaiting their Added Peer line, FIFO.
	pending  []string
	sessions map[int]Session
	ready    bool
	// lastSaveStart is when the tracker last saw the server begin a world
	// save. The completion line is unverified (recon doc), so "began" is
	// the honest fact — the game saves every 10 minutes and on shutdown,
	// making this informational rather than load-bearing.
	lastSaveStart time.Time
}

// NewTracker builds a tracker over the given rules table.
func NewTracker(rules Rules) *Tracker {
	return &Tracker{rules: rules, sessions: map[int]Session{}}
}

// Update feeds the tracker the current log tail. startedAt is the
// supervised process's start time: when it changes, state resets before
// any line is read, because these lines belong to a new server lifetime.
//
// tail is the agent's whole retained ring, not a delta. New lines are
// found by anchoring on the last line of the previous update; if the
// anchor has scrolled out (a busy log between polls), the whole tail is
// reprocessed instead — peer adds and removes are idempotent map
// operations, so reprocessing old lines is safe, and only events that
// scrolled past unseen are lost.
func (t *Tracker) Update(startedAt time.Time, tail []string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !startedAt.Equal(t.startedAt) {
		t.startedAt = startedAt
		t.prevLast = ""
		t.pending = nil
		t.sessions = map[int]Session{}
		t.ready = false
		t.lastSaveStart = time.Time{}
	}

	lines := tail
	if t.prevLast != "" {
		// Search from the end: with duplicate lines the latest occurrence
		// is the safest anchor (never replays more than necessary).
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
	if id, ok := t.rules.Accepted(line); ok {
		// A stale pending id (an accept whose join never completed) is
		// bounded: the queue only grows until the next Added consumes
		// head-first, and resets with the process.
		t.pending = append(t.pending, id)
		return
	}
	if peer, ok := t.rules.Added(line); ok {
		id := ""
		if len(t.pending) > 0 {
			id, t.pending = t.pending[0], t.pending[1:]
		}
		t.sessions[peer] = Session{Peer: peer, SteamID: id, SeenAt: now}
		return
	}
	if peer, ok := t.rules.Removed(line); ok {
		delete(t.sessions, peer)
		return
	}
	if t.rules.Ready(line) {
		t.ready = true
		return
	}
	if t.rules.SaveStart(line) {
		t.lastSaveStart = now
	}
}

// Ready reports whether the server has logged its host-online line this
// lifetime. False also covers a tail that no longer reaches back to boot —
// callers treat it as "unknown", never "down"; process liveness comes from
// the agent, not from log inference (an idle server logs almost nothing).
func (t *Tracker) Ready() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ready
}

// LastSaveStart reports when the server last began writing the world
// during this process lifetime. Zero means none has been seen yet — not
// that none happened, since the tail only reaches back so far.
func (t *Tracker) LastSaveStart() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastSaveStart
}

// Sessions returns the current players, sorted by peer number for stable
// output.
func (t *Tracker) Sessions() []Session {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Session, 0, len(t.sessions))
	for _, s := range t.sessions {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Peer < out[j].Peer })
	return out
}
