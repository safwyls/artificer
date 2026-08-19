// The SPUD object layer: the part of the save below GLOB/LVLS, walked
// just far enough to reach per-character state stored as binary
// properties. Newer game builds (seen on a real server save, 2026-08-19)
// no longer embed the JSON character record in the world save — the world
// carries each character's transform in a SavedCharacterTransformsManager
// actor instead, as UE 5.4 tagged properties inside a SPUD opaque record.
// This file parses exactly that path; the byte-level layout is documented
// in the recon's "SPUD object layer mapped" section and was verified
// against both the committed empty capture and the played save.
//
// Everything here is deliberately tolerant: the object layer is the
// game's own serialization and shifts between builds, so a walk that
// stops making sense abandons that object (or that level) rather than
// failing the world parse — the INFO header stays the loud part.
package dwsave

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

// charTransform is one record of the SavedCharacterTransforms opaque
// property: which character, where they last stood, and the save's own
// freshness stamp for the record.
type charTransform struct {
	guid        string // four u32 quads rendered %08X, the server-log form
	pos         Position
	lastUpdated float64
}

// collectTransforms walks the whole container for character transform
// records. Any structural surprise abandons the current scope silently —
// the caller merges whatever was found.
func collectTransforms(data []byte) []charTransform {
	var out []charTransform
	r := &reader{b: data}
	id, payload, err := r.chunk()
	if err != nil || id != "SAVE" {
		return out
	}
	body := &reader{b: payload}
	for body.rest() > 0 {
		id, sub, err := body.chunk()
		if err != nil {
			return out
		}
		switch id {
		case "GLOB":
			out = append(out, transformsInScope(sub, true)...)
		case "LVLS":
			lr := &reader{b: sub}
			for lr.rest() > 0 {
				lid, lsub, err := lr.chunk()
				if err != nil {
					break
				}
				if lid == "LEVL" {
					out = append(out, transformsInScope(lsub, false)...)
				}
			}
		}
	}
	return out
}

// transformsInScope reads one GLOB or LEVL payload: the scope's class
// metadata, then its object lists, harvesting the transform records of
// any SavedCharacterTransformsManager object.
func transformsInScope(payload []byte, global bool) []charTransform {
	r := &reader{b: payload}
	if _, err := r.str(); err != nil { // CurrentLevel / level name
		return nil
	}
	if !global {
		// LEVL carries two u32 version stamps after its name; GLOB does not.
		if _, err := r.bytes(8); err != nil {
			return nil
		}
	}
	var meta *spudMeta
	var out []charTransform
	for r.rest() > 0 {
		id, sub, err := r.chunk()
		if err != nil {
			return out
		}
		switch id {
		case "META":
			meta = parseSpudMeta(sub)
		case "GOBS", "LATS", "SATS":
			if meta == nil {
				continue
			}
			out = append(out, transformsInObjectList(sub, meta)...)
		}
	}
	return out
}

// spudMeta is a scope's class metadata: parallel name/def tables (class
// def i belongs to class name i, as SPUD maintains them).
type spudMeta struct {
	classNames []string
	propNames  []string
	classDefs  [][]spudPropDef
}

type spudPropDef struct {
	nameID   uint32
	prefixID uint32
	dataType uint16
}

func parseSpudMeta(payload []byte) *spudMeta {
	m := &spudMeta{}
	r := &reader{b: payload}
	for r.rest() > 0 {
		id, sub, err := r.chunk()
		if err != nil {
			return m
		}
		switch id {
		case "CNIX":
			m.classNames = parseNameIndex(sub)
		case "PNIX":
			m.propNames = parseNameIndex(sub)
		case "CLST":
			cr := &reader{b: sub}
			for cr.rest() > 0 {
				cid, csub, err := cr.chunk()
				if err != nil {
					break
				}
				if cid != "CDVE" || len(csub) < 1 {
					continue
				}
				// CDVE: one version byte, then a stock CDEF chunk.
				dr := &reader{b: csub[1:]}
				did, dsub, err := dr.chunk()
				if err != nil || did != "CDEF" {
					m.classDefs = append(m.classDefs, nil)
					continue
				}
				m.classDefs = append(m.classDefs, parseClassDef(dsub))
			}
		}
	}
	return m
}

func parseNameIndex(payload []byte) []string {
	r := &reader{b: payload}
	n, err := r.u32()
	if err != nil || n > fieldCountCap {
		return nil
	}
	out := make([]string, 0, n)
	for i := uint32(0); i < n; i++ {
		s, err := r.str()
		if err != nil {
			return out
		}
		out = append(out, s)
	}
	return out
}

