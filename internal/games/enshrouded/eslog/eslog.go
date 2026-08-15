// Package eslog derives who is on an Enshrouded server from its log
// stream.
//
// Enshrouded has no RCON and no HTTP API, so the log tail the flameagent
// supervisor captures is the live source of player state. This package is
// a small state machine over that tail: peer-add and peer-remove lines
// open and close sessions, a login line names the player, everything else
// is noise by design.
//
// The line vocabulary, captured verbatim from a real server on
// 2026-08-15 (build b466cef15, game version 1024233) with a real client
// joining and leaving — the committed corpus substitutes synthetic ids
// and names, since real player ids never enter the repo:
//
//	[Session] 'HostOnline' (up)!                                    server ready
//	[online] Added peer 0(1) (steamid:76561190000000001)            join
//	[online] Removed peer 0(1)                                      leave
//	[server] Player 'Ember' logged in with Permissions:             the name
//	[server] Start Saving / [server] Saved                          world save
//
// Two properties of the real format shape the design. The join line
// carries the SteamID64 itself, so nothing has to be paired across lines
// to know *who* joined — the id and the peer arrive together. The name
// does not: it appears a few lines later on a `[server] Player '…'` line
// with no peer index on it, so names are attached FIFO to joins still
// awaiting one. A join whose name line never arrives keeps its id and
// stays in the roster; that degrades a label, never the count.
//
// Rules are a versioned table exactly as in the sibling consoles: a game
// patch that changes the format breaks one table, not the package —
// write the next table from a fresh capture. V1 was written from
// community captures of 2024-era builds (`Added Peer #0.`, ids on a
// separate "Session accepted" line, no names anywhere) and matched
// nothing on a current server, which is precisely the failure this
// versioning exists to make cheap; it was removed rather than kept as a
// table no live build emits.
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
	// Peer is the server's own peer index, reused after leaves.
	Peer int
	// Machine is the session's machine index, the second half of the
	// server's `peer 0(1)` notation. Carried for identity only — the peer
	// index is what sessions are keyed on.
	Machine int
	// SteamID is the SteamID64, present on every join line.
	SteamID string
}

// Rules is one version of the log vocabulary. Matchers work on raw lines
// and return ok=false for lines that are not theirs; no rule failure is
// ever fatal to the tracker.
type Rules struct {
	// Version names the capture this table was written against.
	Version string
	// Join extracts a completed join: peer, machine and SteamID64.
	Join func(line string) (Event, bool)
	// Leave extracts the peer index of a disconnect.
	Leave func(line string) (Event, bool)
	// Name extracts the display name from a player-login line.
	Name func(line string) (string, bool)
	// Ready reports the server-online line.
	Ready func(line string) bool
	// SaveStart and SaveDone bracket a world save.
	SaveStart func(line string) bool
	SaveDone  func(line string) bool
}

var (
	joinRe  = regexp.MustCompile(`\[online\] Added peer (\d+)\((\d+)\) \(steamid:(\d+)\)`)
	leaveRe = regexp.MustCompile(`\[online\] Removed peer (\d+)\((\d+)\)`)
	// The "with Permissions" tail is load-bearing: the server also logs
	// `[server] Machine '1': Player '0(0)' logged in`, whose quoted value
	// is a player *handle*, not a name. Requiring the permissions tail is
	// what keeps "0(0)" out of the roster.
	nameRe = regexp.MustCompile(`\[server\] Player '(.+?)' logged in with Permissions`)
)

const (
	readyMarker     = `[Session] 'HostOnline' (up)!`
	saveStartMarker = `[server] Start Saving`
	saveDoneMarker  = `[server] Saved`
)

