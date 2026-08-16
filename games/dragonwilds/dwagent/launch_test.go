package dwagent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/safwyls/sampo/core/agent"
)

func profileFor(name string) agent.Profile {
	// Args arrive resolved by the kit (DefaultArgs when the operator sets
	// none) — mirror that here.
	return buildProfile(name, LaunchConfig{}, "/dragonwilds", Game(LaunchConfig{}).DefaultArgs(7777))
}

// The native profile is what every existing deployment runs; its shape must
// not move.
func TestNativeProfileRunsTheLinuxLauncher(t *testing.T) {
	p := profileFor(ProfileNative)

	if p.Command != "./RSDragonwildsServer.sh" {
		t.Errorf("command = %q", p.Command)
	}
	if !slices.Contains(p.Args, "-log") || !slices.Contains(p.Args, "-Port=7777") {
		t.Errorf("args = %v; -log is load-bearing (the log is the only player-list source)", p.Args)
	}
	if p.Mods {
		t.Error("the Linux build cannot carry UE4SS, so it must not claim mod support")
	}
	if !strings.Contains(p.ConfigRel, "LinuxServer") {
		t.Errorf("config path = %q, want the Linux config dir", p.ConfigRel)
	}
	if p.SteamPlatform != "" {
		t.Errorf("steam platform = %q, want the host's own", p.SteamPlatform)
	}
}

// Everything in this test is a detail that fails silently if it's wrong:
// the game starts, and the mod simply never appears.
func TestWineProfileCarriesWhatUE4SSNeeds(t *testing.T) {
	p := profileFor(ProfileWine)

	if p.Command != "wine" {
		t.Errorf("command = %q, want the wine binary", p.Command)
	}
	// (Bare-name PATH resolution is the kit's behavior now.)
	if len(p.Args) == 0 || !strings.HasSuffix(p.Args[0], "RSDragonwildsServer-Win64-Shipping.exe") {
		t.Fatalf("args = %v, want the Windows server exe first", p.Args)
	}
	if !filepath.IsAbs(p.Args[0]) {
		t.Errorf("exe %q should be absolute, so the working directory is free", p.Args[0])
	}

	env := strings.Join(p.Env, " ")
	// Without this Wine prefers its builtin version.dll, the shim never
	// loads, and UE4SS never injects — the mod is just quietly absent.
	if !strings.Contains(env, "WINEDLLOVERRIDES=version=n,b") {
		t.Errorf("env = %v, missing the version.dll override the shim needs", p.Env)
	}
	// The mod reads this with Windows semantics; a Linux path leaves the
	// bridge idle with no error anywhere.
	if !strings.Contains(env, `DWBRIDGE_DIR=Z:\dragonwilds\dwbridge`) {
		t.Errorf("env = %v, want DWBRIDGE_DIR as a Z:-mapped Windows path", p.Env)
	}
	if !p.Mods {
		t.Error("the Wine profile is the only one that can carry the mod")
	}
	// UE names the config dir after the platform: a Wine server never reads
	// anything written to LinuxServer/.
	if !strings.Contains(p.ConfigRel, "WindowsServer") {
		t.Errorf("config path = %q, want the Windows config dir", p.ConfigRel)
	}
	if p.SteamPlatform != "windows" {
		t.Errorf("steam platform = %q, want windows", p.SteamPlatform)
	}
}

func TestWinePrefixIsOnlySetWhenConfigured(t *testing.T) {
	bare := buildProfile(ProfileWine, LaunchConfig{}, "/g", nil)
	if strings.Contains(strings.Join(bare.Env, " "), "WINEPREFIX") {
		t.Error("an unset prefix should leave Wine's default alone, not export an empty one")
	}
	set := buildProfile(ProfileWine, LaunchConfig{WinePrefix: "/data/wine"}, "/g", nil)
	if !slices.Contains(set.Env, "WINEPREFIX=/data/wine") {
		t.Errorf("env = %v, want the configured prefix", set.Env)
	}
}

// The custom-command escape hatch is the kit's now; what stays this
// game's responsibility is that "custom" never appears in the selectable
// list it declares.
func TestCustomIsNotSelectable(t *testing.T) {
	g := Game(LaunchConfig{})
	if slices.Contains(g.Profiles, agent.ProfileCustom) {
		t.Error("custom must not be selectable from the console")
	}
}

// The two builds share a directory but not a file, so "installed" has to be
// asked per profile — otherwise switching to Wine on a Linux install looks
// ready and fails at exec.
func TestInstalledIsPerBuild(t *testing.T) {
	dir := t.TempDir()
	native, wine := profileFor(ProfileNative), profileFor(ProfileWine)
	// Rebuild against the temp dir so the probes point somewhere real.
	native = buildProfile(ProfileNative, LaunchConfig{}, dir, nil)
	wine = buildProfile(ProfileWine, LaunchConfig{}, dir, nil)

	if installedIn(native, dir) || installedIn(wine, dir) {
		t.Fatal("an empty directory has neither build installed")
	}
	if err := os.WriteFile(filepath.Join(dir, defaultNativeScript), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !installedIn(native, dir) {
		t.Error("the native build should be installed once its launcher exists")
	}
	if installedIn(wine, dir) {
		t.Error("a Linux install must not count as a Windows one — they are different depots")
	}
}

// installedIn mirrors the kit's probe check for these assertions.
func installedIn(p agent.Profile, dir string) bool {
	_, err := os.Stat(filepath.Join(dir, p.Probe))
	return err == nil
}
