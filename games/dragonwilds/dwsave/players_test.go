package dwsave

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

// charRecord builds a character record shaped like the game's own JSON —
// the client character save schema, whose identity keys are the ones the
// recon observed inside a real played world save. Values are synthetic.
func charRecord(name, guid string, saveCount int) map[string]any {
	return map[string]any{
		"Version": 17,
		"meta_data": map[string]any{
			"char_guid": guid,
			"char_name": name,
			"char_type": 0,
			// Keyed by world guid — hyphenated lowercase here, to prove
			// the match normalizes; the fixture world's guid is
			// CA220B254BB44040A0666FB7646ED7FA.
			"worlds_playtime": map[string]any{
				"ca220b25-4bb4-4040-a066-6fb7646ed7fa": 7200.0,
				"00000000-0000-0000-0000-000000000001": 60.0,
			},
		},
		"SaveCount": saveCount,
		"Character": map[string]any{
			"Health":  map[string]any{"CurrentValue": 87.5},
			"Stamina": map[string]any{"CurrentValue": 100.0},
			"LastAccessibleLocation": map[string]any{
				"Position": "X=-96222.633 Y=-3299.294 Z=8697.630",
			},
			"Playtime_sim":  7100.0,
			"Playtime_wall": 7300.0,
		},
		"Inventory": map[string]any{
			"MaxSlotIndex": 30,
			"0":            map[string]any{"GUID": "aaaaaaaaaaaaaaaaaaaaaa", "ItemData": "P3_Aq0nAXu5dlFuBNGgyaw", "Durability": 1211.0},
			"7":            map[string]any{"GUID": "bbbbbbbbbbbbbbbbbbbbbb", "ItemData": "Mw42KUu2HuXm3baIbHa_8g", "Count": 42},
		},
		"PersonalInventory": map[string]any{
			"Loadout": map[string]any{
				"1": map[string]any{"GUID": "cccccccccccccccccccccc", "ItemData": "ewbJ37oeTkypaVfRgI_GPg", "Durability": 88.0},
			},
		},
		"Skills": map[string]any{
			"Skills": []any{
				map[string]any{"Id": "4zYUGF5u_0KbMLkWJmmBbQ", "Xp": 13363},
				map[string]any{"Id": "jqX0Gh6QI0GFFPCDFK_CJQ", "Xp": 388},
			},
		},
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// mustPrettyJSON matches how the game actually embeds its JSON: newlines
// and tab indentation between every token (seen verbatim in the committed
// capture's WorldEventManager blob). The first shipped scan looked for
// `{"` adjacent and walked straight past exactly this — a real played
// save showed "nobody has joined" with multiple players in the world —
// so the pretty form is the load-bearing case, not the compact one.
func mustPrettyJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "\t\t\t", "\t")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// utf16le encodes text the way UE serializes an FString that contains any
// non-ASCII character: the whole string as UTF-16LE, NUL-terminated.
func utf16le(s string) []byte {
	var out []byte
	for _, r := range s {
		if r > 0xFFFF {
			r -= 0x10000
			hi := 0xD800 + (r >> 10)
			lo := 0xDC00 + (r & 0x3FF)
			out = append(out, byte(hi), byte(hi>>8), byte(lo), byte(lo>>8))
			continue
		}
		out = append(out, byte(r), byte(r>>8))
	}
	return append(out, 0, 0)
}

// charRecordV2 is the current-build layout, mirrored from a real client
// character file (2026-08-19): the body nests under GameProgress, the
// guid is plain hex, worlds_playtime holds last-played unix timestamps
// rather than durations, and positions wear the compact V() spelling.
func charRecordV2(name, guidHex string, saveCount int) map[string]any {
	return map[string]any{
		"Version": 8,
		"meta_data": map[string]any{
			"char_guid": guidHex,
			"char_name": name,
			"char_type": 0,
			"worlds_playtime": map[string]any{
				// Timestamps, not durations — must NOT be read as playtime.
				"CA220B254BB44040A0666FB7646ED7FA": 1786311999.0,
				"BD93B2B8B4C1407D8F4D52EE93788EF4": 1787031905.0,
			},
		},
		"SaveCount": 309,
		"GameProgress": map[string]any{
			"Version": 80,
			"Character": map[string]any{
				"Health":        map[string]any{"CurrentValue": 140.0},
				"Stamina":       map[string]any{"CurrentValue": 80.0},
				"Playtime_sim":  85557.8,
				"Playtime_wall": 85605.2,
				"LastAccessibleLocation": map[string]any{
					"Position": "V(X=67189.34, Y=113003.27, Z=2277.47)",
				},
			},
			"Inventory": map[string]any{
				"0":            map[string]any{"GUID": "vXAHS0ME6KqZ9zWpJXOP6Q", "ItemData": "NmzyVLMIY0SYZfOAPICeKg", "Durability": 312.0},
				"32":           map[string]any{"GUID": "K2opGT-OR4ipbJQvzSGGZw", "ItemData": "bbLdJRhwPEWt1ScENYRUCg", "Count": 1012},
				"MaxSlotIndex": 81,
			},
			"PersonalInventory": map[string]any{"MaxSlotIndex": -1},
			"Loadout": map[string]any{
				"0": map[string]any{"GUID": "fVX0xkCdrxNJ9jmeBHGCUQ", "ItemData": "wLzThnOQEUaw90mBnn8QTw", "Durability": 792.0},
				// A hotbar reference, not an item — must be skipped.
				"5":            map[string]any{"PlayerInventoryItemIndex": 58},
				"MaxSlotIndex": 9,
			},
			"Skills": map[string]any{
				"Skills": []any{
					map[string]any{"Id": "4zYUGF5u_0KbMLkWJmmBbQ", "Xp": 12107},
					map[string]any{"Id": "pJggvotwOkuoc98igUn7xA", "Xp": 10280},
				},
			},
		},
	}
}

// TestParseCharacterRecordV2 pins the current-build layout against the
// shapes seen in a real file: SaveCharacters records are now JSON with
// the body under GameProgress.
func TestParseCharacterRecordV2(t *testing.T) {
	raw := mustPrettyJSON(t, charRecordV2("safwyl", "384D68C0479A97B5E99446BAB5A9405D", 309))
	p, err := ParseCharacterRecord(raw, "CA220B254BB44040A0666FB7646ED7FA")
	if err != nil {
		t.Fatalf("ParseCharacterRecord: %v", err)
	}
	if p.CharName != "safwyl" || p.SaveCount != 309 {
		t.Errorf("identity = %q save#%d", p.CharName, p.SaveCount)
	}
	if CanonicalGuid(p.CharGuid) != "384D68C0479A97B5E99446BAB5A9405D" {
		t.Errorf("guid = %q", p.CharGuid)
	}
	if p.Health != 140 || p.Stamina != 80 {
		t.Errorf("vitals = %v/%v", p.Health, p.Stamina)
	}
	// worlds_playtime holds timestamps now; the wall clock is the truth.
	if want := 85605.2 / 3600; math.Abs(p.PlaytimeHours-want) > 1e-6 {
		t.Errorf("PlaytimeHours = %v, want wall-clock %v (a timestamp must never read as a duration)", p.PlaytimeHours, want)
	}
	if p.Position == nil || p.Position.X != 67189.34 || p.Position.Y != 113003.27 || p.Position.Z != 2277.47 {
		t.Errorf("Position = %+v, want the V() form parsed", p.Position)
	}
	if len(p.Skills) != 2 || p.Skills[0].XP != 12107 {
		t.Errorf("Skills = %+v", p.Skills)
	}
	if len(p.Inventory) != 2 || p.Inventory[1].Slot != 32 || p.Inventory[1].Count != 1012 {
		t.Errorf("Inventory = %+v", p.Inventory)
	}
	// The loadout keeps real items and skips hotbar references.
	if len(p.Equipment) != 1 || p.Equipment[0].ID != "wLzThnOQEUaw90mBnn8QTw" {
		t.Errorf("Equipment = %+v", p.Equipment)
	}
}

// TestScanPlayers exercises the harvest over a byte soup shaped like real
// object state: binary noise, a bare record, the same record older (must
// dedupe to the newer), a wrapper document carrying a second record as an
// escaped string, and a JSON document that is not a character at all.
func TestScanPlayers(t *testing.T) {
	worldGuid := "CA220B254BB44040A0666FB7646ED7FA"

	// Pretty-printed like the game writes it — the load-bearing shape.
	rec := mustPrettyJSON(t, charRecord("Aldra", "1D77A8A24F4A9E5B36D5CB921AC1F2E3", 12))
	older := mustJSON(t, charRecord("Aldra", "1D77A8A24F4A9E5B36D5CB921AC1F2E3", 3))
	second := mustPrettyJSON(t, charRecord("Brantag", "9E5B36D5CB921AC1F2E31D77A8A24F4A", 5))
	wrapper := mustJSON(t, map[string]any{
		"CachedStates": map[string]any{"9E5B...": string(second)},
	})
	notAChar := mustPrettyJSON(t, map[string]any{"TriggerData": map[string]any{"CurrentPhase": 2}})

	var soup []byte
	add := func(parts ...[]byte) {
		for _, p := range parts {
			soup = append(soup, 0x00, 0xff, 0x07, '{', 0xc0)
			soup = append(soup, p...)
		}
	}
	add([]byte("LogSpudData noise {\" not json"), rec, notAChar, older, wrapper)
	soup = append(soup, 0xde, 0xad)

	players := scanPlayers(soup, worldGuid)
	if len(players) != 2 {
		t.Fatalf("players = %d, want 2 (%+v)", len(players), players)
	}

	// Sorted by name: Aldra then Brantag.
	a := players[0]
	if a.CharName != "Aldra" || a.CharGuid != "1D77A8A24F4A9E5B36D5CB921AC1F2E3" {
		t.Errorf("player[0] identity = %q/%q", a.CharName, a.CharGuid)
	}
	if a.SaveCount != 12 {
		t.Errorf("SaveCount = %d, want the newer record's 12", a.SaveCount)
	}
	if a.Health != 87.5 || a.Stamina != 100 {
		t.Errorf("vitals = %v/%v", a.Health, a.Stamina)
	}
	// 7200 s against this world's guid, matched across hyphens and case —
	// not the 7300 s wall-clock fallback.
	if a.PlaytimeHours != 2 {
		t.Errorf("PlaytimeHours = %v, want 2", a.PlaytimeHours)
	}
	if a.Position == nil || a.Position.X != -96222.633 || a.Position.Y != -3299.294 || a.Position.Z != 8697.630 {
		t.Errorf("Position = %+v", a.Position)
	}
	if len(a.Skills) != 2 || a.Skills[0].ID != "4zYUGF5u_0KbMLkWJmmBbQ" || a.Skills[0].XP != 13363 {
		t.Errorf("Skills = %+v", a.Skills)
	}
	if len(a.Inventory) != 2 {
		t.Fatalf("Inventory = %+v", a.Inventory)
	}
	if a.Inventory[0].Slot != 0 || a.Inventory[0].ID != "P3_Aq0nAXu5dlFuBNGgyaw" || a.Inventory[0].Count != 1 ||
		a.Inventory[0].Durability == nil || *a.Inventory[0].Durability != 1211 {
		t.Errorf("Inventory[0] = %+v", a.Inventory[0])
	}
	if a.Inventory[1].Slot != 7 || a.Inventory[1].Count != 42 || a.Inventory[1].Durability != nil {
		t.Errorf("Inventory[1] = %+v", a.Inventory[1])
	}
	// The loadout was nested under PersonalInventory in this record.
	if len(a.Equipment) != 1 || a.Equipment[0].ID != "ewbJ37oeTkypaVfRgI_GPg" || a.Equipment[0].Slot != 1 {
		t.Errorf("Equipment = %+v", a.Equipment)
	}

	if players[1].CharName != "Brantag" {
		t.Errorf("player[1] = %q, want the record carried inside the wrapper document", players[1].CharName)
	}
}

// TestScanPlayersFallbacks covers the degrade paths: no per-world playtime
// match falls back to wall clock, a position in an unexpected format stays
// nil, and a top-level loadout wins when present.
func TestScanPlayersFallbacks(t *testing.T) {
	rec := charRecord("Ceridd", "AAAA0000AAAA0000AAAA0000AAAA0000", 1)
	rec["meta_data"].(map[string]any)["worlds_playtime"] = map[string]any{"deadbeef00000000000000000000dead": 50.0}
	rec["Character"].(map[string]any)["LastAccessibleLocation"] = map[string]any{"Position": "somewhere else"}
	rec["Loadout"] = map[string]any{
		"0": map[string]any{"ItemData": "TuG7zUS90qgd7BOHhOxd8Q"},
	}

	players := scanPlayers(mustJSON(t, rec), "CA220B254BB44040A0666FB7646ED7FA")
	if len(players) != 1 {
		t.Fatalf("players = %+v", players)
	}
	p := players[0]
	if want := 7300.0 / 3600; p.PlaytimeHours != want {
		t.Errorf("PlaytimeHours = %v, want wall-clock fallback %v", p.PlaytimeHours, want)
	}
	if p.Position != nil {
		t.Errorf("Position = %+v, want nil for an unparseable string", p.Position)
	}
	if len(p.Equipment) != 1 || p.Equipment[0].ID != "TuG7zUS90qgd7BOHhOxd8Q" {
		t.Errorf("Equipment = %+v, want the top-level loadout", p.Equipment)
	}
}

// TestScanPlayersNestedObject covers a record travelling as a raw nested
// object inside a wrapper document, rather than as an escaped string.
func TestScanPlayersNestedObject(t *testing.T) {
	wrapper := mustPrettyJSON(t, map[string]any{
		"Characters": []any{charRecord("Dagny", "BBBB0000BBBB0000BBBB0000BBBB0000", 2)},
	})
	players := scanPlayers(wrapper, "CA220B254BB44040A0666FB7646ED7FA")
	if len(players) != 1 || players[0].CharName != "Dagny" {
		t.Fatalf("players = %+v, want the nested record", players)
	}
	if players[0].PlaytimeHours != 2 {
		t.Errorf("PlaytimeHours = %v — the nested path must resolve playtime like the direct one", players[0].PlaytimeHours)
	}
}

// TestScanPlayersUTF16 covers UE's wide-string serialization: an FString
// containing any non-ASCII character is written whole as UTF-16LE, so a
// player with an accented name must still be found.
func TestScanPlayersUTF16(t *testing.T) {
	rec := mustPrettyJSON(t, charRecord("Åslaug", "CCCC0000CCCC0000CCCC0000CCCC0000", 7))
	var soup []byte
	soup = append(soup, 0x01, 0x7b, 0x02) // a bare "{" byte that is not JSON
	soup = append(soup, utf16le(string(rec))...)
	soup = append(soup, 0xfe, 0xff)

	players := scanPlayers(soup, "CA220B254BB44040A0666FB7646ED7FA")
	if len(players) != 1 {
		t.Fatalf("players = %+v, want the UTF-16 record", players)
	}
	if players[0].CharName != "Åslaug" || players[0].SaveCount != 7 {
		t.Errorf("player = %q save#%d", players[0].CharName, players[0].SaveCount)
	}
}

// TestParseWithPlayersChunk splices a "Play"-style chunk holding a
// character record into the real capture, the shape the game's own
// truncated "Players" chunk would take, and runs the full Parse: world
// metadata must be untouched and the character found.
func TestParseWithPlayersChunk(t *testing.T) {
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}

	rec := mustPrettyJSON(t, charRecord("Aldra", "1D77A8A24F4A9E5B36D5CB921AC1F2E3", 4))
	// A UE-string-wrapped record inside the chunk, as SPUD stores strings.
	payload := binary.LittleEndian.AppendUint32(nil, uint32(len(rec)+1))
	payload = append(payload, rec...)
	payload = append(payload, 0)
	chunk := append([]byte("Play"), binary.LittleEndian.AppendUint32(nil, uint32(len(payload)))...)
	chunk = append(chunk, payload...)

	// Splice inside SAVE: grow the outer length, append the new chunk.
	save := append([]byte{}, data...)
	binary.LittleEndian.PutUint32(save[4:], binary.LittleEndian.Uint32(save[4:])+uint32(len(chunk)))
	save = append(save, chunk...)

	w, err := Parse(save)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if w.WorldName != "World-75058" || w.SaveGuid != "CA220B254BB44040A0666FB7646ED7FA" {
		t.Errorf("world metadata disturbed: %q %q", w.WorldName, w.SaveGuid)
	}
	var ids []string
	for _, c := range w.Chunks {
		ids = append(ids, c.ID)
	}
	if got := strings.Join(ids, " "); got != "INFO GLOB LVLS Play" {
		t.Errorf("chunks = %q", got)
	}
	if len(w.Players) != 1 {
		t.Fatalf("Players = %+v, want the spliced record", w.Players)
	}
	p := w.Players[0]
	if p.CharName != "Aldra" || p.SaveCount != 4 || p.PlaytimeHours != 2 {
		t.Errorf("player = %+v", p)
	}
}

// TestPlayersJSONShape pins the wire shape the console serves: players is
// always a list (never null), and empty optional fields stay out of the way.
func TestPlayersJSONShape(t *testing.T) {
	w, err := Parse(readFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"players":[]`) {
		t.Errorf("players should serialize as an empty list on an unplayed world")
	}
}
