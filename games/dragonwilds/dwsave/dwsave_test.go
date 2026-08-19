package dwsave

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture is a genuine capture — see testdata/README.md one directory up.
const fixture = "../testdata/world-empty.sav"

var src Source

func readFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return data
}

// TestParseFixture pins every decoded field to the values the capture is
// known to hold. SaveGuid in particular is not self-referential: the same
// string was observed in the live server's log as WorldSaveGuid, so this
// asserts the renderer agrees with the game, not just with itself.
func TestParseFixture(t *testing.T) {
	w, err := Parse(readFixture(t))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if w.WorldName != "World-75058" {
		t.Errorf("WorldName = %q", w.WorldName)
	}
	if w.MapName != "L_World" {
		t.Errorf("MapName = %q", w.MapName)
	}
	if w.SaveGuid != "CA220B254BB44040A0666FB7646ED7FA" {
		t.Errorf("SaveGuid = %q, want the value the server logs as WorldSaveGuid", w.SaveGuid)
	}
	if w.Version != 9 {
		t.Errorf("Version = %d", w.Version)
	}
	if w.SaveFileRevision != 1 {
		t.Errorf("SaveFileRevision = %d", w.SaveFileRevision)
	}
	if w.FriendlyFire {
		t.Error("FriendlyFire = true")
	}
	if w.SurvivalDifficulty != 0 {
		t.Errorf("SurvivalDifficulty = %d", w.SurvivalDifficulty)
	}
	// 1 on a freshly created default (non-hardcore) world — whatever the
	// enum's names turn out to be, this is its observed default.
	if w.HardcoreState != 1 {
		t.Errorf("HardcoreState = %d", w.HardcoreState)
	}
	if w.CrossplayEnabled {
		t.Error("CrossplayEnabled = true")
	}
	if w.SessionPrivacy != 3 {
		t.Errorf("SessionPrivacy = %d", w.SessionPrivacy)
	}
	if w.HasSessionPassword {
		t.Error("HasSessionPassword = true on a passwordless world")
	}
	if w.OwnerID != "" || w.OwnerName != "" {
		t.Errorf("owner = %q/%q, want empty on a server-created world", w.OwnerID, w.OwnerName)
	}
	if w.LastSavedBy != "++dominion+live:232224" {
		t.Errorf("LastSavedBy = %q", w.LastSavedBy)
	}
	if w.HeaderStamp != "2026-08-09T12:37:07.652Z" {
		t.Errorf("HeaderStamp = %q", w.HeaderStamp)
	}
	// TimeOfSave and HeaderStamp record the same instant (ticks truncate to
	// the 100 ns below, hence .651 vs .652).
	want := time.Date(2026, 8, 9, 12, 37, 7, 651_000_000, time.UTC)
	if !w.TimeOfSave.Equal(want) {
		t.Errorf("TimeOfSave = %v, want %v", w.TimeOfSave, want)
	}
	if len(w.Levels) != 1 || w.Levels[0] != "L_World" {
		t.Errorf("Levels = %v", w.Levels)
	}

	var ids []string
	for _, c := range w.Chunks {
		ids = append(ids, c.ID)
	}
	if strings.Join(ids, " ") != "INFO GLOB LVLS" {
		t.Errorf("top-level chunks = %v", ids)
	}

	// A world nobody has joined holds no character records.
	if len(w.Players) != 0 {
		t.Errorf("Players = %+v, want none on an unplayed world", w.Players)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	data := readFixture(t)

	cases := map[string][]byte{
		"empty":       {},
		"short":       data[:6],
		"wrong magic": append([]byte("GVAS"), data[4:]...),
		// Truncation mid-file must error, not half-succeed: a save being
		// written in place looks exactly like this.
		"truncated": data[:len(data)/2],
	}
	for name, in := range cases {
		if _, err := Parse(in); err == nil {
			t.Errorf("%s: Parse succeeded, want error", name)
		}
	}
}

// TestParseToleratesUnknownFields simulates a newer game build: a CINF
// table whose names this parser has never heard of must parse, with the
// known fields still decoded by name.
func TestParseToleratesUnknownFields(t *testing.T) {
	str := func(s string) []byte {
		out := binary.LittleEndian.AppendUint32(nil, uint32(len(s)+1))
		return append(append(out, s...), 0)
	}
	u32 := func(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }
	chunk := func(id string, payload []byte) []byte {
		out := append([]byte(id), u32(uint32(len(payload)))...)
		return append(out, payload...)
	}

	names := [][]byte{str("WorldName"), str("FutureField")}
	values := append(str("Tiny"), 0xAB, 0xCD)
	offs := []uint32{0, uint32(len(str("Tiny"))), uint32(len(values))}

	var cinf []byte
	cinf = append(cinf, u32(2)...)
	for _, n := range names {
		cinf = append(cinf, n...)
	}
	cinf = append(cinf, u32(2)...)
	for _, o := range offs {
		cinf = append(cinf, u32(o)...)
	}
	cinf = append(cinf, values...)

	info := make([]byte, 19) // the unread prefix
	info = append(info, str("2026-01-01T00:00:00.000Z")...)
	info = append(info, chunk("CINF", cinf)...)
	save := chunk("SAVE", chunk("INFO", info))

	w, err := Parse(save)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if w.WorldName != "Tiny" {
		t.Errorf("WorldName = %q", w.WorldName)
	}
	if w.SaveGuid != "" {
		t.Errorf("SaveGuid = %q, want empty when the GUID fields are absent", w.SaveGuid)
	}
}

func TestLocate(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, age time.Duration) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, time.Now().Add(-age), time.Now().Add(-age)); err != nil {
			t.Fatal(err)
		}
		return p
	}

	older := write("World-1.sav", time.Hour)
	newest := write("World-2.sav", time.Minute)
	// The game's own rolling copy and stray files must never be picked,
	// even when newer than every real save.
	write("World-2.sav.backup", 0)
	write("notes.txt", 0)

	got, err := src.Locate(dir)
	if err != nil {
		t.Fatalf("Locate(dir): %v", err)
	}
	if got != newest {
		t.Errorf("Locate(dir) = %s, want %s", got, newest)
	}

	// A direct file path is taken verbatim, newest or not.
	if got, err := src.Locate(older); err != nil || got != older {
		t.Errorf("Locate(file) = %s, %v", got, err)
	}

	if _, err := src.Locate(filepath.Join(dir, "missing")); err == nil {
		t.Error("Locate(missing) succeeded")
	}
	empty := t.TempDir()
	if _, err := src.Locate(empty); err == nil {
		t.Error("Locate(empty dir) succeeded, want a named error")
	}
}

// TestSourceParse runs the full Source path over the real fixture, as the
// savecache would.
func TestSourceParse(t *testing.T) {
	mod := time.Date(2026, 8, 9, 20, 0, 0, 0, time.UTC)
	w, err := src.Parse(context.Background(), fixture, mod)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if w.File != "world-empty.sav" {
		t.Errorf("File = %q", w.File)
	}
	if !w.ModTime.Equal(mod) {
		t.Errorf("ModTime = %v", w.ModTime)
	}
	if w.WorldName != "World-75058" {
		t.Errorf("WorldName = %q", w.WorldName)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := src.Parse(cancelled, fixture, mod); err == nil {
		t.Error("Parse with cancelled context succeeded")
	}
}
