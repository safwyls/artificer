// Package dwsave reads the world metadata out of a Dragonwilds save file.
//
// The save is the SPUD plugin's chunked container, not GVAS — see the
// "Empirical findings" section of games/dragonwilds/docs/recon.md. The layout this
// parser understands was mapped byte-by-byte from a real capture
// (testdata/world-empty.sav, one directory up) and cross-checked against the
// server's own output: the GUID this package renders from the header is the
// exact string the server logs as WorldSaveGuid.
//
// Container shape: the whole file is one RIFF-style chunk — a FourCC id,
// a uint32 little-endian payload length, then the payload — and chunks nest.
// The outer chunk is SAVE; inside it, INFO carries the world metadata this
// package is after, GLOB and LVLS carry the object state it deliberately
// does not walk (the layout is mapped — see the recon's "SPUD object layer
// mapped" section — but only the embedded character records are read, via
// the scan in players.go). Inside INFO, a CINF sub-chunk holds three
// tables: a list of field names, a fence-post offset table (one more
// offset than names), and a values blob the offsets index into. Fields are
// decoded by name, so a future game build adding or reordering fields
// degrades to missing values rather than misread ones.
package dwsave

import (
	"encoding/binary"
	"fmt"
	"time"
)

// Chunk is one top-level chunk of the container: its FourCC id and payload
// size. The inventory is kept on World as an honest record of how much of
// the save exists that this parser does not read.
type Chunk struct {
	ID    string `json:"id"`
	Bytes int    `json:"bytes"`
}

// World is the metadata a Dragonwilds save carries in its INFO chunk, plus
// the level names from LVLS. Field names follow the save's own (WorldOwnerId,
// WorldNameOwner), not what one might wish they were called.
type World struct {
	// WorldName is the world's display name and the save file's basename
	// ("World-75058").
	WorldName string `json:"worldName"`
	// MapName is the level the world runs on ("L_World").
	MapName string `json:"mapName"`
	// SaveGuid is GUID_A..D rendered the way the server itself renders it
	// (uppercase %08X quads — verified against the WorldSaveGuid it logs).
	SaveGuid string `json:"saveGuid"`
	// Version is the header's VERSION field; SaveFileRevision its
	// Meta_SaveFileRevision.
	Version          uint32 `json:"version"`
	SaveFileRevision uint32 `json:"saveFileRevision"`

	FriendlyFire       bool   `json:"friendlyFire"`
	SurvivalDifficulty uint32 `json:"survivalDifficulty"`
	HardcoreState      uint32 `json:"hardcoreState"`
	CrossplayEnabled   bool   `json:"crossplayEnabled"`
	SessionPrivacy     uint32 `json:"sessionPrivacy"`
	// HasSessionPassword reports whether SessionPasswd is non-empty. The
	// password itself is deliberately not surfaced.
	HasSessionPassword bool `json:"hasSessionPassword"`

	// OwnerID is WorldOwnerId — free text, since the server never validates
	// it (recon: boots on the literal "test123"). Empty on a world created
	// by a fresh dedicated server.
	OwnerID string `json:"ownerId"`
	// OwnerName is WorldNameOwner; empty on a server-created world.
	OwnerName string `json:"ownerName"`
	// LastSavedBy is a build tag ("++dominion+live:232224"), not a player.
	LastSavedBy string `json:"lastSavedBy"`

	// HeaderStamp is the timestamp string at the top of INFO, verbatim, and
	// TimeOfSave is the header's TimeOfSave FDateTime (100 ns ticks since
	// year 1), decoded as-is. The two agree with each other to the
	// millisecond — but on the capture host both recorded local wall time
	// despite the stamp's Z suffix (the server's UTC log stamps ran a
	// timezone ahead of them). Neither is rewritten here; a caller that
	// needs a trustworthy "last saved" instant should prefer ModTime.
	HeaderStamp string    `json:"headerStamp"`
	TimeOfSave  time.Time `json:"timeOfSave"`

	// Levels are the LEVL sub-chunk names inside LVLS.
	Levels []string `json:"levels"`
	// Chunks inventories the container's top-level chunks.
	Chunks []Chunk `json:"chunks"`

	// Players are the character records the save embeds — everyone who has
	// played this world, with their skills, inventory and last position.
	// See players.go for how they are found and why that way. Empty on a
	// world nobody has joined.
	Players []PlayerCharacter `json:"players"`

	// File and ModTime identify which save file, and which vintage of it,
	// this parse came from. Filled by Source.Parse, not by Parse.
	File    string    `json:"file"`
	ModTime time.Time `json:"modTime"`
}

// fieldCountCap bounds the CINF name table. The real table has 18 entries;
// a length prefix beyond this is a corrupt or misread file, not a world
// with a thousand settings.
const fieldCountCap = 1024

