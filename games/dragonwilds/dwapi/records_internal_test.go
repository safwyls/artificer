package dwapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/safwyls/artificer/core/crypto"
	"github.com/safwyls/artificer/core/db"
	"github.com/safwyls/artificer/core/store"
	"github.com/safwyls/artificer/games/dragonwilds/dwsave"
)

const testGuid = "384D68C0479A97B5E99446BAB5A9405D"

// onlineWorld is a save read while the character was connected: the game
// caches their record, so the save carries a full sheet.
func onlineWorld(modTime time.Time, x float64) *dwsave.World {
	return &dwsave.World{
		SaveGuid: "BD93B2B8B4C1407D8F4D52EE93788EF4",
		ModTime:  modTime,
		Players: []dwsave.PlayerCharacter{{
			CharGuid:    testGuid,
			CharName:    "safwyl",
			SaveCount:   309,
			Health:      140,
			Skills:      []dwsave.Skill{{ID: "4zYUGF5u_0KbMLkWJmmBbQ", XP: 12107}},
			Inventory:   []dwsave.Item{{Slot: 0, ID: "NmzyVLMIY0SYZfOAPICeKg", Count: 1}},
			Position:    &dwsave.Position{X: x},
			LastUpdated: 1000,
		}},
	}
}

// offlineWorld is the same world after that player logged off: the server
// dropped their cached record, leaving the transform the world keeps.
func offlineWorld(modTime time.Time, x float64) *dwsave.World {
	return &dwsave.World{
		SaveGuid: "BD93B2B8B4C1407D8F4D52EE93788EF4",
		ModTime:  modTime,
		Players: []dwsave.PlayerCharacter{{
			CharGuid:    testGuid,
			Position:    &dwsave.Position{X: x},
			LastUpdated: 2000,
			Skills:      []dwsave.Skill{},
			Inventory:   []dwsave.Item{},
			Equipment:   []dwsave.Item{},
		}},
	}
}

func testHandlers() *handlers {
	return &handlers{companion: newCompanionInbox(), records: newRecordMemory(nil, nil)}
}

// storeHandlers is the same wiring with persistence behind it, as a
// console main builds it.
func storeHandlers(t *testing.T, st *store.Store) *handlers {
	t.Helper()
	return &handlers{companion: newCompanionInbox(), records: newRecordMemory(st, nil)}
}

// TestSheetSurvivesLogout is the behaviour the live server forced: the
// game caches a character's sheet only while they are connected, so a
// console that served only the current save would watch every sheet
// vanish at logout. The remembered sheet stands in, stamped with when it
// was true, while the position stays the save's current one.
func TestSheetSurvivesLogout(t *testing.T) {
	h := testHandlers()
	srv := &store.Server{ID: 1}
	online := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	// While connected: the save's own sheet, unstamped — nothing is fresher.
	got := h.withKnownRecords(context.Background(), srv, onlineWorld(online, 100))
	p := got.Players[0]
	if len(p.Skills) != 1 || p.SeenAt != nil || p.SharedAt != nil {
		t.Fatalf("online player = %+v, want the live sheet unstamped", p)
	}

	// After logout: the sheet is remembered, stamped with the save it came
	// from, and the position comes from the newer save.
	got = h.withKnownRecords(context.Background(), srv, offlineWorld(online.Add(30*time.Minute), 250))
	p = got.Players[0]
	if len(p.Skills) != 1 || p.Skills[0].XP != 12107 {
		t.Errorf("Skills = %+v, want the remembered sheet", p.Skills)
	}
	if p.CharName != "safwyl" || p.SaveCount != 309 || p.Health != 140 {
		t.Errorf("identity/vitals lost: %+v", p)
	}
	if len(p.Inventory) != 1 {
		t.Errorf("Inventory = %+v, want the remembered one", p.Inventory)
	}
	if p.SeenAt == nil || !p.SeenAt.Equal(online) {
		t.Errorf("SeenAt = %v, want the save the sheet came from (%v)", p.SeenAt, online)
	}
	if p.SharedAt != nil {
		t.Error("SharedAt set on a save-derived sheet")
	}
	// Host-fresh facts stay the current save's.
	if p.Position == nil || p.Position.X != 250 || p.LastUpdated != 2000 {
		t.Errorf("position/freshness = %+v/%v, want the current save's", p.Position, p.LastUpdated)
	}
}