// RulesV2 is written from the 2026-08-15 capture (build b466cef15).
var RulesV2 = Rules{
	Version: "v2-capture-0.9.1",
	Join: func(line string) (Event, bool) {
		m := joinRe.FindStringSubmatch(line)
		if m == nil {
			return Event{}, false
		}
		peer, err := strconv.Atoi(m[1])
		if err != nil {
			return Event{}, false
		}
		machine, _ := strconv.Atoi(m[2])
		return Event{Peer: peer, Machine: machine, SteamID: m[3]}, true
	},
	Leave: func(line string) (Event, bool) {
		m := leaveRe.FindStringSubmatch(line)
		if m == nil {
			return Event{}, false
		}
		peer, err := strconv.Atoi(m[1])
		if err != nil {
			return Event{}, false
		}
		machine, _ := strconv.Atoi(m[2])
		return Event{Peer: peer, Machine: machine}, true
	},
	Name: func(line string) (string, bool) {
		m := nameRe.FindStringSubmatch(line)
		if m == nil || m[1] == "" {
			return "", false
		}
		return m[1], true
	},
	Ready: func(line string) bool {
		return strings.Contains(line, readyMarker)
	},
	SaveStart: func(line string) bool {
		return strings.Contains(line, saveStartMarker)
	},
	SaveDone: func(line string) bool {
		// Substring, not equality: the marker is the whole line today, but
		// anchoring on that would break on any future prefix.
		return strings.Contains(line, saveDoneMarker)
	},
}

// Session is one tracked player. Name is empty until the login line that
// carries it is seen — the roster shows the SteamID64 until then, which
// is a worse label but never a missing player.
type Session struct {
	Peer    int
	Machine int
	SteamID string
	Name    string
	SeenAt  time.Time
}

// Tracker holds the derived player state for one server process.
type Tracker struct {
	rules Rules

	mu        sync.Mutex
	startedAt time.Time
	prevLast  string // last line of the previous tail, the merge anchor
	sessions  map[int]Session
	// awaitingName holds peers whose login line hasn't been seen yet, in
	// join order — the name arrives a few lines after the join with no
	// index on it, so it goes to the oldest unnamed session.
	awaitingName []int
	ready        bool
	// lastSave is when the server last finished writing the world during
	// this process lifetime.
	lastSave time.Time
	// saving reports a write in flight (Start Saving seen, Saved not yet).
	saving bool
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
// anchor has scrolled out (a busy log between polls — an idle Enshrouded
// server still prints a session table every ten seconds), the whole tail
// is reprocessed instead. Joins and leaves are idempotent map operations,
// so reprocessing old lines is safe; only events that scrolled past
// unseen are lost.
func (t *Tracker) Update(startedAt time.Time, tail []string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !startedAt.Equal(t.startedAt) {
		t.startedAt = startedAt
		t.prevLast = ""
		t.sessions = map[int]Session{}
		t.awaitingName = nil
		t.ready = false
		t.lastSave = time.Time{}
		t.saving = false
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
	if ev, ok := t.rules.Join(line); ok {
		t.sessions[ev.Peer] = Session{
			Peer: ev.Peer, Machine: ev.Machine, SteamID: ev.SteamID, SeenAt: now,
		}
		t.awaitingName = append(t.awaitingName, ev.Peer)
		return
	}
	if ev, ok := t.rules.Leave(line); ok {
		delete(t.sessions, ev.Peer)
		t.awaitingName = dropPeer(t.awaitingName, ev.Peer)
		return
	}
	if name, ok := t.rules.Name(line); ok {
		// Attach to the oldest join still without one. A name with no
		// pending join is dropped rather than guessed onto someone: a
		// mislabelled player is worse than an id-labelled one.
		for len(t.awaitingName) > 0 {
			peer := t.awaitingName[0]
			t.awaitingName = t.awaitingName[1:]
			if s, ok := t.sessions[peer]; ok {
				s.Name = name
				t.sessions[peer] = s
				break
			}
			// That peer left before its name arrived; try the next.
		}
		return
	}
	if t.rules.Ready(line) {
		t.ready = true
		return
	}
	if t.rules.SaveStart(line) {
		t.saving = true
		return
	}
	if t.rules.SaveDone(line) {
		t.saving, t.lastSave = false, now
	}
}

func dropPeer(peers []int, peer int) []int {
	out := peers[:0]
	for _, p := range peers {
		if p != peer {
			out = append(out, p)
		}
	}
	return out
}

// Ready reports whether the server has logged its host-online line this
// lifetime. False also covers a tail that no longer reaches back to boot —
// callers treat it as "unknown", never "down"; process liveness comes from
// the agent, not from log inference.
func (t *Tracker) Ready() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ready
}

// LastSave reports when the server last finished writing the world during
// this process lifetime, and whether one is in flight right now. A zero
// time means none has been seen yet — not that none happened, since the
// tail only reaches back so far.
func (t *Tracker) LastSave() (at time.Time, inFlight bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.lastSave, t.saving
}

// Sessions returns the current players, sorted by peer index for stable
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
