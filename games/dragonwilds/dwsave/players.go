// Player-character extraction from a Dragonwilds world save.
//
// A dedicated server persists each character who has played the world as a
// JSON document embedded in the save (recon: "Player state in the world
// save: located", observed on a real played save 2026-08-09 — char_guid,
// char_name, worlds_playtime, SaveCount, Customization all verified
// present). The document's full schema is known from the game's own
// client-side character saves, which are the same record written as a
// standalone JSON file.
//
// The JSON sits inside UE strings within SPUD object state (the game logs
// a truncated "Players" chunk when writing it), and that wrapper is the
// one part of the layout no committed capture pins down — the only played
// save seen was a live server's, which held personal data and was not
// committed. So this file harvests the records the robust way: scan for
// embedded JSON documents, which are self-delimiting, and accept only
// those carrying the character-record identity keys. A future game build
// moving the records to a different chunk changes nothing here; a build
// abandoning JSON degrades to an empty player list, never a misread.
package dwsave

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// PlayerCharacter is one character the world save knows. Identity is the
// game's own: the character guid (the DCG guid the server logs on
// disconnect) and display name. The EOS player id is deliberately absent —
// it appears nowhere in the save (recon-verified); mapping character to
// account goes through the ini KnownPlayerList and the join log.
type PlayerCharacter struct {
	CharGuid string `json:"charGuid"`
	CharName string `json:"charName"`
	// SaveCount counts the character's own saves — an activity odometer.
	SaveCount int `json:"saveCount"`
	// PlaytimeHours is this world's entry in the record's per-world
	// playtime map, falling back to the character's wall-clock total when
	// the map has no entry for this world. Zero means not recorded.
	PlaytimeHours float64 `json:"playtimeHours"`
	Health        float64 `json:"health"`
	Stamina       float64 `json:"stamina"`
	// Position is where the character last stood when their state was
	// saved, in UE units (centimetres). Nil when the record carries none.
	Position *Position `json:"position,omitempty"`
	// Skills carry raw XP per skill id; the id → display-name map is
	// vendored frontend data. No level is derived here: the game's XP
	// curve is its own (piecewise, and changed across game versions), so
	// showing the XP honestly beats computing a level from the wrong table.
	Skills []Skill `json:"skills"`
	// Inventory is the backpack; Equipment the worn loadout. Item ids are
	// the game's persistence ids, resolved to names frontend-side.
	Inventory []Item `json:"inventory"`
	Equipment []Item `json:"equipment"`
}

// Position is a UE world position, in centimetres.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// Skill is one skill's raw XP, keyed by the game's skill persistence id.
type Skill struct {
	ID string `json:"id"`
	XP int    `json:"xp"`
}

// Item is one occupied inventory or loadout slot.
type Item struct {
	Slot int    `json:"slot"`
	ID   string `json:"id"`
	// Count is 1 when the save omits it (non-stackable items).
	Count int `json:"count"`
	// Durability is absent for items that have none.
	Durability *float64 `json:"durability,omitempty"`
}

// charJSON is the character record as the game writes it. The schema is
// the client character save's, which community editors read and write and
// whose identity keys match what the recon observed inside a played world
// save byte-for-byte. Unknown fields are ignored by encoding/json, so a
// game build adding fields costs nothing.
type charJSON struct {
	MetaData struct {
		CharGuid string `json:"char_guid"`
		CharName string `json:"char_name"`
		// WorldsPlaytime maps world save guid → seconds played there.
		WorldsPlaytime map[string]float64 `json:"worlds_playtime"`
	} `json:"meta_data"`
	SaveCount int `json:"SaveCount"`
	Character struct {
		Health  struct {
			CurrentValue float64 `json:"CurrentValue"`
		} `json:"Health"`
		Stamina struct {
			CurrentValue float64 `json:"CurrentValue"`
		} `json:"Stamina"`
		LastAccessibleLocation struct {
			// Position is UE's FVector::ToString: "X=1.000 Y=2.000 Z=3.000".
			Position string `json:"Position"`
		} `json:"LastAccessibleLocation"`
		PlaytimeWall float64 `json:"Playtime_wall"`
	} `json:"Character"`
	Inventory         invJSON `json:"Inventory"`
	PersonalInventory invJSON `json:"PersonalInventory"`
	Loadout           invJSON `json:"Loadout"`
	Skills            struct {
		Skills []struct {
			ID string `json:"Id"`
			XP int    `json:"Xp"`
		} `json:"Skills"`
	} `json:"Skills"`
}

// isCharacter reports whether a decoded document is a character record at
// all — the identity keys under meta_data are the discriminator, and no
// other document in the save carries them.
func (c *charJSON) isCharacter() bool {
	return c.MetaData.CharGuid != "" || c.MetaData.CharName != ""
}

// invJSON is an inventory object: numeric slot keys → items, with
// "MaxSlotIndex" and friends alongside. Two container shapes exist in the
// wild — items directly under numeric keys, or nested one level down under
// an "Inventory"/"Loadout" key — so both are kept. Unmarshal never fails:
// a shape this parser doesn't recognize is an empty inventory, not a
// broken player list.
type invJSON struct {
	slots  map[int]itemJSON
	nested map[string]invJSON
}

type itemJSON struct {
	ItemData   string   `json:"ItemData"`
	Count      *int     `json:"Count"`
	Durability *float64 `json:"Durability"`
}