// TestLiveSheetOutranksMemory: once the player is back on, the save's own
// sheet takes over again and the stamp disappears.
func TestLiveSheetOutranksMemory(t *testing.T) {
	h := testHandlers()
	srv := &store.Server{ID: 1}
	first := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	h.withKnownRecords(context.Background(), srv, onlineWorld(first, 100))
	h.withKnownRecords(context.Background(), srv, offlineWorld(first.Add(time.Hour), 250))

	back := onlineWorld(first.Add(2*time.Hour), 300)
	back.Players[0].Skills[0].XP = 20000 // they levelled while away
	got := h.withKnownRecords(context.Background(), srv, back)
	p := got.Players[0]
	if p.SeenAt != nil {
		t.Errorf("SeenAt = %v on a live sheet", p.SeenAt)
	}
	if p.Skills[0].XP != 20000 {
		t.Errorf("XP = %d, want the live sheet's", p.Skills[0].XP)
	}
}

// TestSharedSheetFillsAnUnseenCharacter: a player whose session this
// console never observed still gets a sheet from their companion push,
// stamped as shared rather than seen.
func TestSharedSheetFillsAnUnseenCharacter(t *testing.T) {
	h := testHandlers()
	srv := &store.Server{ID: 1}

	rec := map[string]any{
		"meta_data": map[string]any{"char_guid": testGuid, "char_name": "safwyl"},
		"SaveCount": 12,
		"Skills": map[string]any{
			"Skills": []any{map[string]any{"Id": "4zYUGF5u_0KbMLkWJmmBbQ", "Xp": 999}},
		},
	}
	raw, _ := json.Marshal(rec)
	if !h.companion.put(srv.ID, dwsave.CanonicalGuid(testGuid), raw) {
		t.Fatal("push refused")
	}

	got := h.withKnownRecords(context.Background(), srv, offlineWorld(time.Now(), 42))
	p := got.Players[0]
	if len(p.Skills) != 1 || p.Skills[0].XP != 999 {
		t.Fatalf("Skills = %+v, want the shared sheet", p.Skills)
	}
	if p.SharedAt == nil {
		t.Error("SharedAt not stamped on a companion-sourced sheet")
	}
	if p.SeenAt != nil {
		t.Error("SeenAt stamped on a companion-sourced sheet")
	}
	if p.Position == nil || p.Position.X != 42 {
		t.Errorf("Position = %+v, want the save's transform", p.Position)
	}
}

// TestRevokeForgetsOnlySharedSheets: disabling sharing must forget what
// players shared — including copies folded into memory — while the
// sheets the console read from the save itself are its own and stay.
func TestRevokeForgetsOnlySharedSheets(t *testing.T) {
	h := testHandlers()
	srv := &store.Server{ID: 1}
	seen := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	// One character the console saw in the save.
	h.withKnownRecords(context.Background(), srv, onlineWorld(seen, 100))
	// Another that only ever arrived by companion push.
	otherGuid := "AAAA0000AAAA0000AAAA0000AAAA0000"
	raw, _ := json.Marshal(map[string]any{
		"meta_data": map[string]any{"char_guid": otherGuid, "char_name": "Aldra"},
		"Skills":    map[string]any{"Skills": []any{map[string]any{"Id": "x", "Xp": 5}}},
	})
	h.companion.put(srv.ID, dwsave.CanonicalGuid(otherGuid), raw)
	h.withKnownRecords(context.Background(), srv, offlineWorld(seen.Add(time.Minute), 100))

	h.companion.drop(srv.ID)
	h.records.forgetCompanionSourced(srv.ID)

	if _, ok := h.records.lookup(srv.ID, testGuid); !ok {
		t.Error("a sheet read from the save was forgotten by a sharing revoke")
	}
	if _, ok := h.records.lookup(srv.ID, otherGuid); ok {
		t.Error("a companion-shared sheet survived the revoke")
	}
}

