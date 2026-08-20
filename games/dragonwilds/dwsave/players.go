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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// ParseCharacterRecord parses one character record as the game writes it —
// a client character save file (plain JSON) or the same record embedded in
// an old-build world save. worldGuid resolves the record's per-world
// playtime map to this world; empty falls back to the character's
// wall-clock total. It errors on anything that is not a character record,
// so callers can use it as validation.
func ParseCharacterRecord(data []byte, worldGuid string) (*PlayerCharacter, error) {
	var c charJSON
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("not valid JSON: %w", err)
	}
	if !c.isCharacter() {
		return nil, fmt.Errorf("not a character record: no char_guid or char_name under meta_data")
	}
	p := convertChar(&c, worldGuid)
	return &p, nil
}

// CanonicalGuid folds the two guid spellings the game uses into one
// comparison key: the JSON record's char_guid is the 16 guid bytes
// base64url-encoded (22 characters, no padding), while the binary
// transform record and the server log render them as 32 hex digits.
// Anything unrecognized keys as itself.
func CanonicalGuid(s string) string {
	return canonicalGuid(s)
}

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
	// The survival meters, 0–100 as the game keeps them. Zero when the
	// record carries none (transform-only entries).
	Sustenance float64 `json:"sustenance,omitempty"`
	Hydration  float64 `json:"hydration,omitempty"`
	Endurance  float64 `json:"endurance,omitempty"`
	// Progression summarizes the record's unlock lists as counts — the
	// full lists are thousands of opaque ids, and a character sheet wants
	// the shape of progress, not the ids. Nil on transform-only entries.
	Progression *Progression `json:"progression,omitempty"`
	// Position is where the character last stood when their state was
	// saved, in UE units (centimetres). Nil when the record carries none.
	// The world save's own transform record wins over the character
	// record's last-accessible location when both exist.
	Position *Position `json:"position,omitempty"`
	// LastUpdated is the transform record's own freshness stamp, in the
	// game's clock (seconds; the epoch is the game's, not wall time).
	// Zero when the save carries no transform for this character.
	LastUpdated float64 `json:"lastUpdated,omitempty"`
	// SharedAt is when a companion app last relayed this character's
	// record (see games/dragonwilds/docs/companion.md). Nil for
	// save-derived records; set by the console's merge, never by Parse —
	// character data lives on each player's machine, so a record richer
	// than guid+position can only arrive by being shared.
	SharedAt *time.Time `json:"sharedAt,omitempty"`
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

