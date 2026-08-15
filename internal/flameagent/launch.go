package flameagent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Launch profiles: how the agent starts the game.
//
// Enshrouded ships exactly one dedicated-server build — Windows,
// enshrouded_server.exe — and no native Linux binary exists
// (docs/enshrouded-recon.md, "Steam app id & platform"). On Linux it runs
// under a compatibility layer; plain Wine is what the bare-metal guides
// use and what this agent's image carries, with the binary overridable
// for hosts that prefer a Proton entrypoint. The profile machinery is
// inherited from the sibling consoles, where a game with two builds needs
// the choice to be explicit and persistent; here it carries the one wine
// profile plus the custom escape hatch, and stays because a second build
// (a native server at 1.0?) would slot in as a new profile rather than a
// redesign.

const (
	// ProfileWine is the Windows dedicated server under Wine — the only
	// build the game has.
	ProfileWine = "wine"
	// ProfileCustom is what a hand-configured GAME_CMD/GAME_ARGS becomes.
	// It is not selectable from the console: the operator has already said
	// exactly what to run, and silently replacing that would be rude.
	ProfileCustom = "custom"
)

// defaultWindowsExe is the server binary inside the install root. SteamCMD
// installs it flat — no platform-suffixed directories here, unlike UE
// games.
const defaultWindowsExe = "enshrouded_server.exe"

// configRelPath is where enshrouded_server.json lives: next to the exe,
// at the install root. The server generates it with defaults on first
// start when absent; the supervisor seeds it earlier so the first start
// is already named and password-protected.
const configRelPath = "enshrouded_server.json"

// Profile is one way of starting the game, with everything that differs
// between launch methods gathered in a single place.
type Profile struct {
	// Name is the stable id: wine | custom.
	Name string `json:"name"`
	// Label is how the console names it.
	Label string `json:"label"`
	// Command is the executable. A bare name (no separator) is looked up
	// on PATH — that's how "wine" works; anything else resolves inside the
	// install dir.
	Command string   `json:"command"`
	Args    []string `json:"args"`
	// Env is added to the agent's own environment for the game process.
	Env []string `json:"-"`
	// Dir is the working directory, relative to the install root. Empty
	// means the install root itself — which matters here: the server
	// resolves saveDirectory/logDirectory ("./savegame", "./logs")
	// against its working directory.
	Dir string `json:"-"`
	// Probe is the file whose presence means "the game is installed",
	// relative to the install root.
	Probe string `json:"-"`
	// ConfigRel is where this profile's enshrouded_server.json lives,
	// relative to the install root.
	ConfigRel string `json:"configPath"`
	// SteamPlatform is the depot to install; Enshrouded is Windows-only,
	// so every install forces the windows platform type.
	SteamPlatform string `json:"steamPlatform,omitempty"`
}

// LaunchConfig is the per-profile tuning the environment can supply.
type LaunchConfig struct {
	// Profile is the selected profile name; empty means wine.
	Profile string
	// WineBin is the wine executable (default "wine64", falling back to
	// "wine" when only that exists on PATH).
	WineBin string
	// WinePrefix is WINEPREFIX for the game. Empty defaults to
	// <install>/.wineprefix so the prefix lives in the install volume and
	// survives agent-container recreation — a fresh prefix costs a slow
	// first boot, not data, but there is no reason to pay it per deploy.
	WinePrefix string
	// GameExe overrides the server exe path, relative to install.
	GameExe string
}

// resolveWineBin picks the wine binary when none is configured: wine64
// where it exists (the bare-metal guides' choice), plain wine otherwise
// (some distributions ship only the unified binary).
func resolveWineBin(configured string) string {
	if configured != "" {
		return configured
	}
	if _, err := exec.LookPath("wine64"); err == nil {
		return "wine64"
	}
	return "wine"
}

// buildProfile assembles the profile for name, given where the game
// lives.
//
// gameArgs, when set, is appended to the exe — an operator override for
// flags this agent doesn't know about. The port is deliberately not a
// launch argument: Enshrouded reads queryPort from
// enshrouded_server.json, which the supervisor enforces before each
// start.
func buildProfile(name string, cfg LaunchConfig, installDir string, gameCommand string, gameArgs []string) Profile {
	// An explicit command is the operator saying exactly what to run.
	// Honour it verbatim under its own name rather than pretending it is
	// one of the known profiles.
	if gameCommand != "" {
		return Profile{
			Name: ProfileCustom, Label: "Custom command",
			Command: gameCommand, Args: gameArgs,
			Probe: gameCommand, ConfigRel: configRelPath,
		}
	}

	exe := cfg.GameExe
	if exe == "" {
		exe = defaultWindowsExe
	}
	prefix := cfg.WinePrefix
	if prefix == "" {
		prefix = filepath.Join(installDir, ".wineprefix")
	}
	env := []string{
		// A headless server has no use for Wine's chatter, and it would
		// otherwise interleave with the game log the player list is parsed
		// from.
		"WINEDEBUG=-all",
		"WINEPREFIX=" + prefix,
	}
	return Profile{
		Name: ProfileWine, Label: "Windows build under Wine",
		Command: resolveWineBin(cfg.WineBin),
		// The exe is passed absolute so the working directory stays free
		// to be the install root, where the server's relative save/log
		// directories belong.
		Args:          append([]string{filepath.Join(installDir, exe)}, gameArgs...),
		Env:           env,
		Dir:           "",
		Probe:         exe,
		ConfigRel:     configRelPath,
		SteamPlatform: "windows",
	}
}

// resolveCommand turns a profile's Command into something exec can run: a
// bare name goes to PATH, anything else is relative to the install dir.
func (p Profile) resolveCommand(installDir string) string {
	if !strings.ContainsAny(p.Command, `/\`) {
		return p.Command
	}
	if filepath.IsAbs(p.Command) {
		return p.Command
	}
	return filepath.Join(installDir, p.Command)
}

// installed reports whether the game's files are present.
func (p Profile) installed(installDir string) bool {
	_, err := os.Stat(filepath.Join(installDir, p.Probe))
	return err == nil
}

// SelectableProfiles are the profiles the console may switch between.
// Custom is deliberately absent — see ProfileCustom.
var SelectableProfiles = []string{ProfileWine}

// validProfile reports whether name is one the console may select.
func validProfile(name string) bool {
	for _, p := range SelectableProfiles {
		if p == name {
			return true
		}
	}
	return false
}

// runnable reports whether the profile's command can actually be executed
// here — whether this image has Wine in it at all. Not the same question
// as "is the game installed": answering it up front lets the console
// explain the real fix instead of showing a start button that cannot
// work.
func (p Profile) runnable(installDir string) bool {
	if !strings.ContainsAny(p.Command, `/\`) {
		_, err := exec.LookPath(p.Command)
		return err == nil
	}
	_, err := os.Stat(p.resolveCommand(installDir))
	return err == nil
}