// TestMemoryIsBounded: a leaked token or a long-lived console cannot grow
// the memory without limit.
func TestMemoryIsBounded(t *testing.T) {
	m := newRecordMemory(nil, nil)
	at := time.Now()
	for i := 0; i < maxRememberedRecords+8; i++ {
		guid := string(rune('A'+i%26)) + "0000000000000000000000000000000"
		m.remember(context.Background(), 1, guid, dwsave.PlayerCharacter{CharGuid: guid}, at.Add(time.Duration(i)*time.Second), false)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if n := len(m.byServer[1]); n > maxRememberedRecords {
		t.Errorf("remembered %d sheets, cap is %d", n, maxRememberedRecords)
	}
}

// newTestStore builds a real sqlite-backed store, as a console main has.
func newTestStore(t *testing.T) (*store.Store, int64) {
	t.Helper()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	box, err := crypto.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	st := store.New(sqlDB, box)
	id, err := st.CreateServer(context.Background(), &store.Server{Name: "wilds", Host: "10.0.0.5", Enabled: true})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	return st, id
}

// TestSheetSurvivesConsoleRestart is why the memory is backed by the
// store at all: a sheet is an observation of a moment that will not come
// back until that player logs in again, so a redeploy must not empty the
// view for a group that plays weekly.
func TestSheetSurvivesConsoleRestart(t *testing.T) {
	st, serverID := newTestStore(t)
	srv := &store.Server{ID: serverID}
	online := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

	// One console reads a save while the player is connected.
	h := storeHandlers(t, st)
	h.withKnownRecords(context.Background(), srv, onlineWorld(online, 100))

	// The console restarts: fresh memory, same database. The player is
	// long gone, so the save carries only their transform.
	restarted := storeHandlers(t, st)
	got := restarted.withKnownRecords(context.Background(), srv, offlineWorld(online.Add(2*time.Hour), 250))
	p := got.Players[0]
	if len(p.Skills) != 1 || p.Skills[0].XP != 12107 {
		t.Fatalf("Skills = %+v, want the sheet reloaded from the store", p.Skills)
	}
	if p.CharName != "safwyl" || p.Health != 140 {
		t.Errorf("reloaded sheet lost detail: %+v", p)
	}
	if p.SeenAt == nil || !p.SeenAt.Equal(online) {
		t.Errorf("SeenAt = %v, want the save the sheet came from (%v)", p.SeenAt, online)
	}
	if p.Position == nil || p.Position.X != 250 {
		t.Errorf("Position = %+v, want the current save's", p.Position)
	}
}

// TestSharedSheetsAreNotPersisted: a player's shared sheet must be
// forgettable, so it never reaches disk — a revoke cannot un-write a row
// a later restart would reload.
func TestSharedSheetsAreNotPersisted(t *testing.T) {
	st, serverID := newTestStore(t)
	srv := &store.Server{ID: serverID}

	h := storeHandlers(t, st)
	raw, _ := json.Marshal(map[string]any{
		"meta_data": map[string]any{"char_guid": testGuid, "char_name": "safwyl"},
		"Skills":    map[string]any{"Skills": []any{map[string]any{"Id": "x", "Xp": 7}}},
	})
	h.companion.put(srv.ID, dwsave.CanonicalGuid(testGuid), raw)
	h.withKnownRecords(context.Background(), srv, offlineWorld(time.Now(), 42))

	entries, err := st.ListGameState(context.Background(), serverID, sheetScope)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("companion-shared sheet reached the store: %+v", entries)
	}
}