func (v *invJSON) UnmarshalJSON(b []byte) error {
	v.slots = map[int]itemJSON{}
	v.nested = map[string]invJSON{}
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	for k, raw := range m {
		if idx, err := strconv.Atoi(k); err == nil {
			var it itemJSON
			if json.Unmarshal(raw, &it) == nil && it.ItemData != "" {
				v.slots[idx] = it
			}
			continue
		}
		if k == "Inventory" || k == "Loadout" {
			var sub invJSON
			if json.Unmarshal(raw, &sub) == nil {
				v.nested[k] = sub
			}
		}
	}
	return nil
}

// items flattens to sorted slots, looking one level into a nested
// container of the same name when the direct level holds none.
func (v invJSON) items(nestedKey string) []Item {
	slots := v.slots
	if len(slots) == 0 {
		if sub, ok := v.nested[nestedKey]; ok {
			slots = sub.slots
		}
	}
	out := make([]Item, 0, len(slots))
	for idx, it := range slots {
		item := Item{Slot: idx, ID: it.ItemData, Count: 1, Durability: it.Durability}
		if it.Count != nil {
			item.Count = *it.Count
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

// scanPlayers walks the whole save image for embedded character records.
// JSON documents are self-delimiting, so each candidate "{"-run either
// decodes (and is skipped whole if it isn't a character) or fails fast.
// Records reachable only as escaped strings inside a wrapper document are
// found by walking the wrapper's string values.
func scanPlayers(data []byte, worldGuid string) []PlayerCharacter {
	found := map[string]PlayerCharacter{}
	marker := []byte(`{"`)
	pos := 0
	for {
		i := bytes.Index(data[pos:], marker)
		if i < 0 {
			break
		}
		abs := pos + i
		dec := json.NewDecoder(bytes.NewReader(data[abs:]))
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			pos = abs + len(marker)
			continue
		}
		harvestDoc(raw, worldGuid, found)
		pos = abs + int(dec.InputOffset())
	}

	players := make([]PlayerCharacter, 0, len(found))
	for _, p := range found {
		players = append(players, p)
	}
	sort.Slice(players, func(i, j int) bool {
		a, b := strings.ToLower(players[i].CharName), strings.ToLower(players[j].CharName)
		if a != b {
			return a < b
		}
		return players[i].CharGuid < players[j].CharGuid
	})
	return players
}

// harvestDoc takes one decoded JSON document: a character record is
// converted and merged; anything else has its string values walked in
// case the record travels escaped inside a wrapper document.
func harvestDoc(raw []byte, worldGuid string, found map[string]PlayerCharacter) {
	var c charJSON
	if json.Unmarshal(raw, &c) == nil && c.isCharacter() {
		merge(found, convertChar(&c, worldGuid))
		return
	}
	var generic any
	if json.Unmarshal(raw, &generic) != nil {
		return
	}
	walkStrings(generic, func(s string) {
		t := strings.TrimSpace(s)
		if !strings.HasPrefix(t, "{") {
			return
		}
		harvestDoc([]byte(t), worldGuid, found)
	})
}

func walkStrings(v any, fn func(string)) {
	switch x := v.(type) {
	case string:
		fn(x)
	case map[string]any:
		for _, e := range x {
			walkStrings(e, fn)
		}
	case []any:
		for _, e := range x {
			walkStrings(e, fn)
		}
	}
}

// merge dedups by character identity; a save can hold more than one copy
// of a record, and the one that has been saved more times is the newer.
func merge(found map[string]PlayerCharacter, p PlayerCharacter) {
	key := p.CharGuid
	if key == "" {
		key = "name:" + p.CharName
	}
	if prev, ok := found[key]; ok && prev.SaveCount >= p.SaveCount {
		return
	}
	found[key] = p
}

func convertChar(c *charJSON, worldGuid string) PlayerCharacter {
	p := PlayerCharacter{
		CharGuid:  c.MetaData.CharGuid,
		CharName:  c.MetaData.CharName,
		SaveCount: c.SaveCount,
		Health:    c.Character.Health.CurrentValue,
		Stamina:   c.Character.Stamina.CurrentValue,
		Inventory: c.Inventory.items("Inventory"),
		Skills:    make([]Skill, 0, len(c.Skills.Skills)),
	}
	for _, s := range c.Skills.Skills {
		p.Skills = append(p.Skills, Skill{ID: s.ID, XP: s.XP})
	}
	// The loadout has been seen top-level and nested under
	// PersonalInventory; take whichever is populated.
	p.Equipment = c.Loadout.items("Loadout")
	if len(p.Equipment) == 0 {
		p.Equipment = c.PersonalInventory.items("Loadout")
	}
	if pos, ok := parsePosition(c.Character.LastAccessibleLocation.Position); ok {
		p.Position = &pos
	}
	p.PlaytimeHours = playtimeHours(c, worldGuid)
	return p
}

// parsePosition reads UE's FVector::ToString form. Anything else is "no
// position", never a guess.
func parsePosition(s string) (Position, bool) {
	var p Position
	n, err := fmt.Sscanf(s, "X=%f Y=%f Z=%f", &p.X, &p.Y, &p.Z)
	if err != nil || n != 3 {
		return Position{}, false
	}
	return p, true
}

// playtimeHours resolves this world's playtime. The per-world map is keyed
// by world save guid in a case/hyphenation the game chooses; normalizing
// both sides makes the match, and the character's wall-clock total is the
// fallback when this world has no entry.
func playtimeHours(c *charJSON, worldGuid string) float64 {
	want := normalizeGuid(worldGuid)
	if want != "" {
		for k, seconds := range c.MetaData.WorldsPlaytime {
			if normalizeGuid(k) == want {
				return seconds / 3600
			}
		}
	}
	return c.Character.PlaytimeWall / 3600
}

func normalizeGuid(s string) string {
	return strings.ToUpper(strings.ReplaceAll(s, "-", ""))
}
