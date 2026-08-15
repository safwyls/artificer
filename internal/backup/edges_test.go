package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/safwyls/flamekeeper/internal/store"
)

// The world file is whatever the server would load: the newest .sav. The
// verify step only rejects obvious truncation — the save format itself is
// unverified (recon doc), so no magic-bytes rule exists to enforce.
func TestNewestSavAndVerify(t *testing.T) {
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
		write("older.sav", 64, now.Add(-time.Hour))
		newest := write("current.sav", 64, now)
		got, err := newestSav(dir)
		if err != nil || got != newest {
			t.Errorf("newestSav = %q, %v; want %q", got, err, newest)
		}
	})

	t.Run("empty directory reports no saves", func(t *testing.T) {
		if _, err := newestSav(t.TempDir()); err == nil {
			t.Error("an empty save dir was accepted")
		}
	})

	t.Run("rejects a truncated save", func(t *testing.T) {
		torn := write("torn.sav", 4, time.Now())
		err := verifySav(torn)
		if err == nil {
			t.Fatal("a truncated save was accepted")
		}
		if !strings.Contains(err.Error(), "bytes") {
			t.Errorf("unhelpful error: %v", err)
		}
	})

	t.Run("accepts a plausible save", func(t *testing.T) {
		ok := write("world.sav", 64, time.Now())
		if err := verifySav(ok); err != nil {
			t.Errorf("plausible save rejected: %v", err)
		}
	})

	t.Run("reports a missing file", func(t *testing.T) {
		if err := verifySav(filepath.Join(dir, "nope.sav")); err == nil {
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