func parseClassDef(payload []byte) []spudPropDef {
	r := &reader{b: payload}
	if _, err := r.str(); err != nil { // class name, repeated from CNIX
		return nil
	}
	nb, err := r.bytes(2)
	if err != nil {
		return nil
	}
	n := binary.LittleEndian.Uint16(nb)
	defs := make([]spudPropDef, 0, n)
	for i := 0; i < int(n); i++ {
		var d spudPropDef
		if d.nameID, err = r.u32(); err != nil {
			return nil
		}
		if d.prefixID, err = r.u32(); err != nil {
			return nil
		}
		tb, err := r.bytes(2)
		if err != nil {
			return nil
		}
		d.dataType = binary.LittleEndian.Uint16(tb)
		defs = append(defs, d)
	}
	return defs
}

// transformsInObjectList walks NOBJ/SPWN children for the transforms
// manager and decodes its opaque property.
func transformsInObjectList(payload []byte, meta *spudMeta) []charTransform {
	var out []charTransform
	r := &reader{b: payload}
	for r.rest() > 0 {
		id, sub, err := r.chunk()
		if err != nil {
			return out
		}
		if id != "NOBJ" && id != "SPWN" {
			continue
		}
		or := &reader{b: sub}
		classID, err := or.u32()
		if err != nil || int(classID) >= len(meta.classNames) {
			continue
		}
		if !strings.HasSuffix(meta.classNames[classID], ".SavedCharacterTransformsManager") {
			continue
		}
		if id == "NOBJ" {
			if _, err := or.str(); err != nil {
				continue
			}
		} else {
			if _, err := or.bytes(16); err != nil {
				continue
			}
		}
		// The Dominion component-class-id array, then two version stamps.
		nc, err := or.u32()
		if err != nil || nc > fieldCountCap {
			continue
		}
		if _, err := or.bytes(int(nc)*4 + 8); err != nil {
			continue
		}
		raw, ok := opaqueProp(or, meta, classID, "SavedCharacterTransforms")
		if !ok {
			continue
		}
		out = append(out, parseTransformRecords(raw)...)
	}
	return out
}

// opaqueProp finds one named opaque-typed property in an object's PROP
// chunk, by the scope's class def.
func opaqueProp(or *reader, meta *spudMeta, classID uint32, propName string) ([]byte, bool) {
	if int(classID) >= len(meta.classDefs) {
		return nil, false
	}
	defs := meta.classDefs[classID]
	for or.rest() > 0 {
		id, sub, err := or.chunk()
		if err != nil {
			return nil, false
		}
		if id != "PROP" {
			continue
		}
		pr := &reader{b: sub}
		n, err := pr.u32()
		if err != nil || n > fieldCountCap {
			return nil, false
		}
		offs := make([]uint32, n)
		for i := range offs {
			if offs[i], err = pr.u32(); err != nil {
				return nil, false
			}
		}
		dlen, err := pr.u32()
		if err != nil {
			return nil, false
		}
		pdata, err := pr.bytes(int(dlen))
		if err != nil {
			return nil, false
		}
		for i, d := range defs {
			if i >= int(n) || d.dataType != 64 {
				continue
			}
			if int(d.nameID) >= len(meta.propNames) || meta.propNames[d.nameID] != propName {
				continue
			}
			lo := offs[i]
			hi := dlen
			if i+1 < int(n) {
				hi = offs[i+1]
			}
			if lo > hi || int(hi) > len(pdata) {
				return nil, false
			}
			return pdata[lo:hi], true
		}
		return nil, false
	}
	return nil, false
}

// --- The UE 5.4 tagged-property stream ---
//
// An opaque record is: u32 record count, then per record a stream of
// property tags terminated by a property named "None". Each tag:
// name (FString), a recursive type-name tree (FString + u32 param count +
// params), u32 payload size, u8 flags (EPropertyTagFlags: 0x01 array
// index follows, 0x02 property guid follows, 0x08 the payload is native
// binary, 0x10 the bool value is true), then the payload. A struct
// payload without the native flag is itself a tagged stream.

type typeName struct {
	name   string
	params []typeName
}

func readTypeName(r *reader, depth int) (typeName, error) {
	var t typeName
	if depth > 8 {
		return t, fmt.Errorf("type name tree too deep")
	}
	var err error
	if t.name, err = r.str(); err != nil {
		return t, err
	}
	n, err := r.u32()
	if err != nil {
		return t, err
	}
	if n > 16 {
		return t, fmt.Errorf("implausible type param count %d", n)
	}
	for i := uint32(0); i < n; i++ {
		p, err := readTypeName(r, depth+1)
		if err != nil {
			return t, err
		}
		t.params = append(t.params, p)
	}
	return t, nil
}

