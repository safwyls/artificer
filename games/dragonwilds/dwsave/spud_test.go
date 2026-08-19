package dwsave

import (
	"encoding/base64"
	"encoding/binary"
	"math"
	"os"
	"testing"
)

// Builders for the binary fixture: the UE 5.4 tagged-property grammar and
// the SPUD scope around it, byte-shaped like the real played save the
// layout was mapped from (recon, 2026-08-19).

func bstr(s string) []byte {
	out := binary.LittleEndian.AppendUint32(nil, uint32(len(s)+1))
	return append(append(out, s...), 0)
}

func bu32(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }

func bchunk(id string, payload []byte) []byte {
	out := append([]byte(id), bu32(uint32(len(payload)))...)
	return append(out, payload...)
}

// btree writes a type-name tree: name, param count, params.
func btree(name string, params ...[]byte) []byte {
	out := append(bstr(name), bu32(uint32(len(params)))...)
	for _, p := range params {
		out = append(out, p...)
	}
	return out
}

// btag writes one property tag.
func btag(name string, tree []byte, flags byte, payload []byte) []byte {
	out := append(bstr(name), tree...)
	out = append(out, bu32(uint32(len(payload)))...)
	out = append(out, flags)
	return append(out, payload...)
}

var bnone = bstr("None")

func guidBytes(seed byte) []byte {
	b := make([]byte, 16)
	for i := range b {
		b[i] = seed + byte(i)
	}
	return b
}

// btransformRecord builds one character's tagged record, including tags
// the parser must skip without desyncing.
func btransformRecord(guid []byte, x, y, z float64, lastUpdated float32) []byte {
	f64 := func(v float64) []byte {
		return binary.LittleEndian.AppendUint64(nil, math.Float64bits(v))
	}
	structTree := func(structName, pkg string) []byte {
		return btree("StructProperty", btree(structName, btree(pkg)))
	}

	innerGuid := btag("InnerGuid", structTree("Guid", "/Script/CoreUObject"), 0x08, guid)
	charGuid := btag("CharacterGuid", structTree("DomCharacterGuid", "/Script/Dominion"), 0,
		append(innerGuid, bnone...))

	quat := append(append(append(f64(0), f64(0)...), f64(0)...), f64(1)...)
	vec := func(a, b, c float64) []byte { return append(append(f64(a), f64(b)...), f64(c)...) }
	var tbody []byte
	tbody = append(tbody, btag("Rotation", structTree("Quat", "/Script/CoreUObject"), 0x08, quat)...)
	tbody = append(tbody, btag("Translation", structTree("Vector", "/Script/CoreUObject"), 0x08, vec(x, y, z))...)
	tbody = append(tbody, btag("Scale3D", structTree("Vector", "/Script/CoreUObject"), 0x08, vec(1, 1, 1))...)
	tbody = append(tbody, bnone...)
	transform := btag("Transform", structTree("Transform", "/Script/CoreUObject"), 0, tbody)

	last := btag("LastUpdated", btree("FloatProperty"), 0,
		binary.LittleEndian.AppendUint32(nil, math.Float32bits(lastUpdated)))
	// Tags a future build might add — must be skipped, never a desync.
	stray := btag("FutureCounter", btree("IntProperty"), 0, bu32(7))
	strayBool := btag("bFutureFlag", btree("BoolProperty"), 0x10, nil)

	var rec []byte
	rec = append(rec, charGuid...)
	rec = append(rec, transform...)
	rec = append(rec, stray...)
	rec = append(rec, strayBool...)
	rec = append(rec, last...)
	return append(rec, bnone...)
}

// btransformsLevel wraps records in the full scope: META (class + prop
// names + CDVE/CDEF defs), a LATS with the manager NOBJ, inside a LEVL.
func btransformsLevel(records ...[]byte) []byte {
	const className = "/Script/Dominion.SavedCharacterTransformsManager"

	var opaque []byte
	opaque = append(opaque, bu32(uint32(len(records)))...)
	for _, r := range records {
		opaque = append(opaque, r...)
	}

	// PROP: two properties — the opaque record, then bCanBeDamaged.
	var prop []byte
	prop = append(prop, bu32(2)...)
	prop = append(prop, bu32(0)...)
	prop = append(prop, bu32(uint32(len(opaque)))...)
	data := append(append([]byte{}, opaque...), 1)
	prop = append(prop, bu32(uint32(len(data)))...)
	prop = append(prop, data...)

	var nobj []byte
	nobj = append(nobj, bu32(0)...) // class id
	nobj = append(nobj, bstr("SavedCharacterTransformsManager_UAID_TEST_1")...)
	nobj = append(nobj, bu32(0)...)                  // no components
	nobj = append(nobj, bu32(522)...)
	nobj = append(nobj, bu32(1017)...)
	nobj = append(nobj, bchunk("PROP", prop)...)

	var cdef []byte
	cdef = append(cdef, bstr(className)...)
	cdef = append(cdef, 2, 0) // u16 property count
	cdef = append(cdef, bu32(0)...)
	cdef = append(cdef, bu32(0xFFFFFFFF)...)
	cdef = append(cdef, 64, 0)
	cdef = append(cdef, bu32(1)...)
	cdef = append(cdef, bu32(0xFFFFFFFF)...)
	cdef = append(cdef, 0, 0)
	cdve := append([]byte{0}, bchunk("CDEF", cdef)...)

	var meta []byte
	meta = append(meta, bchunk("VERS", bu32(5))...)
	meta = append(meta, bchunk("CNIX", append(bu32(1), bstr(className)...))...)
	meta = append(meta, bchunk("CLST", bchunk("CDVE", cdve))...)
	var pnix []byte
	pnix = append(pnix, bu32(2)...)
	pnix = append(pnix, bstr("SavedCharacterTransforms")...)
	pnix = append(pnix, bstr("bCanBeDamaged")...)
	meta = append(meta, bchunk("PNIX", pnix)...)

	var levl []byte
	levl = append(levl, bstr("L_World")...)
	levl = append(levl, bu32(522)...)
	levl = append(levl, bu32(1017)...)
	levl = append(levl, bchunk("META", meta)...)
	levl = append(levl, bchunk("LATS", bchunk("NOBJ", nobj))...)
	return levl
}

