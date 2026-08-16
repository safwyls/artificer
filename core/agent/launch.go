package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// How the agent starts the game: the game module assembles the real
// launch profile via Game.BuildProfile — a wine wrapper, a native
// script, whatever the game ships — and the kit adds the custom escape
// hatch for an operator who has already said exactly what to run.

// ProfileCustom is what a hand-configured GAME_CMD/GAME_ARGS becomes:
// the operator has already said exactly what to run, and the agent
// honours it verbatim rather than second-guessing it.
const ProfileCustom = "custom"

// Profile is one way of starting the game, with everything that differs
// between launch methods gathered in a single place.
type Profile struct {
	// Name is the stable id ("wine", "native", "custom").
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
	// ConfigRel is where this profile's settings file lives, relative to
	// the install root.
	ConfigRel string `json:"configPath"`
	// SteamPlatform is the depot platform to force on install ("windows"
	// for games run under Wine); empty means the host's. Switching to a
	// profile with a different platform means re-installing the game,
	// which the console has to say out loud.
	SteamPlatform string `json:"steamPlatform,omitempty"`
	// Mods reports whether this profile can carry a command mod (the
	// reason a chooser exists at all for Dragonwilds).
	Mods bool `json:"mods"`
}

// buildProfile assembles the named profile: an explicit command is the
// operator saying exactly what to run — honoured verbatim under its own
// name — else the game's hook builds it. Args resolve here so custom
// commands and game builds get the same defaulting.
func buildProfile(g Game, name, installDir string, gamePort int, gameCommand string, gameArgs []string) Profile {
	args := gameArgs
	if len(args) == 0 && g.DefaultArgs != nil {
		args = g.DefaultArgs(gamePort)
	}
	if gameCommand != "" {
		return Profile{
			Name: ProfileCustom, Label: "Custom command",
			Command: gameCommand, Args: args,
			Probe: gameCommand, ConfigRel: g.ConfigRelPath,
		}
	}
	return g.BuildProfile(name, installDir, gamePort, args)
}

// validProfile reports whether name is one the console may select.
func (g Game) validProfile(name string) bool {
	for _, p := range g.Profiles {
		if p == name {
			return true
		}
	}
	return false
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