type propTag struct {
	name    string
	typ     typeName
	flags   byte
	payload []byte
}

// boolValue is only meaningful when typ.name is BoolProperty.
func (t *propTag) boolValue() bool { return t.flags&0x10 != 0 }

// structName is the struct type for a StructProperty tag.
func (t *propTag) structName() string {
	if len(t.typ.params) > 0 {
		return t.typ.params[0].name
	}
	return ""
}

// readTag reads one tag; a name of "None" ends a stream and carries no
// further fields.
func readTag(r *reader) (propTag, error) {
	var t propTag
	var err error
	if t.name, err = r.str(); err != nil {
		return t, err
	}
	if t.name == "None" {
		return t, nil
	}
	if t.typ, err = readTypeName(r, 0); err != nil {
		return t, err
	}
	size, err := r.u32()
	if err != nil {
		return t, err
	}
	fb, err := r.bytes(1)
	if err != nil {
		return t, err
	}
	t.flags = fb[0]
	if t.flags&0x01 != 0 { // array index
		if _, err := r.bytes(4); err != nil {
			return t, err
		}
	}
	if t.flags&0x02 != 0 { // property guid
		if _, err := r.bytes(16); err != nil {
			return t, err
		}
	}
	if t.payload, err = r.bytes(int(size)); err != nil {
		return t, err
	}
	return t, nil
}

// eachTag walks one tagged stream until its None terminator or the end of
// the buffer, whichever comes first (nested builtin structs end without a
// terminator).
func eachTag(raw []byte, fn func(propTag)) error {
	r := &reader{b: raw}
	for r.rest() >= 4 {
		t, err := readTag(r)
		if err != nil {
			return err
		}
		if t.name == "None" {
			return nil
		}
		fn(t)
	}
	return nil
}

// parseTransformRecords decodes the SavedCharacterTransforms opaque
// record: per character a CharacterGuid (a DomCharacterGuid wrapping a
// native Guid), a Transform whose Translation is the position, and a
// LastUpdated float.
func parseTransformRecords(raw []byte) []charTransform {
	r := &reader{b: raw}
	count, err := r.u32()
	if err != nil || count > fieldCountCap {
		return nil
	}
	var out []charTransform
	for i := uint32(0); i < count && r.rest() > 0; i++ {
		var ct charTransform
		ok := true
		for r.rest() >= 4 {
			t, err := readTag(r)
			if err != nil {
				return out // desync: keep what already parsed cleanly
			}
			if t.name == "None" {
				break
			}
			switch {
			case t.name == "CharacterGuid" && t.typ.name == "StructProperty":
				eachTag(t.payload, func(inner propTag) {
					if inner.typ.name == "StructProperty" && inner.structName() == "Guid" && len(inner.payload) == 16 {
						ct.guid = renderGuid(inner.payload)
					}
				})
			case t.name == "Transform" && t.typ.name == "StructProperty":
				eachTag(t.payload, func(inner propTag) {
					if inner.name == "Translation" && inner.structName() == "Vector" && len(inner.payload) == 24 {
						ct.pos = Position{
							X: math.Float64frombits(binary.LittleEndian.Uint64(inner.payload[0:])),
							Y: math.Float64frombits(binary.LittleEndian.Uint64(inner.payload[8:])),
							Z: math.Float64frombits(binary.LittleEndian.Uint64(inner.payload[16:])),
						}
					}
				})
			case t.name == "LastUpdated" && t.typ.name == "FloatProperty" && len(t.payload) == 4:
				ct.lastUpdated = float64(math.Float32frombits(binary.LittleEndian.Uint32(t.payload)))
			}
		}
		if ok && ct.guid != "" {
			out = append(out, ct)
		}
	}
	return out
}

// renderGuid renders 16 guid bytes the way the server logs them: four
// little-endian u32 quads as %08X each (the same rendering the world's
// own SaveGuid uses).
func renderGuid(b []byte) string {
	return fmt.Sprintf("%08X%08X%08X%08X",
		binary.LittleEndian.Uint32(b[0:]),
		binary.LittleEndian.Uint32(b[4:]),
		binary.LittleEndian.Uint32(b[8:]),
		binary.LittleEndian.Uint32(b[12:]))
}
