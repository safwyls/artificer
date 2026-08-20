package dwapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/safwyls/artificer/core/store"
	"github.com/safwyls/artificer/games/dragonwilds/dwsave"
)

// The console's memory of character sheets.
//
// The server caches a player's full character record — skills, inventory,
// vitals — only while that player is connected, and drops it when they
// leave (observed on a live server, 2026-08-20; see the recon's "Where
// player state lives"). So a world save read at any moment carries full
// sheets for whoever is on and bare transform records for everyone else,
// and a console that showed only what the current save says would watch
// each sheet evaporate at logout.
//
// This file keeps the last full record seen for each character and
// restores it when the save stops carrying one. A restored sheet is
// stamped with when it was true (SeenAt / SharedAt) rather than passed
// off as current: the position beside it still comes from the save's live
// transform record, which outlives the session.
//
// Sheets the console read from the world save are **persisted** (core's
// neutral game_state table): they are observations of a moment that will
// not come back until that player logs in again, so losing them to a
// console restart would empty the view for a group that plays weekly.
// Sheets a player *shared* through the companion app are memory-only, on
// purpose — revoking sharing must be able to forget them completely, and
// the companion re-sends within minutes of a restart anyway.

const (
	// maxRememberedRecords bounds the memory per server: a world holds six
	// players, with headroom for characters who have come and gone. The
	// oldest sheet gives way when a new character arrives at the cap.
	maxRememberedRecords = 16
	// sheetScope namespaces this game's rows in core's game_state table.
	sheetScope = "dragonwilds.character_sheet"
)

// rememberedRecord is one character sheet the console has seen, with the
// instant it was true and where it came from. Provenance is load-bearing:
// revoking companion sharing must forget what players shared, while the
// sheets the save itself carried are the console's own reading and stay.
type rememberedRecord struct {
	player        dwsave.PlayerCharacter
	seenAt        time.Time
	fromCompanion bool
}

// storedSheet is the persisted shape: the sheet plus when it was true.
// The value column carries both so a reload needs no second lookup.
type storedSheet struct {
	SeenAt time.Time              `json:"seenAt"`
	Player dwsave.PlayerCharacter `json:"player"`
}

type recordMemory struct {
	// store persists save-read sheets; nil keeps everything in memory
	// (the shape tests use when persistence isn't what they're pinning).
	store  *store.Store
	logger *slog.Logger

	mu       sync.Mutex
	byServer map[int64]map[string]rememberedRecord
	loaded   map[int64]bool
}

func newRecordMemory(st *store.Store, logger *slog.Logger) *recordMemory {
	if logger == nil {
		logger = slog.Default()
	}
	return &recordMemory{
		store:    st,
		logger:   logger,
		byServer: map[int64]map[string]rememberedRecord{},
		loaded:   map[int64]bool{},
	}
}

