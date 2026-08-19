// Package dwlog derives who is on a Dragonwilds server from its log stream.
//
// Dragonwilds has no RCON, no HTTP API and no query protocol; the log tail
// the wkagent supervisor captures is the only live source of player state.
// This package is a small state machine over that tail: join and leave lines
// open and close sessions, everything else is noise by design.
//
// Rules are versioned tables (see RulesV1) so a game patch that changes the
// log format breaks one table, not the package: write the next table from a
// fresh capture, leave the old one for older installs. v0 was written from
// community report before any real player had joined; v1 is written from a
// real capture (2026-08-09, game 0.12.1.4 / UE 5.6.1) of a client joining
// and leaving, whose lines carry both the player id and the name. The
// committed corpus keeps the real line shapes with synthetic ids and names
// substituted in — real player ids never enter the repo.
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
// Event is one player identified by a log line. ID is the 32-hex EOS
// ProductUserId when the line carries one, empty when it doesn't; Name is
// the display name. Either may be empty, not both.
type Event struct {
	ID   string
	Name string
}

type Rules struct {
	// Version names the capture this table was written against.
	Version string
	// Join returns the joining player.
	Join func(line string) (Event, bool)
	// Leave reports that a line is a disconnect. The event is best-effort:
	// an empty one means the line doesn't identify the player in a form
	// this table understands, and the tracker will try to attribute it to
	// a tracked name appearing in the line.
	Leave func(line string) (Event, bool)
	// Save reports that the world was written to disk, returning the slot
	// (world) name. Verified against a real capture in both tables — see
	// testdata/server-lifecycle.log.
	Save func(line string) (slot string, ok bool)
	// Character extracts a character-identity pairing from a line: the
	// character guid (DCG, the id the world save's transform records use)
	// and the display name. The disconnect line carries both. Nil when the
	// table's vocabulary has no such line.
	Character func(line string) (guid, name string, ok bool)
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
// is assumed not to carry a name in a known position. Kept for older
// installs; the real capture behind v1 confirmed both markers do occur.
var RulesV0 = Rules{
	Version: "v0-community-0.12",
	Join: func(line string) (Event, bool) {
		i := strings.Index(line, joinMarkerV0)
		if i < 0 {
			return Event{}, false
		}
		name := strings.TrimSpace(line[i+len(joinMarkerV0):])
		if name == "" {
			return Event{}, false
		}
		return Event{Name: name}, true
	},
	Leave: func(line string) (Event, bool) {
		if !strings.Contains(line, leaveMarkerV0) {
			return Event{}, false
		}
		return Event{}, true
	},
	Save: saveV0,
}

func saveV0(line string) (string, bool) {
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
}

// v1 markers, from a real capture of a client joining and leaving
// (2026-08-09, game 0.12.1.4 / UE 5.6.1). The session lines carry the
// player id and name symmetrically:
//
//	LogDomMatcherSession: Player ADDED to session [<32hex>]-[<name>]
//	LogDomMatcherSession: Player Removed from session [<32hex>]-[<name>]
//
// The ADDED/Removed case difference is the game's own. The v0 markers both
// fired in the same capture — "Join succeeded" one millisecond after ADDED,
// ClientRequestDisconnect just before Removed — so v1's Join matches only
// ADDED (a second join pattern would double-add a player under a second
// key), while its Leave also understands the disconnect line
// (Account[XP:<id>] ... Character Name[<name>]) because leave deletes are
// idempotent and a crash path might emit one shape without the other.
const (
	joinMarkerV1   = "LogDomMatcherSession: Player ADDED to session ["
	leaveMarkerV1  = "LogDomMatcherSession: Player Removed from session ["
	leaveAccount   = "Account["
	leaveCharacter = "Character Name["
	leaveCharGuid  = "Guid[DCG:"
)

// sessionPair parses "<id>]-[<name>]" — the text following a v1 session
// marker (which itself ends at the opening bracket).
func sessionPair(rest string) (Event, bool) {
	sep := strings.Index(rest, "]-[")
	if sep < 0 {
		return Event{}, false
	}
	end := strings.LastIndexByte(rest, ']')
	if end <= sep+2 {
		return Event{}, false
	}
	ev := Event{ID: rest[:sep], Name: rest[sep+3 : end]}
	if ev.ID == "" && ev.Name == "" {
		return Event{}, false
	}
	return ev, true
}

// bracketed returns the text inside the first "<label>...]" after label,
// e.g. bracketed(line, "Character Name[") on the disconnect line.
func bracketed(line, label string) string {
	i := strings.Index(line, label)
	if i < 0 {
		return ""
	}
	rest := line[i+len(label):]
	end := strings.IndexByte(rest, ']')
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// RulesV1 is written from testdata/player-session.log — a real capture with
// synthetic ids and names substituted in (real player ids stay out of the
// repo). It keys players by their real id, which is what makes save-side
// matching via CanonicalUID possible at all.
var RulesV1 = Rules{
	Version: "v1-capture-0.12",
	Join: func(line string) (Event, bool) {
		i := strings.Index(line, joinMarkerV1)
		if i < 0 {
			return Event{}, false
		}
		return sessionPair(line[i+len(joinMarkerV1):])
	},
	Leave: func(line string) (Event, bool) {
		if i := strings.Index(line, leaveMarkerV1); i >= 0 {
			if ev, ok := sessionPair(line[i+len(leaveMarkerV1):]); ok {
				return ev, true
			}
			return Event{}, true
		}
		if strings.Contains(line, leaveMarkerV0) {
			ev := Event{Name: bracketed(line, leaveCharacter)}
			// The account id is written platform-tagged ("XP:<id>");
			// the tag is not part of the id the session lines use.
			if acct := bracketed(line, leaveAccount); acct != "" {
				if colon := strings.LastIndexByte(acct, ':'); colon >= 0 {
					acct = acct[colon+1:]
				}
				ev.ID = acct
			}
			return ev, true
		}
		return Event{}, false
	},
	Save: saveV0,
	// The disconnect line pairs the character guid with the name:
	//   ... Character Name[<name>] Guid[DCG:<32HEX>] ...
	// This is the only server-side source of that pairing — character
	// records live on each player's own machine (recon, 2026-08-19), so
	// the world save's transform records carry guids and no names, and
	// this line is what names them.
	Character: func(line string) (string, string, bool) {
		guid := bracketed(line, leaveCharGuid)
		name := bracketed(line, leaveCharacter)
		if guid == "" || name == "" {
			return "", "", false
		}
		return strings.ToUpper(guid), name, true
	},
}

// Session is one tracked player. ID is the player id when the rules table
// extracted one (v1 always does; v0 never can) — empty means Name doubles
// as the identity, which is unique in practice on a 6-player friends
// server. SeenAt is when this tracker first saw the join line, which is
// poll-time coarse; the log's own timestamps are not parsed.
type Session struct {
	ID     string
	Name   string
	SeenAt time.Time
}

// key is what sessions are stored under: the id when there is one, the
// name otherwise. Join and leave lines for the same player must land on
// the same key, which v1's symmetric id-carrying lines guarantee.
func (e Event) key() string {
	if e.ID != "" {
		return e.ID
	}
	return e.Name
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
	// charNames maps character guid (uppercase 32 hex, the DCG id) to
	// display name, learned from disconnect lines. Unlike sessions this
	// deliberately survives process restarts: it is identity knowledge,
	// not liveness — a name learned last week still correctly names that
	// character's transform record in the world save.
	charNames map[string]string
}

// NewTracker builds a tracker over the given rules table.
func NewTracker(rules Rules) *Tracker {
	return &Tracker{rules: rules, sessions: map[string]Session{}, charNames: map[string]string{}}
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
	if t.rules.Character != nil {
		if guid, name, ok := t.rules.Character(line); ok {
			t.charNames[guid] = name
		}
		// Fall through: the same line is usually also a leave.
	}
	if ev, ok := t.rules.Join(line); ok {
		if key := ev.key(); key != "" {
			if _, tracked := t.sessions[key]; !tracked {
				t.sessions[key] = Session{ID: ev.ID, Name: ev.Name, SeenAt: now}
			}
		}
		return
	}
	if slot, ok := t.rules.Save(line); ok {
		t.lastSave, t.lastSaveSlot = now, slot
		return
	}
	if ev, ok := t.rules.Leave(line); ok {
		key := ev.key()
		if _, tracked := t.sessions[key]; !tracked {
			// The line's own identification missed (or names a player we
			// never saw join): fall back to searching it for a tracked name.
			key = t.attribute(line)
		}
		if key != "" {
			delete(t.sessions, key)
		}
		// An unattributable leave closes nothing: a phantom entry in the
		// player list is a visible, recoverable error (it clears on the next
		// restart), while removing the wrong player fabricates a leave event
		// downstream in the collector.
		return
	}
}

// attribute finds the tracked player a leave line mentions by name, longest
// name first so "Bram" never claims a line about "Bramblejaw". Returns the
// session key.
func (t *Tracker) attribute(line string) string {
	keys := make([]string, 0, len(t.sessions))
	for key := range t.sessions {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(t.sessions[keys[i]].Name) > len(t.sessions[keys[j]].Name)
	})
	for _, key := range keys {
		if name := t.sessions[key].Name; name != "" && strings.Contains(line, name) {
			return key
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

// CharacterNames returns the guid → name pairings learned so far. The map
// is a copy; the empty map just means no disconnect line has been seen yet
// (the pairing only appears when a player leaves).
func (t *Tracker) CharacterNames() map[string]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]string, len(t.charNames))
	for k, v := range t.charNames {
		out[k] = v
	}
	return out
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
