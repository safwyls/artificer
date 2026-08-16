package backup

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/safwyls/flametender/internal/store"
)

// The world file is the newest hex-named blob — extensionless, so there is
// nothing to match on but the absence of a sidecar suffix. The verify step
// only rejects obvious truncation: the blob format is proprietary and has
// no public parser (recon doc), so no magic-bytes rule exists to enforce.
func TestNewestWorldFileAndVerify(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, size int, mod time.Time) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("picks the newest world file", func(t *testing.T) {
		now := time.Now()
		write("3ad85aea-1", 64, now.Add(-time.Hour))
		newest := write("3ad85aea", 64, now)
		got, err := newestWorldFile(dir)
		if err != nil || got != newest {
			t.Errorf("newestWorldFile = %q, %v; want %q", got, err, newest)
		}
	})

	t.Run("empty directory reports no saves", func(t *testing.T) {
		if _, err := newestWorldFile(t.TempDir()); err == nil {
			t.Error("an empty save dir was accepted")
		}
	})

	// The sidecars are rewritten on every save, so they are usually the
	// newest thing in the directory — and neither is a world. A directory
	// holding only sidecars has nothing to back up, and picking one would
	// hand the size check a 40-byte file and call the world truncated.
	t.Run("never mistakes a sidecar for the world", func(t *testing.T) {
		side := t.TempDir()
		if err := os.WriteFile(filepath.Join(side, "3ad85aea-index"), []byte(`{"latest":0}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(side, "3ad85aea-info"), []byte(`{"name":"x"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := newestWorldFile(side); err == nil {
			t.Error("a directory of sidecars was accepted as a world")
		}

		// And with a world present, the newer sidecar must not win.
		now := time.Now()
		world := filepath.Join(side, "3ad85aea")
		if err := os.WriteFile(world, make([]byte, 64), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(world, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
			t.Fatal(err)
		}
		got, err := newestWorldFile(side)
		if err != nil || got != world {
			t.Errorf("newestWorldFile = %q, %v; want the world %q", got, err, world)
		}
	})

	t.Run("rejects a truncated save", func(t *testing.T) {
		torn := write("3bd85c7d", 4, time.Now())
		err := verifyWorldFile(torn)
		if err == nil {
			t.Fatal("a truncated save was accepted")
		}
		if !strings.Contains(err.Error(), "bytes") {
			t.Errorf("unhelpful error: %v", err)
		}
	})

	t.Run("accepts a plausible save", func(t *testing.T) {
		ok := write("38d857c4", 64, time.Now())
		if err := verifyWorldFile(ok); err != nil {
			t.Errorf("plausible save rejected: %v", err)
		}
	})

	t.Run("reports a missing file", func(t *testing.T) {
		if err := verifyWorldFile(filepath.Join(dir, "nope")); err == nil {
			t.Error("a missing save was accepted")
		}
	})
}

func TestBackupNowWithoutASaveConfigured(t *testing.T) {
	r := testRunner(t)
	_, err := r.BackupNow(context.Background(), &store.Server{ID: 1, Name: "bare"})
	if err == nil {
		t.Fatal("backing up a server with no save reported success")
	}
	if !strings.Contains(err.Error(), "no save path") {
		t.Errorf("unhelpful error: %v", err)
	}
}

// A second backup while one is running is refused rather than queued or run
// concurrently — two writers on one archive directory is not worth the risk.
func TestBackupNowRefusesAConcurrentRun(t *testing.T) {
	r := testRunner(t)
	srv := srvWith(fakeSave(t))

	r.mu.Lock()
	r.running[srv.ID] = true
	r.mu.Unlock()

	if _, err := r.BackupNow(context.Background(), srv); err != ErrBusy {
		t.Errorf("concurrent backup: %v, want ErrBusy", err)
	}

	// The flag is the only thing standing in the way; clearing it lets the
	// next call through.
	r.mu.Lock()
	delete(r.running, srv.ID)
	r.mu.Unlock()
	if _, err := r.BackupNow(context.Background(), srv); err != nil {
		t.Errorf("backup after the flag cleared: %v", err)
	}
}

func TestBackupNowRejectsAMissingSaveDirectory(t *testing.T) {
	r := testRunner(t)
	srv := srvWith(filepath.Join(t.TempDir(), "does-not-exist"))

	if _, err := r.BackupNow(context.Background(), srv); err == nil {
		t.Error("backing up a missing save directory reported success")
	}
	// The busy flag must not be left set behind a failure, or the server
	// can never be backed up again without a restart.
	if r.Running(srv.ID) {
		t.Error("the running flag survived a failed backup")
	}
}

func TestListAndPathOnAServerWithNoBackups(t *testing.T) {
	r := testRunner(t)

	snaps, err := r.List(99)
	if err != nil {
		t.Fatalf("listing a server with no backups: %v", err)
	}
	if len(snaps) != 0 {
		t.Errorf("snapshots = %d, want none", len(snaps))
	}
	if _, err := r.Path(99, "whatever.zip"); err == nil {
		t.Error("Path resolved a snapshot that doesn't exist")
	}
}

// Retention keeps the newest N; a keep of zero or less is treated as the
// default rather than deleting everything.
func TestPruneHonoursKeep(t *testing.T) {
	r := testRunner(t)
	srv := srvWith(fakeSave(t))
	ctx := context.Background()

	// Three snapshots at distinct timestamps. Each is renamed a day further
	// back so the next backup can't collide on the second-resolution stamp
	// the archive name is built from.
	dir := filepath.Join(r.root, "7")
	for i := 0; i < 3; i++ {
		if _, err := r.BackupNow(ctx, srv); err != nil {
			t.Fatal(err)
		}
		snaps, _ := r.List(srv.ID)
		if len(snaps) == 0 {
			t.Fatal("no snapshot after a backup")
		}
		aged := time.Now().UTC().AddDate(0, 0, -(i+1)).Format(nameFormat) + ".zip"
		if err := os.Rename(filepath.Join(dir, snaps[0].Name), filepath.Join(dir, aged)); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := r.prune(srv.ID, 2)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if removed != 1 {
		t.Errorf("pruned %d, want 1", removed)
	}
	if snaps, _ := r.List(srv.ID); len(snaps) != 2 {
		t.Errorf("kept %d snapshots, want 2", len(snaps))
	}
}

// The regression this package existed in for the whole of Phase 1: it
// arrived from Palworld matching `*.sav`, Enshrouded's saves have no
// extension at all, and so every snapshot failed at the first step and
// wrote nothing. The old fixtures wrote `.sav` files, which is why the
// tests were green the entire time.
//
// This asserts against the real layout by name: a snapshot of a directory
// holding nothing but hex blobs and sidecars must succeed and must
// contain them.
func TestSnapshotsARealEnshroudedSaveDirectory(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		"3ad85aea":       append([]byte("ENSH"), make([]byte, 128)...),
		"3ad85aea-1":     append([]byte("ENSH"), make([]byte, 128)...),
		"3ad85aea-index": []byte(`{"latest":0,"time":1234567890,"deleted":false}`),
		"3ad85aea-info":  []byte(`{"name":"Grimwood Bastion"}`),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	r := testRunner(t)
	srv := &store.Server{ID: 7, Name: "Grimwood", SavePath: dir, BackupKeep: 3}
	snap, err := r.BackupNow(context.Background(), srv)
	if err != nil {
		t.Fatalf("snapshotting a real save layout failed: %v", err)
	}
	if snap.Bytes == 0 {
		t.Fatal("the snapshot is empty — the archive matched nothing")
	}

	path, err := r.Path(srv.ID, snap.Name)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	got := map[string]bool{}
	for _, f := range zr.File {
		got[f.Name] = true
	}
	for name := range files {
		if !got[name] {
			t.Errorf("%q is missing from the archive; it holds %v", name, got)
		}
	}

	// And a success clears any earlier failure, so a stale reason can't
	// outlive the problem.
	if f := r.LastFailure(srv.ID); f != nil {
		t.Errorf("a successful snapshot left a failure behind: %+v", f)
	}
}

// A failed snapshot has to leave a reason somewhere the UI can read. It
// runs detached from the request that started it, so without this the
// page can only show the running flag clearing and no new file — which
// is indistinguishable from nothing having happened.
func TestAFailedSnapshotRecordsWhy(t *testing.T) {
	r := testRunner(t)
	// An empty save directory: the server has not saved yet.
	srv := &store.Server{ID: 8, Name: "Grimwood", SavePath: t.TempDir(), BackupKeep: 3}

	if _, err := r.BackupNow(context.Background(), srv); err == nil {
		t.Fatal("a save directory with no world should not produce a snapshot")
	}
	f := r.LastFailure(srv.ID)
	if f == nil {
		t.Fatal("the failure was not recorded")
	}
	if !strings.Contains(f.Error, "world files") {
		t.Errorf("unhelpful recorded reason: %q", f.Error)
	}
	if f.At.IsZero() {
		t.Error("the failure has no timestamp")
	}
}
