package palworld

// The must-not-lose backup guards (drift ledger §F): the magic-bytes
// mid-write check, and .sav-only archive membership — asserted through
// the core backup runner with this game's SaveLayout doing the deciding,
// the way a palcon deployment runs it.

import (
	"archive/zip"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/safwyls/sampo/core/agentfiles"
	"github.com/safwyls/sampo/core/backup"
	"github.com/safwyls/sampo/core/store"
)

// A save caught mid-write would archive fine and only reveal itself on
// restore day, so the magic is checked before anything is written.
func TestVerifySavMagic(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, head []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		body := append(head, make([]byte, 64)...)
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	header := func(magic string, at int) []byte {
		h := make([]byte, 24)
		copy(h[at:], magic)
		return h
	}

	t.Run("accepts the two plain containers", func(t *testing.T) {
		for _, magic := range []string{"PlZ", "PlM"} {
			if err := verifySavMagic(write("ok_"+magic, header(magic, 8))); err != nil {
				t.Errorf("%s should be accepted: %v", magic, err)
			}
		}
	})

	t.Run("accepts an Xbox chunked container", func(t *testing.T) {
		// The real header sits 12 bytes further in.
		h := header("CNK", 8)
		copy(h[20:], "PlZ")
		if err := verifySavMagic(write("cnk", h)); err != nil {
			t.Errorf("chunked container should be accepted: %v", err)
		}
	})

	t.Run("rejects a chunked container with junk inside", func(t *testing.T) {
		h := header("CNK", 8)
		copy(h[20:], "XXX")
		if err := verifySavMagic(write("cnk_bad", h)); err == nil {
			t.Error("a chunked container with no save inside was accepted")
		}
	})

	t.Run("rejects unknown magic", func(t *testing.T) {
		if err := verifySavMagic(write("junk", header("ZIP", 8))); err == nil {
			t.Error("junk magic was accepted")
		}
	})

	t.Run("rejects a file too short to have a header", func(t *testing.T) {
		short := filepath.Join(dir, "short")
		if err := os.WriteFile(short, []byte("tiny"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := verifySavMagic(short)
		if err == nil {
			t.Fatal("a truncated save was accepted")
		}
		if !strings.Contains(err.Error(), "too short") {
			t.Errorf("unhelpful error: %v", err)
		}
	})

	t.Run("reports a missing file", func(t *testing.T) {
		if err := verifySavMagic(filepath.Join(dir, "nope")); err == nil {
			t.Error("a missing save was accepted")
		}
	})
}

// levelSavPath accepts either the save directory or the Level.sav inside
// it, since both spellings turn up in configured paths.
func TestLevelSavPath(t *testing.T) {
	dir := t.TempDir()
	level := filepath.Join(dir, "Level.sav")
	if err := os.WriteFile(level, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := levelSavPath(dir); err != nil || got != level {
		t.Errorf("directory form = %q, %v", got, err)
	}
	for _, direct := range []string{"/saves/world/Level.sav", "/saves/world/level.sav", "/saves/world/LEVEL.SAV"} {
		if got, err := levelSavPath(direct); err != nil || got != direct {
			t.Errorf("%q should be used as-is, got %q, %v", direct, got, err)
		}
	}
	if _, err := levelSavPath(t.TempDir()); err == nil {
		t.Error("a save dir without a Level.sav should be an error")
	}
}

// fakeSave lays out a save directory shaped like a real world: valid
// magic on Level.sav, player files, a non-.sav stray, and a nested
// backup dir that must be skipped.
func fakeSave(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	level := append([]byte{0, 0, 0, 0, 0, 0, 0, 0}, []byte("PlZ")...)
	level = append(level, make([]byte, 64)...)
	mustWrite := func(rel string, data []byte) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("Level.sav", level)
	mustWrite("LevelMeta.sav", []byte("meta"))
	mustWrite("Players/1111.sav", []byte("p1"))
	mustWrite("Players/1111_dps.sav", []byte("dps"))
	mustWrite("stray.txt", []byte("not a save"))
	mustWrite("backup/old.sav", []byte("the game's own backup"))
	return dir
}

func testRunner(t *testing.T) *backup.Runner {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return backup.New(nil, nil, logger, t.TempDir(), agentfiles.New(t.TempDir(), logger))
}

// srvWith carries the game id, which is what routes the runner to this
// package's SaveLayout (the init in palworld.go registered it).
func srvWith(saveDir string) *store.Server {
	return &store.Server{ID: 7, Name: "test", Game: Definition.ID, SavePath: saveDir, BackupKeep: 3, Enabled: true}
}

func TestBackupArchivesSavFilesOnly(t *testing.T) {
	r := testRunner(t)
	save := fakeSave(t)

	snap, err := r.BackupNow(context.Background(), srvWith(save))
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	path, err := r.Path(7, snap.Name)
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"Level.sav", "LevelMeta.sav", "Players/1111.sav", "Players/1111_dps.sav"} {
		if !names[want] {
			t.Errorf("archive missing %s (has %v)", want, names)
		}
	}
	if names["stray.txt"] || names["backup/old.sav"] {
		t.Errorf("archive includes files it must skip: %v", names)
	}
}

func TestBackupRejectsTornLevelSav(t *testing.T) {
	r := testRunner(t)
	save := t.TempDir()
	// Plausible length, wrong magic — a save caught mid-write.
	torn := append([]byte("not a palworld header"), make([]byte, 64)...)
	if err := os.WriteFile(filepath.Join(save, "Level.sav"), torn, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.BackupNow(context.Background(), srvWith(save)); err == nil {
		t.Fatal("expected a torn world save to be rejected")
	}
	if snaps, _ := r.List(7); len(snaps) != 0 {
		t.Fatalf("snaps = %v, want none", snaps)
	}
}