// spliceChunk grows a real save image by one top-level chunk.
func spliceChunk(t *testing.T, chunk []byte) []byte {
	t.Helper()
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	save := append([]byte{}, data...)
	binary.LittleEndian.PutUint32(save[4:], binary.LittleEndian.Uint32(save[4:])+uint32(len(chunk)))
	return append(save, chunk...)
}

// TestTransformRecords runs the full Parse over the real capture plus a
// spliced level holding two binary transform records — the shape the
// 2026-08-19 played save stores characters in.
func TestTransformRecords(t *testing.T) {
	g1, g2 := guidBytes(0x10), guidBytes(0x40)
	levl := btransformsLevel(
		btransformRecord(g1, 46522.25, 176842.5, -4000.5, 186648.5),
		btransformRecord(g2, 67189.25, 113003.75, 2365.25, 106841),
	)
	save := spliceChunk(t, bchunk("LVLS", bchunk("LEVL", levl)))

	w, err := Parse(save)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(w.Players) != 2 {
		t.Fatalf("Players = %+v, want the two transform records", w.Players)
	}
	p := w.Players[0]
	if p.CharGuid != renderGuid(g1) {
		t.Errorf("CharGuid = %q, want %q", p.CharGuid, renderGuid(g1))
	}
	if p.CharName != "" {
		t.Errorf("CharName = %q, want empty for a transform-only record", p.CharName)
	}
	if p.Position == nil || p.Position.X != 46522.25 || p.Position.Y != 176842.5 || p.Position.Z != -4000.5 {
		t.Errorf("Position = %+v", p.Position)
	}
	if p.LastUpdated != 186648.5 {
		t.Errorf("LastUpdated = %v", p.LastUpdated)
	}
	// Never nil on the wire: the UI maps over these.
	if p.Skills == nil || p.Inventory == nil || p.Equipment == nil {
		t.Error("transform-only record must carry empty, not nil, lists")
	}
}

// TestTransformMergesWithRecord proves the two guid spellings meet: a
// JSON record whose char_guid is the base64url form of the same 16 bytes
// merges with the binary transform, keeping the record's identity and
// taking the transform's position.
func TestTransformMergesWithRecord(t *testing.T) {
	g := guidBytes(0x77)
	rec := charRecord("Aldra", base64.RawURLEncoding.EncodeToString(g), 9)
	levl := btransformsLevel(btransformRecord(g, 1000, 2000, 300, 42))

	save := spliceChunk(t, bchunk("LVLS", bchunk("LEVL", levl)))
	// Embed the JSON record too, as a Play-style chunk.
	body := mustPrettyJSON(t, rec)
	payload := append(binary.LittleEndian.AppendUint32(nil, uint32(len(body)+1)), body...)
	payload = append(payload, 0)
	withPlay := spliceTwo(t, save, bchunk("Play", payload))

	w, err := Parse(withPlay)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(w.Players) != 1 {
		t.Fatalf("Players = %+v, want one merged character", w.Players)
	}
	p := w.Players[0]
	if p.CharName != "Aldra" || p.SaveCount != 9 {
		t.Errorf("record identity lost: %+v", p)
	}
	if p.Position == nil || p.Position.X != 1000 {
		t.Errorf("Position = %+v, want the transform's", p.Position)
	}
	if p.LastUpdated != 42 {
		t.Errorf("LastUpdated = %v", p.LastUpdated)
	}
	if len(p.Skills) != 2 {
		t.Errorf("Skills = %+v, want the record's", p.Skills)
	}
}

// spliceTwo appends another chunk to an already-spliced image.
func spliceTwo(t *testing.T, save, chunk []byte) []byte {
	t.Helper()
	out := append([]byte{}, save...)
	binary.LittleEndian.PutUint32(out[4:], binary.LittleEndian.Uint32(out[4:])+uint32(len(chunk)))
	return append(out, chunk...)
}
