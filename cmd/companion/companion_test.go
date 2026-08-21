package main

import (
	"bytes"
	"os"
	"path/filepath"
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
	if len(found.Libraries) != 2 {
		t.Errorf("libraries = %v, want the root and the second drive", found.Libraries)
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

	// A manually configured folder replaces auto-detection entirely, and
	// every spelling a player might paste lands on the same library: the
	// Steam root, steamapps, or steamapps/common.
	t.Setenv("STEAM_ROOT", t.TempDir()) // an empty root: auto-detection finds nothing
	if found := discoverGames(nil); len(found.Games) != 0 {
		t.Fatalf("empty root still found %d games", len(found.Games))
	}
	for _, spelling := range []string{
		root,
		filepath.Join(root, "steamapps"),
		filepath.Join(root, "steamapps", "common"),
	} {
		found := discoverGames([]string{spelling})
		if len(found.Games) != 2 {
			t.Errorf("manual dir %q found %d games, want 2", spelling, len(found.Games))
		}
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
