package companion

// A world's history, read from the vault.
//
// Two views are built from one call. **Activity** is what has happened to
// the worlds this machine is linked to — every version anyone checked in,
// newest first. **Conflicts** is the subset flagged `conflict`, which is
// the same list filtered: a conflict is not a separate record, it is a
// version the vault refused to fast-forward onto (savesync.Checkin sets
// it when a check-in arrives from an ended session, or from one whose
// base is no longer the head).
//
// The vault is the only thing that can see either. This machine knows
// what *it* did; it cannot know that someone else checked the same world
// in from another PC, which is exactly what a conflict is.
//
// Read on demand rather than on the custody poll: this is one HTTP call
// per linked world, both tabs are visited rarely, and none of it is
// needed to sync a save. The cache exists so that switching between the
// two tabs, or a re-render, does not re-ask.

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// errNoHistory is what both views get before a vault is configured. The
// same sentence Settings would send them to, rather than an empty list
// that reads as "nothing has ever happened".
var errNoHistory = errors.New("not connected — set the service URL and your token in Settings")

// historyTTL is how long a fetched history stays fresh. Long enough that
// flipping between Activity and Conflicts is free, short enough that the
// tab is not lying about a world someone just checked in.
const historyTTL = 20 * time.Second

// maxHistoryWorlds bounds one refresh. Each world is a request to the
// vault, and a machine with a very long link list should not turn one tab
// switch into a burst — the newest worlds are the ones a history is about.
const maxHistoryWorlds = 24

