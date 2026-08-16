package flameagent

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func profileFor(name string) Profile {
	return buildProfile(name, LaunchConfig{WineBin: "wine64"}, "/enshrouded", "", nil)
}

// Everything in this test fails silently if it's wrong: the game starts
// under some default and simply behaves differently.
func TestWineProfileRunsTheWindowsServer(t *testing.T) {
	p := profileFor(ProfileWine)

	if p.Command != "wine64" {
		t.Errorf("command = %q, want the configured wine binary", p.Command)
	}
	// A bare name must stay bare so exec finds it on PATH; joining it to
	// the install dir would look for /enshrouded/wine64.
	if got := p.resolveCommand("/enshrouded"); got != "wine64" {
		t.Errorf("resolved = %q, want a PATH lookup", got)
	}
	if len(p.Args) == 0 || !strings.HasSuffix(p.Args[0], "enshrouded_server.exe") {
		t.Fatalf("args = %v, want the server exe first", p.Args)
	}
	if !filepath.IsAbs(p.Args[0]) {
		t.Errorf("exe %q should be absolute, so the working directory is free", p.Args[0])
	}
	// The server resolves ./savegame and ./logs against its working
	// directory; anywhere but the install root scatters the world.
	if p.Dir != "" {
		t.Errorf("dir = %q, want the install root", p.Dir)
	}

	env := strings.Join(p.Env, " ")
	// Wine's chatter would interleave with the game log the player list
	// is parsed from.
	if !strings.Contains(env, "WINEDEBUG=-all") {
		t.Errorf("env = %v, missing WINEDEBUG=-all", p.Env)
	}
	// The prefix defaults into the install volume so it survives
	// agent-container recreation.
	if !strings.Contains(env, "WINEPREFIX=/enshrouded/.wineprefix") {
		t.Errorf("env = %v, want the prefix inside the install volume", p.Env)
	}
	if p.ConfigRel != "enshrouded_server.json" {
		t.Errorf("config path = %q, want the json at the install root", p.ConfigRel)
	}
	// Enshrouded has no Linux depot: installing the host's own platform
	// would fetch nothing runnable.
	if p.SteamPlatform != "windows" {
		t.Errorf("steam platform = %q, want windows", p.SteamPlatform)
	}
}

func TestWinePrefixOverrideWins(t *testing.T) {
	set := buildProfile(ProfileWine, LaunchConfig{WineBin: "wine64", WinePrefix: "/data/wine"}, "/g", "", nil)
	if !slices.Contains(set.Env, "WINEPREFIX=/data/wine") {
		t.Errorf("env = %v, want the configured prefix", set.Env)
	}
}

// The port is deliberately not a launch argument — Enshrouded reads
// queryPort from its json, which the supervisor enforces instead. A stray
// port flag would be silently ignored at best.
func TestLaunchArgsCarryNoPort(t *testing.T) {
	p := profileFor(ProfileWine)
	for _, a := range p.Args[1:] {
		if strings.Contains(strings.ToLower(a), "port") {
			t.Errorf("args = %v carry a port flag; the json owns the port", p.Args)
		}
	}
}

// An explicit game command is the operator having already decided. The
// wine profile must not quietly override it.
func TestExplicitCommandIsHonouredVerbatim(t *testing.T) {
	p := buildProfile(ProfileWine, LaunchConfig{}, "/g", "./my-launcher.sh", []string{"-x"})

	if p.Name != ProfileCustom {
		t.Errorf("name = %q, want custom", p.Name)
	}
	if p.Command != "./my-launcher.sh" || !slices.Contains(p.Args, "-x") {
		t.Errorf("the operator's command was not honoured verbatim: %q %v", p.Command, p.Args)
	}
}

func TestInstalledProbesTheServerExe(t *testing.T) {
	dir := t.TempDir()
	p := buildProfile(ProfileWine, LaunchConfig{WineBin: "wine64"}, dir, "", nil)

	if p.installed(dir) {
		t.Fatal("an empty directory has no game installed")
	}
	if err := os.WriteFile(filepath.Join(dir, defaultWindowsExe), []byte("MZ"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !p.installed(dir) {
		t.Error("the game should count as installed once its exe exists")
	}
}