// ensureLoaded reads a server's persisted sheets once per process. A
// failure is logged and forgotten: the console still works from whatever
// the current save carries, which is the pre-persistence behaviour.
func (m *recordMemory) ensureLoaded(ctx context.Context, serverID int64) {
	m.mu.Lock()
	done := m.store == nil || m.loaded[serverID]
	m.mu.Unlock()
	if done {
		return
	}

	entries, err := m.store.ListGameState(ctx, serverID, sheetScope)
	if err != nil {
		m.logger.Warn("reading remembered character sheets", "server", serverID, "error", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.loaded[serverID] { // another request got here first
		return
	}
	recs := m.byServer[serverID]
	if recs == nil {
		recs = map[string]rememberedRecord{}
		m.byServer[serverID] = recs
	}
	// Newest first from the store, so the cap keeps the freshest.
	for _, e := range entries {
		if len(recs) >= maxRememberedRecords {
			break
		}
		var sheet storedSheet
		if err := json.Unmarshal(e.Value, &sheet); err != nil {
			m.logger.Warn("discarding unreadable remembered sheet", "server", serverID, "key", e.Key, "error", err)
			continue
		}
		if prev, ok := recs[e.Key]; ok && !sheet.SeenAt.After(prev.seenAt) {
			continue
		}
		recs[e.Key] = rememberedRecord{player: sheet.Player, seenAt: sheet.SeenAt}
	}
	m.loaded[serverID] = true
}

// remember stores a sheet if it is newer than the one already held. At
// the cap a new character displaces the stalest sheet rather than being
// turned away — the interesting characters are the ones playing now.
func (m *recordMemory) remember(ctx context.Context, serverID int64, guid string, p dwsave.PlayerCharacter, seenAt time.Time, fromCompanion bool) {
	m.mu.Lock()
	recs := m.byServer[serverID]
	if recs == nil {
		recs = map[string]rememberedRecord{}
		m.byServer[serverID] = recs
	}
	if prev, known := recs[guid]; known && !seenAt.After(prev.seenAt) {
		m.mu.Unlock()
		return
	}
	evicted := ""
	if _, known := recs[guid]; !known && len(recs) >= maxRememberedRecords {
		evicted = stalestKey(recs)
		delete(recs, evicted)
	}
	recs[guid] = rememberedRecord{player: p, seenAt: seenAt, fromCompanion: fromCompanion}
	m.mu.Unlock()

	if m.store == nil {
		return
	}
	if evicted != "" {
		if err := m.store.DeleteGameState(ctx, serverID, sheetScope, evicted); err != nil {
			m.logger.Warn("dropping evicted character sheet", "server", serverID, "error", err)
		}
	}
	// Only what the console read for itself is persisted; shared sheets
	// stay in memory so a revoke can forget them completely.
	if fromCompanion {
		return
	}
	value, err := json.Marshal(storedSheet{SeenAt: seenAt, Player: p})
	if err != nil {
		return
	}
	if err := m.store.PutGameState(ctx, serverID, sheetScope, guid, value, seenAt); err != nil {
		m.logger.Warn("remembering character sheet", "server", serverID, "error", err)
	}
}

// stalestKey names the oldest sheet held, for eviction.
func stalestKey(recs map[string]rememberedRecord) string {
	oldest, at := "", time.Time{}
	for guid, rec := range recs {
		if oldest == "" || rec.seenAt.Before(at) {
			oldest, at = guid, rec.seenAt
		}
	}
	return oldest
}

// lookup returns the remembered sheet for a character, if any.
func (m *recordMemory) lookup(serverID int64, guid string) (rememberedRecord, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.byServer[serverID][guid]
	return rec, ok
}

// forgetCompanionSourced drops the sheets that arrived by companion push,
// leaving what the console read from the save. Called when an admin
// revokes sharing: a revoke that left shared sheets on screen would be a
// lie to the players who shared them. Nothing to clean up in the store —
// shared sheets are never written there.
func (m *recordMemory) forgetCompanionSourced(serverID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for guid, rec := range m.byServer[serverID] {
		if rec.fromCompanion {
			delete(m.byServer[serverID], guid)
		}
	}
}

// hasSheet reports whether a parsed player carries a full character
// record rather than a bare transform. Skills are the discriminator: the
// game writes none for a character it only knows a position for, and no
// real character has an empty skill list (every one starts at level 1 in
// twelve skills).
func hasSheet(p dwsave.PlayerCharacter) bool { return len(p.Skills) > 0 }

// withKnownRecords is the whole merge: it remembers the sheets this save
// carries, then fills in the rest from memory or from companion pushes,
// whichever snapshot is fresher. The save's own transform position and
// freshness always win — they are host-side and current even when the
// sheet beside them is a memory.
func (h *handlers) withKnownRecords(ctx context.Context, srv *store.Server, world *dwsave.World) *dwsave.World {
	if world == nil || (h.records == nil && h.companion == nil) {
		return world
	}
	if h.records != nil {
		h.records.ensureLoaded(ctx, srv.ID)
	}

	out := *world
	out.Players = make([]dwsave.PlayerCharacter, len(world.Players))
	copy(out.Players, world.Players)

	// The save's own mtime is when its sheets were true — not now, since
	// the game autosaves every ~5 minutes.
	savedAt := world.ModTime

	index := make(map[string]int, len(out.Players))
	for i, p := range out.Players {
		guid := dwsave.CanonicalGuid(p.CharGuid)
		index[guid] = i
		// A sheet in the save means that player was connected as of this
		// save; remember it for after they log off.
		if h.records != nil && guid != "" && hasSheet(p) {
			h.records.remember(ctx, srv.ID, guid, p, savedAt, false)
		}
	}

	// Companion pushes, parsed against this world so playtime resolves.
	shared := map[string]dwsave.PlayerCharacter{}
	sharedAt := map[string]time.Time{}
	if h.companion != nil {
		for guid, rec := range h.companion.snapshot(srv.ID) {
			p, err := dwsave.ParseCharacterRecord(rec.raw, world.SaveGuid)
			if err != nil {
				continue // validated at push time; a failure here is a bug, not a 500
			}
			shared[guid] = *p
			sharedAt[guid] = rec.receivedAt
			if h.records != nil {
				h.records.remember(ctx, srv.ID, guid, *p, rec.receivedAt, true)
			}
		}
	}

	// Fill in every player the save left as a bare transform.
	for i := range out.Players {
		if hasSheet(out.Players[i]) {
			continue // the save's current sheet: nothing is fresher
		}
		guid := dwsave.CanonicalGuid(out.Players[i].CharGuid)
		if guid == "" {
			continue
		}
		best, at, source := bestSnapshot(h, srv.ID, guid, shared, sharedAt)
		if source == sourceNone {
			continue
		}
		restore(&out.Players[i], best, at, source)
	}

	// Characters the world save has never placed — a player who has
	// shared a sheet but never appeared in a transform record.
	appended := make([]string, 0, len(shared))
	for guid := range shared {
		if _, ok := index[guid]; !ok {
			appended = append(appended, guid)
		}
	}
	sort.Strings(appended) // deterministic order
	for _, guid := range appended {
		p := shared[guid]
		at := sharedAt[guid]
		p.SharedAt = &at
		out.Players = append(out.Players, p)
	}
	return &out
}

type snapshotSource int

const (
	sourceNone snapshotSource = iota
	sourceShared
	sourceRemembered
)

// bestSnapshot picks the freshest sheet available for a character.
func bestSnapshot(h *handlers, serverID int64, guid string,
	shared map[string]dwsave.PlayerCharacter, sharedAt map[string]time.Time,
) (dwsave.PlayerCharacter, time.Time, snapshotSource) {
	var (
		best   dwsave.PlayerCharacter
		at     time.Time
		source = sourceNone
	)
	if p, ok := shared[guid]; ok {
		best, at, source = p, sharedAt[guid], sourceShared
	}
	if h.records != nil {
		if rec, ok := h.records.lookup(serverID, guid); ok && (source == sourceNone || rec.seenAt.After(at)) {
			best, at, source = rec.player, rec.seenAt, sourceRemembered
		}
	}
	return best, at, source
}

// restore overlays a remembered sheet onto a bare transform record,
// keeping what the save still knows first-hand and stamping the sheet
// with when it was true.
func restore(dst *dwsave.PlayerCharacter, sheet dwsave.PlayerCharacter, at time.Time, source snapshotSource) {
	position, lastUpdated, name := dst.Position, dst.LastUpdated, dst.CharName

	*dst = sheet
	if position != nil {
		dst.Position = position
	}
	dst.LastUpdated = lastUpdated
	// A name the log taught us beats one from an older sheet only when
	// the sheet has none; the sheet's own name is the character's.
	if dst.CharName == "" {
		dst.CharName = name
	}
	stamp := at
	switch source {
	case sourceShared:
		dst.SharedAt = &stamp
		dst.SeenAt = nil
	case sourceRemembered:
		// A remembered sheet may itself have come from a companion push;
		// SeenAt is when the console last held it as true either way.
		dst.SeenAt = &stamp
		dst.SharedAt = nil
	}
}
