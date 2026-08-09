package agentctl

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
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

func TestExtractTarRejectsEscapes(t *testing.T) {
	for _, name := range []string{"../evil.sav", "a/../../evil.sav", "/etc/evil.sav"} {
		dir := filepath.Join(t.TempDir(), "out")
		if err := extractTar(tarOf(t, map[string]string{name: "x"}), dir); err == nil {
			t.Errorf("entry %q extracted without error", name)
		}
	}
}