// HistoryEntry is one version in one world's history, flattened with the
// world's identity so a merged list needs no second lookup.
type HistoryEntry struct {
	WorldID   int64     `json:"worldId"`
	WorldName string    `json:"worldName"`
	GameTitle string    `json:"gameTitle,omitempty"`
	VersionID int64     `json:"versionId"`
	Kind      string    `json:"kind"`
	Conflict  bool      `json:"conflict"`
	Head      bool      `json:"head"`
	Bytes     int64     `json:"bytes"`
	Uploader  string    `json:"uploader,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// History is the merged answer: every version of every linked world,
// newest first, plus whatever could not be read.
type History struct {
	Entries []HistoryEntry `json:"entries"`
	// Failed names the worlds whose history could not be read, and why.
	// A vault that answered for four worlds and failed on the fifth must
	// say so — a short list that looks complete is worse than an error.
	Failed []HistoryFailure `json:"failed"`
	// FetchedAt is when this was read, so the view can age itself.
	FetchedAt time.Time `json:"fetchedAt"`
	// Truncated reports that more worlds are linked than were read.
	Truncated int `json:"truncated,omitempty"`
}

// HistoryFailure is one world the vault would not answer for.
type HistoryFailure struct {
	WorldID   int64  `json:"worldId"`
	WorldName string `json:"worldName"`
	Error     string `json:"error"`
}

// worldDetail is the vault's per-world answer (core/api's
// syncWorldDetail): custody state, the whole version list, and the
// uploader names those versions point at.
type worldDetail struct {
	Status   syncWorldDTO `json:"status"`
	Versions []struct {
		ID         int64     `json:"id"`
		Kind       string    `json:"kind"`
		Conflict   bool      `json:"conflict"`
		Bytes      int64     `json:"bytes"`
		UploaderID *int64    `json:"uploaderId,omitempty"`
		CreatedAt  time.Time `json:"createdAt"`
	} `json:"versions"`
	Uploaders map[string]string `json:"uploaders"`
}

// History returns the merged history of every linked world, fetching it
// if what is cached has aged out.
func (a *App) History() (History, error) {
	a.mu.Lock()
	cached, at := a.history, a.historyAt
	a.mu.Unlock()
	if !at.IsZero() && time.Since(at) < historyTTL {
		return cached, nil
	}
	return a.RefreshHistory()
}

// RefreshHistory re-reads every linked world's history from the vault,
// ignoring the cache. This is what the view's own refresh calls.
func (a *App) RefreshHistory() (History, error) {
	if !a.SyncConfigured() {
		return History{}, errNoHistory
	}

	a.mu.Lock()
	type target struct {
		id   int64
		name string
		game string
	}
	targets := make([]target, 0, len(a.cfg.Links))
	for _, l := range a.cfg.Links {
		// Scanned inline rather than via world(), which takes the lock
		// this loop already holds.
		name := ""
		for i := range a.worldSync.Worlds {
			if a.worldSync.Worlds[i].World.ID == l.WorldID {
				name = a.worldSync.Worlds[i].World.Name
				break
			}
		}
		targets = append(targets, target{id: l.WorldID, name: name, game: l.GameTitle})
	}
	a.mu.Unlock()

	// Newest link last in config order says nothing useful, so bound by
	// world id: the highest ids are the most recently created worlds.
	sort.Slice(targets, func(i, j int) bool { return targets[i].id > targets[j].id })
	truncated := 0
	if len(targets) > maxHistoryWorlds {
		truncated = len(targets) - maxHistoryWorlds
		targets = targets[:maxHistoryWorlds]
	}

	var (
		mu      sync.Mutex
		entries []HistoryEntry
		failed  []HistoryFailure
		wg      sync.WaitGroup
	)
	// Concurrently, but a few at a time: this is somebody's home server
	// on the other end of a tunnel, not a fleet.
	gate := make(chan struct{}, 4)
	for _, t := range targets {
		wg.Add(1)
		go func(t target) {
			defer wg.Done()
			gate <- struct{}{}
			defer func() { <-gate }()

			var out worldDetail
			if err := a.syncDo("GET", fmt.Sprintf("/worlds/%d", t.id), nil, &out); err != nil {
				mu.Lock()
				failed = append(failed, HistoryFailure{WorldID: t.id, WorldName: t.name, Error: err.Error()})
				mu.Unlock()
				return
			}
			name := out.Status.World.Name
			if name == "" {
				name = t.name
			}
			game := out.Status.World.GameTitle
			if game == "" {
				game = t.game
			}
			head := out.Status.World.HeadVersion
			mine := make([]HistoryEntry, 0, len(out.Versions))
			for _, v := range out.Versions {
				e := HistoryEntry{
					WorldID: t.id, WorldName: name, GameTitle: game,
					VersionID: v.ID, Kind: v.Kind, Conflict: v.Conflict,
					Head:  head != nil && *head == v.ID,
					Bytes: v.Bytes, CreatedAt: v.CreatedAt,
				}
				if v.UploaderID != nil {
					e.Uploader = out.Uploaders[fmt.Sprint(*v.UploaderID)]
				}
				mine = append(mine, e)
			}
			mu.Lock()
			entries = append(entries, mine...)
			mu.Unlock()
		}(t)
	}
	wg.Wait()

	// Newest first, and stable on ties: two versions can share a second.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].VersionID > entries[j].VersionID
		}
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})
	sort.Slice(failed, func(i, j int) bool { return failed[i].WorldID < failed[j].WorldID })

	// Empty, not nil: the page reads both as arrays, and the rule this
	// codebase learned the hard way is that a nil slice marshals to null.
	if entries == nil {
		entries = []HistoryEntry{}
	}
	if failed == nil {
		failed = []HistoryFailure{}
	}
	h := History{Entries: entries, Failed: failed, FetchedAt: time.Now(), Truncated: truncated}

	a.mu.Lock()
	a.history, a.historyAt = h, h.FetchedAt
	a.mu.Unlock()
	return h, nil
}

// Conflicts is the history filtered to the versions the vault flagged.
// Derived, never stored: one list, filtered two ways, so Activity and
// Conflicts cannot disagree about what happened.
func (h History) Conflicts() []HistoryEntry {
	out := make([]HistoryEntry, 0, 4)
	for _, e := range h.Entries {
		if e.Conflict {
			out = append(out, e)
		}
	}
	return out
}