// Parse reads a whole save file image. It fails loudly on anything that
// contradicts the mapped layout — a half-written or truncated save should
// error, not half-succeed (that is also why savecache has a settle window).
func Parse(data []byte) (*World, error) {
	r := &reader{b: data}
	id, payload, err := r.chunk()
	if err != nil {
		return nil, fmt.Errorf("reading outer chunk: %w", err)
	}
	if id != "SAVE" {
		return nil, fmt.Errorf("not a Dragonwilds save: leading chunk is %q, want SAVE", id)
	}

	w := &World{}
	infoSeen := false
	body := &reader{b: payload}
	for body.rest() > 0 {
		id, sub, err := body.chunk()
		if err != nil {
			return nil, fmt.Errorf("walking top-level chunks: %w", err)
		}
		w.Chunks = append(w.Chunks, Chunk{ID: id, Bytes: len(sub)})
		switch id {
		case "INFO":
			if err := parseInfo(sub, w); err != nil {
				return nil, fmt.Errorf("INFO chunk: %w", err)
			}
			infoSeen = true
		case "LVLS":
			if err := parseLevels(sub, w); err != nil {
				return nil, fmt.Errorf("LVLS chunk: %w", err)
			}
		}
	}
	if !infoSeen {
		return nil, fmt.Errorf("save has no INFO chunk")
	}
	// Character records ride inside the object state this parser does not
	// otherwise walk; the scan is wrapper-independent (see players.go) and
	// needs the world guid decoded above to resolve per-world playtime.
	w.Players = scanPlayers(data, w.SaveGuid)
	return w, nil
}

// parseInfo decodes the INFO payload: a fixed prefix, a timestamp string,
// then the CINF field tables.
func parseInfo(payload []byte, w *World) error {
	r := &reader{b: payload}
	// The prefix is u16 + four u32 + u8 of unknown meaning (observed
	// 8, 522, 1017, 0, 255, 0 — the 522/1017 pair repeats in each LEVL
	// header, so they read as engine/game version stamps). Skipped, not
	// interpreted.
	if _, err := r.bytes(2 + 4*4 + 1); err != nil {
		return fmt.Errorf("header prefix: %w", err)
	}
	stamp, err := r.str()
	if err != nil {
		return fmt.Errorf("header timestamp: %w", err)
	}
	w.HeaderStamp = stamp

	id, sub, err := r.chunk()
	if err != nil {
		return fmt.Errorf("field table chunk: %w", err)
	}
	if id != "CINF" {
		return fmt.Errorf("expected CINF after header, found %q — save layout changed", id)
	}
	fields, err := parseFieldTable(sub)
	if err != nil {
		return fmt.Errorf("CINF: %w", err)
	}
	return decodeFields(fields, w)
}

// parseFieldTable reads CINF's three tables into name → raw value bytes.
func parseFieldTable(payload []byte) (map[string][]byte, error) {
	r := &reader{b: payload}
	n, err := r.u32()
	if err != nil {
		return nil, fmt.Errorf("name count: %w", err)
	}
	if n > fieldCountCap {
		return nil, fmt.Errorf("implausible field count %d", n)
	}
	names := make([]string, n)
	for i := range names {
		if names[i], err = r.str(); err != nil {
			return nil, fmt.Errorf("name %d: %w", i, err)
		}
	}
	// The offset table repeats the entry count, then holds fence-post
	// offsets: one more than there are names, the last being the end of
	// the values blob.
	m, err := r.u32()
	if err != nil {
		return nil, fmt.Errorf("offset count: %w", err)
	}
	if m != n {
		return nil, fmt.Errorf("offset table counts %d entries for %d names", m, n)
	}
	offs := make([]uint32, n+1)
	for i := range offs {
		if offs[i], err = r.u32(); err != nil {
			return nil, fmt.Errorf("offset %d: %w", i, err)
		}
	}
	values := r.b[r.off:]
	fields := make(map[string][]byte, n)
	for i, name := range names {
		lo, hi := offs[i], offs[i+1]
		if lo > hi || int(hi) > len(values) {
			return nil, fmt.Errorf("field %q spans %d..%d of a %d-byte value blob", name, lo, hi, len(values))
		}
		fields[name] = values[lo:hi]
	}
	return fields, nil
}