// Progression is the countable shape of a character's advancement. Quest
// states follow the game's own enum (0 not started, 1 in progress,
// 2 complete — community-verified against the quest editor tooling and a
// real record).
type Progression struct {
	QuestsCompleted  int `json:"questsCompleted"`
	QuestsInProgress int `json:"questsInProgress"`
	Recipes          int `json:"recipes"`
	Spells           int `json:"spells"`
	Buildings        int `json:"buildings"`
	Journal          int `json:"journal"`
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

// charJSON is the character record as the game writes it. Two layouts
// exist in the wild, both seen in real files: older builds carried the
// body (Character/Inventory/Loadout/Skills) at the top level, current
// builds nest the same body under "GameProgress" (verified against a
// real current-build record, 2026-08-19). Unknown fields are ignored by
// encoding/json, so a game build adding fields costs nothing.
type charJSON struct {
	MetaData struct {
		CharGuid string `json:"char_guid"`
		CharName string `json:"char_name"`
		// WorldsPlaytime is keyed by world save guid. Its values changed
		// meaning between builds: seconds played there originally,
		// last-played unix timestamps now — see playtimeHours.
		WorldsPlaytime map[string]float64 `json:"worlds_playtime"`
	} `json:"meta_data"`
	SaveCount int `json:"SaveCount"`

	recordBody              // old builds: the body at top level
	GameProgress *recordBody `json:"GameProgress"` // current builds
}

// body picks whichever layout this record uses.
func (c *charJSON) body() *recordBody {
	if c.GameProgress != nil {
		return c.GameProgress
	}
	return &c.recordBody
}

// recordBody is the character state itself, identical between layouts.
type recordBody struct {
	Character struct {
		Health struct {
			CurrentValue float64 `json:"CurrentValue"`
		} `json:"Health"`
		Stamina struct {
			CurrentValue float64 `json:"CurrentValue"`
		} `json:"Stamina"`
		Sustenance struct {
			SustenanceValue float64 `json:"SustenanceValue"`
		} `json:"Sustenance"`
		Hydration struct {
			HydrationValue float64 `json:"HydrationValue"`
		} `json:"Hydration"`
		Endurance struct {
			EnduranceValue float64 `json:"EnduranceValue"`
		} `json:"Endurance"`
		LastAccessibleLocation struct {
			// Position appears as UE's plain FVector::ToString
			// ("X=1.000 Y=2.000 Z=3.000") in old records and the compact
			// "V(X=1.00, Y=2.00, Z=3.00)" form in current ones.
			Position string `json:"Position"`
		} `json:"LastAccessibleLocation"`
		PlaytimeWall float64 `json:"Playtime_wall"`
	} `json:"Character"`
	QuestProgress struct {
		Quests []struct {
			QuestState int `json:"QuestState"`
		} `json:"Quests"`
	} `json:"QuestProgress"`
	Progress struct {
		RecipesUnlocked   []json.RawMessage `json:"RecipesUnlocked"`
		SpellsUnlocked    []json.RawMessage `json:"SpellsUnlocked"`
		BuildingsUnlocked []json.RawMessage `json:"BuildingsUnlocked"`
	} `json:"Progress"`
	Journal struct {
		UnlockedEntries []json.RawMessage `json:"UnlockedEntries"`
	} `json:"Journal"`
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
// The game pretty-prints the JSON it embeds (newlines and tabs between
// every token — visible in the committed capture's WorldEventManager
// blob), so the scan keys on a bare "{" with a whitespace-tolerant
// lookahead, never on "{" and a quote being adjacent. JSON documents are
// self-delimiting, so each candidate either decodes (and is skipped whole
// if it isn't a character) or fails fast. Records reachable only inside a
// wrapper document — as escaped strings or as nested objects — are found
// by walking the wrapper. A second pass catches documents UE serialized
// as UTF-16, which it does to a whole string the moment one character in
// it is non-ASCII — a player with an accented name must not vanish.
func scanPlayers(data []byte, worldGuid string) []PlayerCharacter {
	found := map[string]PlayerCharacter{}
	scanASCII(data, worldGuid, found)
	scanUTF16(data, worldGuid, found)

	// Newer game builds keep no JSON record in the world save at all —
	// the world carries each character's transform instead (see spud.go).
	// Transforms enrich a JSON-found record with the save's own position,
	// and stand alone as a name-less record when there is no JSON side.
	for _, ct := range collectTransforms(data) {
		key := canonicalGuid(ct.guid)
		if p, ok := found[key]; ok {
			pos := ct.pos
			p.Position = &pos
			p.LastUpdated = ct.lastUpdated
			found[key] = p
			continue
		}
		pos := ct.pos
		found[key] = PlayerCharacter{
			CharGuid:    ct.guid,
			Position:    &pos,
			LastUpdated: ct.lastUpdated,
			Skills:      []Skill{},
			Inventory:   []Item{},
			Equipment:   []Item{},
		}
	}

	players := make([]PlayerCharacter, 0, len(found))
	for _, p := range found {
		players = append(players, p)
	}
	sort.Slice(players, func(i, j int) bool {
		a, b := strings.ToLower(players[i].CharName), strings.ToLower(players[j].CharName)
		// Named characters lead; name-less transform records follow.
		if (a == "") != (b == "") {
			return a != ""
		}
		if a != b {
			return a < b
		}
		return players[i].CharGuid < players[j].CharGuid
	})
	return players
}

// canonicalGuid folds the two guid spellings the game uses into one merge
// key: the JSON record's char_guid is the 16 guid bytes base64url-encoded
// (22 characters, no padding), while the binary transform record renders
// them as 32 hex digits. Anything unrecognized keys as itself.
func canonicalGuid(s string) string {
	if len(s) == 22 {
		// Strict: a 22-char group has four trailing bits, and the lenient
		// decoder would silently drop nonzero ones — aliasing distinct
		// strings onto one guid. The game writes canonical encodings.
		if b, err := base64.RawURLEncoding.Strict().DecodeString(s); err == nil && len(b) == 16 {
			return renderGuid(b)
		}
	}
	if len(s) == 32 {
		return strings.ToUpper(s)
	}
	return s
}

func scanASCII(data []byte, worldGuid string, found map[string]PlayerCharacter) {
	pos := 0
	for pos < len(data) {
		i := bytes.IndexByte(data[pos:], '{')
		if i < 0 {
			break
		}
		abs := pos + i
		if !plausibleObject(data[abs:]) {
			pos = abs + 1
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(data[abs:]))
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			pos = abs + 1
			continue
		}
		harvestDoc(raw, worldGuid, found)
		pos = abs + int(dec.InputOffset())
	}
}

// scanUTF16 finds JSON objects stored as UTF-16LE text. It decodes each
// plausible wide run back to UTF-8 and harvests that; the run ends at the
// string's NUL terminator or the first unit that cannot be part of text.
func scanUTF16(data []byte, worldGuid string, found map[string]PlayerCharacter) {
	marker := []byte{'{', 0}
	pos := 0
	for pos+1 < len(data) {
		i := bytes.Index(data[pos:], marker)
		if i < 0 {
			break
		}
		abs := pos + i
		run := decodeWideRun(data[abs:])
		if !plausibleObject(run) {
			pos = abs + 2
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(run))
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			pos = abs + 2
			continue
		}
		harvestDoc(raw, worldGuid, found)
		// Skip exactly the wide bytes the document occupied: re-encode
		// the consumed UTF-8 to count its UTF-16 units, so multi-byte
		// runes neither overshoot into a neighbouring document nor
		// trigger a rescan of this one.
		consumed := utf16.Encode([]rune(string(run[:dec.InputOffset()])))
		pos = abs + 2*len(consumed)
	}
}

