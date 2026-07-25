package palsave

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func havePython(t *testing.T, module string) bool {
	t.Helper()
	return exec.Command("python3", "-c", "import "+module).Run() == nil
}

// assertFixture checks the extraction of the shared two-player fixture. Both
// save containers hold identical data, so both must produce identical output.
func assertFixture(t *testing.T, result *Result) {
	t.Helper()

	if len(result.Players) != 2 {
		t.Fatalf("want 2 players, got %d", len(result.Players))
	}
	kyoshi := result.Players[0]
	if kyoshi.Nickname != "Kyoshi" || kyoshi.Level != 42 {
		t.Fatalf("unexpected first player: %+v", kyoshi)
	}
	if len(kyoshi.Party) != 2 || len(kyoshi.Palbox) != 2 || len(kyoshi.Base) != 1 {
		t.Fatalf("kyoshi buckets wrong: party=%d palbox=%d base=%d",
			len(kyoshi.Party), len(kyoshi.Palbox), len(kyoshi.Base))
	}
	boss := kyoshi.Party[1]
	if boss.CharacterID != "BOSS_Anubis" || !boss.IsBoss || boss.TalentHP != 100 {
		t.Fatalf("unexpected boss pal: %+v", boss)
	}
	if !kyoshi.Palbox[1].IsLucky {
		t.Fatalf("Kitsunebi should be lucky: %+v", kyoshi.Palbox[1])
	}

	ren := result.Players[1]
	if ren.Nickname != "Ren" || len(ren.Party) != 1 || len(ren.Palbox) != 1 || len(ren.Base) != 0 {
		t.Fatalf("unexpected ren: %+v", ren)
	}
}

// A cached result for one save must return immediately even while another
// save's parse holds the parse lock — with several servers configured, the
// pals/map pages of server A shouldn't stall on server B's slow parse.
func TestCachedReadNotBlockedByOtherParse(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.py")
	script := "import json, time\ntime.sleep(1)\nprint(json.dumps({\"players\": [], \"guilds\": []}))\n"
	if err := os.WriteFile(stub, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	reader := &Reader{scriptPath: stub, cache: make(map[string]cacheEntry)}

	makeSave := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "Level.sav"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	saveA, saveB := makeSave("a"), makeSave("b")

	// Prime the cache for A (pays one stub parse).
	if _, err := reader.Read(context.Background(), saveA); err != nil {
		t.Fatal(err)
	}

	// B's parse runs in the background, holding the parse lock ~1s.
	done := make(chan error, 1)
	go func() {
		_, err := reader.Read(context.Background(), saveB)
		done <- err
	}()
	time.Sleep(100 * time.Millisecond) // let B reach the extractor

	begin := time.Now()
	if _, err := reader.Read(context.Background(), saveA); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(begin); d > 500*time.Millisecond {
		t.Errorf("cached read for A blocked %v behind B's parse", d)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// The fixtures are synthetic — see testdata/README.md — so they exercise the
// real decompress/GVAS-parse/extract path with no copyrighted game data.
func TestRead(t *testing.T) {
	if !havePython(t, "palworld_save_tools") {
		t.Skip("python3 with palworld-save-tools not available")
	}

	tests := []struct {
		name string
		// path is relative to this package; a directory must resolve to the
		// Level.sav inside it, a file must be read directly.
		path       string
		needsOodle bool
	}{
		{name: "PlZ/zlib via directory", path: "testdata"},
		{name: "PlM/oodle via file", path: "testdata/Level_oodle.sav", needsOodle: true},
		// 0.6-era layout: pals carry no OwnerPlayerUId and players keep their
		// container ids in Players/<uid>.sav, so ownership resolves by
		// container. Produced zero pals for every player before that was
		// handled — hence a fixture rather than trusting the old one.
		{name: "container-based ownership", path: "testdata/newlayout"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.needsOodle && !havePython(t, "ooz") {
				t.Skip("python3 with pyooz not available")
			}
			reader, err := NewReader(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path, err := filepath.Abs(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := reader.Read(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			assertFixture(t, result)

			// A second read of an unchanged file must come from cache —
			// verified by pointer identity, since a re-parse would allocate.
			again, err := reader.Read(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if again != result {
				t.Fatal("expected cached result on unchanged mtime")
			}
		})
	}
}