// decodeFields interprets the raw field values whose names this parser
// knows. A missing name leaves its World field zero; an unknown name is
// ignored — both are how a newer game build should degrade.
func decodeFields(fields map[string][]byte, w *World) error {
	var err error
	take := func(name string, f func(raw []byte) error) {
		raw, ok := fields[name]
		if !ok || err != nil {
			return
		}
		if ferr := f(raw); ferr != nil {
			err = fmt.Errorf("field %q: %w", name, ferr)
		}
	}

	take("VERSION", asU32(&w.Version))
	take("Meta_SaveFileRevision", asU32(&w.SaveFileRevision))
	take("WorldName", asStr(&w.WorldName))
	take("WorldMapName", asStr(&w.MapName))
	take("WorldOwnerId", asStr(&w.OwnerID))
	take("WorldNameOwner", asStr(&w.OwnerName))
	take("LastSavedBy", asStr(&w.LastSavedBy))
	take("FriendlyFire", asBool8(&w.FriendlyFire))
	take("SurvivalDifficulty", asU32(&w.SurvivalDifficulty))
	take("HardcoreState", asU32(&w.HardcoreState))
	take("SessionPrivacy", asU32(&w.SessionPrivacy))
	take("CrossplayEnabled", asBool32(&w.CrossplayEnabled))
	take("SessionPasswd", func(raw []byte) error {
		s := ""
		if ferr := asStr(&s)(raw); ferr != nil {
			return ferr
		}
		w.HasSessionPassword = s != ""
		return nil
	})
	take("TimeOfSave", func(raw []byte) error {
		if len(raw) != 8 {
			return fmt.Errorf("want 8 bytes, have %d", len(raw))
		}
		w.TimeOfSave = fromTicks(binary.LittleEndian.Uint64(raw))
		return nil
	})

	// The GUID is stored as four u32 quads; the server renders them as
	// consecutive %08X. Only rendered when all four are present.
	var a, b, c, d uint32
	take("GUID_A", asU32(&a))
	take("GUID_B", asU32(&b))
	take("GUID_C", asU32(&c))
	take("GUID_D", asU32(&d))
	if err == nil {
		if _, ok := fields["GUID_A"]; ok {
			w.SaveGuid = fmt.Sprintf("%08X%08X%08X%08X", a, b, c, d)
		}
	}
	return err
}

// parseLevels collects the name each LEVL sub-chunk of LVLS opens with.
func parseLevels(payload []byte, w *World) error {
	r := &reader{b: payload}
	for r.rest() > 0 {
		id, sub, err := r.chunk()
		if err != nil {
			return err
		}
		if id != "LEVL" {
			continue
		}
		name, err := (&reader{b: sub}).str()
		if err != nil {
			return fmt.Errorf("level name: %w", err)
		}
		w.Levels = append(w.Levels, name)
	}
	return nil
}

// fromTicks converts an FDateTime tick count (100 ns intervals since
// 0001-01-01) to a time. Ticks overflow time.Duration, so the arithmetic
// goes through time.Date's normalizing seconds argument instead.
func fromTicks(ticks uint64) time.Time {
	if ticks == 0 {
		return time.Time{}
	}
	const perSecond = 10_000_000
	return time.Date(1, 1, 1, 0, 0, int(ticks/perSecond), int(ticks%perSecond)*100, time.UTC)
}

func asU32(dst *uint32) func([]byte) error {
	return func(raw []byte) error {
		if len(raw) != 4 {
			return fmt.Errorf("want 4 bytes, have %d", len(raw))
		}
		*dst = binary.LittleEndian.Uint32(raw)
		return nil
	}
}

func asBool8(dst *bool) func([]byte) error {
	return func(raw []byte) error {
		if len(raw) != 1 {
			return fmt.Errorf("want 1 byte, have %d", len(raw))
		}
		*dst = raw[0] != 0
		return nil
	}
}

func asBool32(dst *bool) func([]byte) error {
	return func(raw []byte) error {
		var v uint32
		if err := asU32(&v)(raw); err != nil {
			return err
		}
		*dst = v != 0
		return nil
	}
}

// asStr decodes a UE string: u32 length including the trailing NUL, then
// the bytes. A zero length is an empty string with no bytes at all.
func asStr(dst *string) func([]byte) error {
	return func(raw []byte) error {
		s, err := (&reader{b: raw}).str()
		if err != nil {
			return err
		}
		*dst = s
		return nil
	}
}

// reader is a bounds-checked cursor over a byte slice. Every read error
// names the position so a corrupt save diagnoses itself.
type reader struct {
	b   []byte
	off int
}

func (r *reader) rest() int { return len(r.b) - r.off }

func (r *reader) bytes(n int) ([]byte, error) {
	if n < 0 || r.rest() < n {
		return nil, fmt.Errorf("need %d bytes at offset %d, have %d", n, r.off, r.rest())
	}
	out := r.b[r.off : r.off+n]
	r.off += n
	return out, nil
}

func (r *reader) u32() (uint32, error) {
	b, err := r.bytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

// str reads a u32 length (counting the trailing NUL) then that many bytes,
// returning the string without the NUL.
func (r *reader) str() (string, error) {
	n, err := r.u32()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	if int(n) > r.rest() {
		return "", fmt.Errorf("string of %d bytes at offset %d overruns the buffer", n, r.off)
	}
	b, _ := r.bytes(int(n))
	if b[n-1] == 0 {
		b = b[:n-1]
	}
	return string(b), nil
}

// chunk reads one FourCC + u32 length + payload.
func (r *reader) chunk() (string, []byte, error) {
	id, err := r.bytes(4)
	if err != nil {
		return "", nil, fmt.Errorf("chunk id: %w", err)
	}
	n, err := r.u32()
	if err != nil {
		return "", nil, fmt.Errorf("chunk %q length: %w", id, err)
	}
	payload, err := r.bytes(int(n))
	if err != nil {
		return "", nil, fmt.Errorf("chunk %q payload: %w", id, err)
	}
	return string(id), payload, nil
}