// decodeWideRun converts a UTF-16LE run to UTF-8, stopping at the NUL
// terminator UE writes, at an unpaired trailing byte, or at a generous
// size cap (a character record is tens of KB, not tens of MB).
func decodeWideRun(b []byte) []byte {
	const maxUnits = 4 << 20
	units := make([]uint16, 0, 1024)
	for i := 0; i+1 < len(b) && len(units) < maxUnits; i += 2 {
		u := uint16(b[i]) | uint16(b[i+1])<<8
		if u == 0 {
			break
		}
		units = append(units, u)
	}
	return []byte(string(utf16.Decode(units)))
}

// plausibleObject is the cheap pre-filter before spinning up a JSON
// decoder: an object literal opens with "{", then — whitespace aside —
// either a key or the closing brace.
func plausibleObject(b []byte) bool {
	if len(b) == 0 || b[0] != '{' {
		return false
	}
	for _, c := range b[1:] {
		switch c {
		case ' ', '\t', '\r', '\n':
			continue
		case '"', '}':
			return true
		default:
			return false
		}
	}
	return false
}

// harvestDoc takes one decoded JSON document: a character record is
// converted and merged; anything else is walked for records travelling
// inside it — as escaped strings or as nested objects.
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
	walkDoc(generic, worldGuid, found)
}

func walkDoc(v any, worldGuid string, found map[string]PlayerCharacter) {
	switch x := v.(type) {
	case string:
		t := strings.TrimSpace(x)
		if strings.HasPrefix(t, "{") {
			harvestDoc([]byte(t), worldGuid, found)
		}
	case map[string]any:
		// A nested object carrying the record's discriminator key is
		// re-examined as a record in its own right.
		if _, ok := x["meta_data"]; ok {
			if b, err := json.Marshal(x); err == nil {
				var c charJSON
				if json.Unmarshal(b, &c) == nil && c.isCharacter() {
					merge(found, convertChar(&c, worldGuid))
					return
				}
			}
		}
		for _, e := range x {
			walkDoc(e, worldGuid, found)
		}
	case []any:
		for _, e := range x {
			walkDoc(e, worldGuid, found)
		}
	}
}

