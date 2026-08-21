package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The config migration chain: wkcompanion-era and first-cut Artificer
// Companion configs both map forward, and a config with no sync side
// maps to empty — its relay credential has nothing to authenticate.
func TestConfigMigration(t *testing.T) {
	legacy := []byte(`{
		"consoleUrl": "https://wilds.example.com",
		"token": "companion-relay-token",
		"saveDir": "C:\\chars",
		"sync": {"token": "personal-sync-token", "worldId": 3, "worldDir": "C:\\worlds\\midgard", "sessionId": 7, "baseVersion": 5}
	}`)
	cfg, err := parseConfig(legacy)
	if err != nil {
		t.Fatalf("parse legacy: %v", err)
	}
	if cfg.ServerURL != "https://wilds.example.com" || cfg.Token != "personal-sync-token" {
		t.Errorf("migrated connection = %q %q", cfg.ServerURL, cfg.Token)
	}
	if len(cfg.Links) != 1 || cfg.Links[0].WorldID != 3 || cfg.Links[0].SessionID != 7 {
		t.Errorf("migrated link = %+v", cfg.Links)
	}

	relayOnly := []byte(`{"consoleUrl": "https://wilds.example.com", "token": "relay-only"}`)
	cfg, err = parseConfig(relayOnly)
	if err != nil {
		t.Fatalf("parse relay-only: %v", err)
	}
	if cfg.configured() {
		t.Errorf("relay-only config should not map to a sync connection: %+v", cfg)
	}

	current := []byte(`{"serverUrl": "https://vault.example.com", "token": "tok", "links": [{"worldId": 1, "dir": "/w"}]}`)
	cfg, err = parseConfig(current)
	if err != nil || cfg.ServerURL != "https://vault.example.com" || len(cfg.Links) != 1 {
		t.Errorf("current shape mangled: %+v (%v)", cfg, err)
	}
}

// Steam discovery: the library list follows libraryfolders.vdf and the
// manifests yield name, app id and install dir.
func TestSteamDiscovery(t *testing.T) {
	root := t.TempDir()
	second := t.TempDir()
	main := filepath.Join(root, "steamapps")
	os.MkdirAll(main, 0o755)
	os.MkdirAll(filepath.Join(second, "steamapps"), 0o755)
	os.WriteFile(filepath.Join(main, "libraryfolders.vdf"), []byte(`
"libraryfolders"
{
	"0" { "path" "`+root+`" }
	"1" { "path" "`+second+`" }
}`), 0o644)
	os.WriteFile(filepath.Join(main, "appmanifest_1374490.acf"), []byte(`
"AppState"
{
	"appid"		"1374490"
	"name"		"RuneScape: Dragonwilds"
	"installdir"		"RSDragonwilds"
}`), 0o644)
	os.WriteFile(filepath.Join(second, "steamapps", "appmanifest_1234.acf"), []byte(`
"AppState"
{
	"appid"		"1234"
	"name"		"Some Other Game"
	"installdir"		"SomeOtherGame"
}`), 0o644)

	t.Setenv("STEAM_ROOT", root)
	found := discoverGames(nil)
	if len(found.Games) != 2 {
		t.Fatalf("found %d games, want 2: %+v", len(found.Games), found.Games)
	}
	byName := map[string]discoveredGame{}
	for _, g := range found.Games {
		byName[g.Name] = g
	}
	if g := byName["RuneScape: Dragonwilds"]; g.AppID != "1374490" || g.InstallDir != "RSDragonwilds" {
		t.Errorf("dragonwilds manifest misread: %+v", g)
	}
	if _, ok := byName["Some Other Game"]; !ok {
		t.Error("second library's manifest not found")
	}
	// The trail records both libraries as resolved hits.
	hits := 0
	for _, p := range found.Probes {
		if p.Resolved != "" && p.Note != "already scanned" {
			hits++
		}
	}
	if hits != 2 {
		t.Errorf("scan trail shows %d resolved libraries, want 2: %+v", hits, found.Probes)
	}
}

// Whatever the player pastes should land on the same library — and when
// it can't, the trail has to say why rather than dropping it silently.
// The quoted case is Windows Explorer's "Copy as path", which is how a
// pasted path most often arrives.
func TestSteamDirSpellings(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "steamapps")
	os.MkdirAll(filepath.Join(main, "common", "Palworld"), 0o755)
	os.WriteFile(filepath.Join(main, "appmanifest_1623730.acf"), []byte(`
"AppState"
{
	"appid"		"1623730"
	"name"		"Palworld"
	"installdir"		"Palworld"
}`), 0o644)
	// An empty auto-detect root, so only the configured folder can find
	// anything.
	t.Setenv("STEAM_ROOT", t.TempDir())
	if found := discoverGames(nil); len(found.Games) != 0 {
		t.Fatalf("empty root found %d games", len(found.Games))
	}

	for _, spelling := range []string{
		root,
		main,
		filepath.Join(main, "common"),
		filepath.Join(main, "common", "Palworld"),
		`"` + root + `"`,
		"  " + root + "  ",
		root + string(filepath.Separator),
	} {
		found := discoverGames([]string{spelling})
		if len(found.Games) != 1 || found.Games[0].Name != "Palworld" {
			t.Errorf("configured %q found %+v, want Palworld", spelling, found.Games)
			continue
		}
		if found.Probes[0].Resolved != main {
			t.Errorf("configured %q resolved to %q, want %q", spelling, found.Probes[0].Resolved, main)
		}
	}

	// A path that leads nowhere is reported, not dropped.
	found := discoverGames([]string{filepath.Join(root, "nope")})
	if len(found.Probes) == 0 || found.Probes[0].Resolved != "" || found.Probes[0].Note == "" {
		t.Errorf("a bad path should be reported with a reason: %+v", found.Probes)
	}
}

