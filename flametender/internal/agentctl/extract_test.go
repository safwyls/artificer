package agentctl

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tarOf(t *testing.T, entries map[string]string) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)
	for name, content := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	return buf
}

func TestExtractTar(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	err := extractTar(tarOf(t, map[string]string{"Level.sav": "x", "Players/1.sav": "y"}), dir)
	if err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{"Level.sav": "x", "Players/1.sav": "y"} {
		got, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v", rel, got, err)
		}
	}
}

// TestExtractTarKeepsModTime: the bundle's file times must survive into
// the mirror — the save cache keys on them and the UI reports them as when
// the game last saved — except epoch-era times from agents that never set
// ModTime, which would be worse than the extraction time they replace.
func TestExtractTarKeepsModTime(t *testing.T) {
	saved := time.Date(2026, 8, 9, 21, 46, 0, 0, time.UTC)

	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)
	for name, mod := range map[string]time.Time{"World.sav": saved, "Old.sav": {}} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: 1, ModTime: mod}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()

	dir := filepath.Join(t.TempDir(), "out")
	if err := extractTar(buf, dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "World.sav"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(saved) {
		t.Errorf("World.sav mtime = %v, want %v", info.ModTime(), saved)
	}
	old, err := os.Stat(filepath.Join(dir, "Old.sav"))
	if err != nil {
		t.Fatal(err)
	}
	if old.ModTime().Before(mtimeFloor) {
		t.Errorf("Old.sav took the epoch mtime %v; an unset bundle time should be ignored", old.ModTime())
	}
}

func TestExtractTarRejectsEscapes(t *testing.T) {
	for _, name := range []string{"../evil.sav", "a/../../evil.sav", "/etc/evil.sav"} {
		dir := filepath.Join(t.TempDir(), "out")
		if err := extractTar(tarOf(t, map[string]string{name: "x"}), dir); err == nil {
			t.Errorf("entry %q extracted without error", name)
		}
	}
}