// merge dedups by character identity; a save can hold more than one copy
// of a record, and the one that has been saved more times is the newer.
func merge(found map[string]PlayerCharacter, p PlayerCharacter) {
	key := canonicalGuid(p.CharGuid)
	if key == "" {
		key = "name:" + p.CharName
	}
	if prev, ok := found[key]; ok && prev.SaveCount >= p.SaveCount {
		return
	}
	found[key] = p
}

func convertChar(c *charJSON, worldGuid string) PlayerCharacter {
	b := c.body()
	prog := Progression{
		Recipes:   len(b.Progress.RecipesUnlocked),
		Spells:    len(b.Progress.SpellsUnlocked),
		Buildings: len(b.Progress.BuildingsUnlocked),
		Journal:   len(b.Journal.UnlockedEntries),
	}
	for _, q := range b.QuestProgress.Quests {
		switch q.QuestState {
		case 1:
			prog.QuestsInProgress++
		case 2:
			prog.QuestsCompleted++
		}
	}
	p := PlayerCharacter{
		CharGuid:    c.MetaData.CharGuid,
		CharName:    c.MetaData.CharName,
		SaveCount:   c.SaveCount,
		Health:      b.Character.Health.CurrentValue,
		Stamina:     b.Character.Stamina.CurrentValue,
		Sustenance:  b.Character.Sustenance.SustenanceValue,
		Hydration:   b.Character.Hydration.HydrationValue,
		Endurance:   b.Character.Endurance.EnduranceValue,
		Progression: &prog,
		Inventory:   b.Inventory.items("Inventory"),
		Skills:      make([]Skill, 0, len(b.Skills.Skills)),
	}
	for _, s := range b.Skills.Skills {
		p.Skills = append(p.Skills, Skill{ID: s.ID, XP: s.XP})
	}
	// The loadout has been seen top-level and nested under
	// PersonalInventory; take whichever is populated.
	p.Equipment = b.Loadout.items("Loadout")
	if len(p.Equipment) == 0 {
		p.Equipment = b.PersonalInventory.items("Loadout")
	}
	if pos, ok := parsePosition(b.Character.LastAccessibleLocation.Position); ok {
		p.Position = &pos
	}
	p.PlaytimeHours = playtimeHours(c, worldGuid)
	return p
}

// parsePosition reads both position spellings the game has used: plain
// FVector::ToString ("X=1.000 Y=2.000 Z=3.000") and the compact
// "V(X=1.00, Y=2.00, Z=3.00)" current records carry. Anything else is
// "no position", never a guess.
func parsePosition(s string) (Position, bool) {
	var p Position
	if n, err := fmt.Sscanf(s, "X=%f Y=%f Z=%f", &p.X, &p.Y, &p.Z); err == nil && n == 3 {
		return p, true
	}
	if n, err := fmt.Sscanf(s, "V(X=%f, Y=%f, Z=%f)", &p.X, &p.Y, &p.Z); err == nil && n == 3 {
		return p, true
	}
	return Position{}, false
}

// maxPlausibleSeconds separates the two meanings worlds_playtime has had:
// old builds stored seconds played per world, current builds store
// last-played unix timestamps. Three years of continuous play is beyond
// any real duration and far below any current timestamp.
const maxPlausibleSeconds = 100_000_000

// playtimeHours resolves this record's playtime. An old-build per-world
// entry (a duration, matched by normalized world guid) is the most
// precise answer; a current-build entry is a timestamp, not a duration,
// so the character's wall-clock total is the honest figure there.
func playtimeHours(c *charJSON, worldGuid string) float64 {
	want := normalizeGuid(worldGuid)
	if want != "" {
		for k, seconds := range c.MetaData.WorldsPlaytime {
			if normalizeGuid(k) == want && seconds < maxPlausibleSeconds {
				return seconds / 3600
			}
		}
	}
	return c.body().Character.PlaytimeWall / 3600
}

func normalizeGuid(s string) string {
	return strings.ToUpper(strings.ReplaceAll(s, "-", ""))
}