// A library whose manifests are missing still has games: the folders
// under common/ are the fallback, so the panel is never empty beside a
// library the player can see in Explorer.
func TestGamesFromCommonWithoutManifests(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "steamapps", "common", "Witchspire"), 0o755)
	os.MkdirAll(filepath.Join(root, "steamapps", "common", "Palworld"), 0o755)
	t.Setenv("STEAM_ROOT", root)

	found := discoverGames(nil)
	if len(found.Games) != 2 {
		t.Fatalf("found %d games from common/, want 2: %+v", len(found.Games), found.Games)
	}
	if found.Games[0].Name != "Palworld" || found.Games[1].Name != "Witchspire" {
		t.Errorf("games = %+v, want them sorted", found.Games)
	}
}

// A save folder round-trips through package + extract: same files, same
// contents, backup folders skipped, traversal refused.
func TestBundleRoundTrip(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "World.sav"), []byte("0123456789abcdef0123456789abcdef"), 0o644)
	os.MkdirAll(filepath.Join(src, "chunks"), 0o755)
	os.WriteFile(filepath.Join(src, "chunks", "0.dat"), []byte("chunkdata"), 0o644)
	os.MkdirAll(filepath.Join(src, "Backups"), 0o755)
	os.WriteFile(filepath.Join(src, "Backups", "old.sav"), []byte("stale"), 0o644)

	var buf bytes.Buffer
	if err := packageWorldDir(src, &buf); err != nil {
		t.Fatalf("package: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "restored")
	if err := extractBundleTo(&buf, dst); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "World.sav")); string(data) != "0123456789abcdef0123456789abcdef" {
		t.Errorf("world did not round-trip: %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "chunks", "0.dat")); string(data) != "chunkdata" {
		t.Errorf("nested file did not round-trip: %q", data)
	}
	if _, err := os.Stat(filepath.Join(dst, "Backups")); !os.IsNotExist(err) {
		t.Error("rolling backup folder was packaged")
	}
}

// The save finder against the shapes games actually use: Steam Cloud
// keyed by app id (exact), an Unreal-style folder under LOCALAPPDATA, a
// publisher folder two levels down, and a name that only matches after
// normalization.
func TestSaveCandidates(t *testing.T) {
	steam := t.TempDir()
	home := t.TempDir()
	local := filepath.Join(home, "AppData", "Local")
	lib := filepath.Join(steam, "steamapps")
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// Steam Cloud: userdata/<account>/<appid>/remote, non-empty.
	cloud := filepath.Join(steam, "userdata", "1234567", "1623730", "remote")
	os.MkdirAll(cloud, 0o755)
	os.WriteFile(filepath.Join(cloud, "world.sav"), []byte("x"), 0o644)

	// Unreal shape: %LOCALAPPDATA%\<InstallDir>\Saved\SaveGames
	unreal := filepath.Join(local, "Palworld", "Saved", "SaveGames")
	os.MkdirAll(unreal, 0o755)

	got := saveCandidatesFor(discoveredGame{Name: "Palworld", AppID: "1623730", InstallDir: "Palworld"}, []string{lib})
	if len(got) < 2 {
		t.Fatalf("found %d candidates, want the cloud save and the Unreal folder: %+v", len(got), got)
	}
	if got[0].Path != cloud {
		t.Errorf("strongest candidate = %q, want the Steam Cloud save %q", got[0].Path, cloud)
	}
	if !strings.Contains(got[0].Why, "Steam Cloud") {
		t.Errorf("cloud candidate reason = %q", got[0].Why)
	}
	found := false
	for _, c := range got {
		if c.Path == unreal {
			found = true
		}
	}
	if !found {
		t.Errorf("the Unreal Saved/SaveGames folder was missed: %+v", got)
	}

	// A publisher folder in between, and a name that needs normalizing:
	// "RuneScape: Dragonwilds" living under Jagex\RSDragonwilds.
	pub := filepath.Join(home, "Documents", "My Games", "Jagex", "RSDragonwilds", "Saved")
	os.MkdirAll(pub, 0o755)
	got = saveCandidatesFor(discoveredGame{Name: "RuneScape: Dragonwilds", InstallDir: "RSDragonwilds"}, nil)
	if len(got) == 0 || got[0].Path != pub {
		t.Errorf("publisher-nested save missed: %+v", got)
	}

	// A game with nothing anywhere gets nothing — no false positives.
	if got := saveCandidatesFor(discoveredGame{Name: "Some Game Nobody Installed", InstallDir: "NopeNopeNope"}, nil); len(got) != 0 {
		t.Errorf("invented candidates for an absent game: %+v", got)
	}
}
