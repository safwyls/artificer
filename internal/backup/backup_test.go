package backup

import (
	"archive/zip"
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/safwyls/dwcon/internal/agentfiles"
	"github.com/safwyls/dwcon/internal/store"
)

// fakeSave lays out a save directory shaped like a real Dragonwilds world:
// a .sav big enough to pass the truncation check, a smaller sibling, a
// non-.sav stray, and a nested backup dir that must be skipped.
func fakeSave(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	level := append([]byte{0, 0, 0, 0, 0, 0, 0, 0}, []byte("GVAS")...)
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
	mustWrite("Ashenfall.sav", level)
	mustWrite("Ashenfall_old.sav", []byte("older world file"))
	mustWrite("Players/1111.sav", []byte("p1"))
	mustWrite("Players/1111_dps.sav", []byte("dps"))
	mustWrite("stray.txt", []byte("not a save"))
	mustWrite("backup/old.sav", []byte("the game's own backup"))
	return dir
}

func testRunner(t *testing.T) *Runner {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return New(nil, nil, logger, t.TempDir(), agentfiles.New(t.TempDir(), logger))
}

func srvWith(saveDir string) *store.Server {
	return &store.Server{ID: 7, Name: "test", SavePath: saveDir, BackupKeep: 3, Enabled: true}
}

func archiveNames(t *testing.T, path string) map[string]bool {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	return names
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
	names := archiveNames(t, path)
	for _, want := range []string{"Ashenfall.sav", "Ashenfall_old.sav", "Players/1111.sav", "Players/1111_dps.sav"} {
		if !names[want] {
			t.Errorf("archive missing %s (has %v)", want, names)
		}
	}
	if names["stray.txt"] || names["backup/old.sav"] {
		t.Errorf("archive includes files it must skip: %v", names)
	}

	snaps, err := r.List(7)
	if err != nil || len(snaps) != 1 || snaps[0].Name != snap.Name {
		t.Fatalf("list = %v, %v", snaps, err)
	}
}

func TestBackupRejectsTornSav(t *testing.T) {
	r := testRunner(t)
	save := t.TempDir()
	// Below the truncation floor — a save caught mid-write or corrupt.
	if err := os.WriteFile(filepath.Join(save, "World.sav"), []byte("torn"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.BackupNow(context.Background(), srvWith(save)); err == nil {
		t.Fatal("expected a torn world save to be rejected")
	}
	// A failed backup must leave no partial archive behind.
	if snaps, _ := r.List(7); len(snaps) != 0 {
		t.Fatalf("snaps = %v, want none", snaps)
	}
	var files []string
	filepath.WalkDir(r.root, func(p string, d fs.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if len(files) != 0 {
		t.Fatalf("leftover files: %v", files)
	}
}

func TestRetentionKeepsNewest(t *testing.T) {
	r := testRunner(t)
	save := fakeSave(t)
	dir := filepath.Join(r.root, "7")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Seed old snapshots by name; keep=3 means after one real backup only
	// the two newest seeds survive alongside it.
	for _, name := range []string{"20260101-000000.zip", "20260102-000000.zip", "20260103-000000.zip"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.BackupNow(context.Background(), srvWith(save)); err != nil {
		t.Fatalf("backup: %v", err)
	}
	snaps, err := r.List(7)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 3 {
		t.Fatalf("got %d snapshots, want 3 (keep)", len(snaps))
	}
	for _, s := range snaps {
		if s.Name == "20260101-000000.zip" {
			t.Error("oldest snapshot should have been pruned")
		}
	}
}

func TestPathRejectsTraversal(t *testing.T) {
	r := testRunner(t)
	for _, name := range []string{"../../../etc/passwd", "..%2Fsecret.zip", "20260101-000000.zip.tmp", "notatimestamp.zip"} {
		if _, err := r.Path(7, name); err == nil {
			t.Errorf("Path(%q) should be rejected", name)
		}
	}
}

func TestIsDueSkipsUnchangedSave(t *testing.T) {
	r := testRunner(t)
	save := fakeSave(t)
	srv := srvWith(save)
	srv.BackupIntervalHours = 1

	// No snapshot yet: due.
	due, err := r.isDue(context.Background(), srv)
	if err != nil || !due {
		t.Fatalf("first backup: due=%v err=%v, want due", due, err)
	}
	if _, err := r.BackupNow(context.Background(), srv); err != nil {
		t.Fatal(err)
	}
	// Fresh snapshot: not due regardless of save mtime.
	if due, _ := r.isDue(context.Background(), srv); due {
		t.Fatal("should not be due right after a backup")
	}

	// Age the snapshot past the interval WITHOUT touching the save: the
	// interval has passed but nothing changed, so still not due.
	snaps, _ := r.List(srv.ID)
	old := time.Now().Add(-2*time.Hour).UTC().Format(nameFormat) + ".zip"
	dir := filepath.Join(r.root, "7")
	if err := os.Rename(filepath.Join(dir, snaps[0].Name), filepath.Join(dir, old)); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-3 * time.Hour)
	// Age every world file: isDue reads the newest .sav, whichever that is.
	for _, name := range []string{"Ashenfall.sav", "Ashenfall_old.sav"} {
		if err := os.Chtimes(filepath.Join(save, name), past, past); err != nil {
			t.Fatal(err)
		}
	}
	if due, _ := r.isDue(context.Background(), srv); due {
		t.Fatal("unchanged save should not be re-archived")
	}

	// Touch the save newer than the snapshot: due again.
	now := time.Now()
	if err := os.Chtimes(filepath.Join(save, "Ashenfall.sav"), now, now); err != nil {
		t.Fatal(err)
	}
	if due, _ := r.isDue(context.Background(), srv); !due {
		t.Fatal("changed save past the interval should be due")
	}
}
