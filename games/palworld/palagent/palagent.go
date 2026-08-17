// Package palagent is Palworld's half of the sidecar agent: the
// agent.Game spec the palagent binary runs with. Everything here used to
// live inside palcon's own agent; the kit (core/agent) now owns the
// shared machinery and this package owns the game.
//
// Deliberately does NOT import games/palworld (the console-side game
// module): that package registers into the game registry, and an agent
// binary has no business linking a console registry for one integer.
// The Steam app id is duplicated here as data, and the agreement test in
// games/palworld keeps the two honest — the same design the other agents
// follow.
package palagent

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/safwyls/sampo/core/agent"
	"github.com/safwyls/sampo/games/palworld/palconfig"
)

// AppID is the Palworld dedicated server (the game client is 1623730) —
// see the package comment for why it is spelled here.
const AppID = 2394010

// DefaultGamePort is Palworld's game UDP port. The REST and RCON admin
// ports (8212, 25575) are TCP siblings the console reaches directly; the
// agent doesn't relay them.
const DefaultGamePort = 8211

// configRelPath is where the Linux dedicated server keeps its settings.
const configRelPath = "Pal/Saved/Config/LinuxServer/PalWorldSettings.ini"

// defaultScript is the launcher SteamCMD installs at the install root.
const defaultScript = "PalServer.sh"

// Game assembles Palworld's agent.Game spec.
func Game() agent.Game {
	return agent.Game{
		AgentName:         "palagent",
		AppID:             AppID,
		DefaultGamePort:   DefaultGamePort,
		ConfigRelPath:     filepath.FromSlash(configRelPath),
		ConfigContentType: "text/plain; charset=utf-8",
		// No upload validator: the ini grammar is the game's own, and a
		// malformed file doesn't regenerate permissive defaults the way
		// Enshrouded's does — the game falls back to DefaultPalWorldSettings
		// values without touching passwords it was given at the REST/RCON
		// layer. palconfig's never-add write policy is the guard that
		// matters for dashboard edits.
		SaveDirName: filepath.Join("Pal", "Saved", "SaveGames"),
		// The world lives one level deeper than a fixed directory: the save
		// dir is the folder holding Level.sav under SaveGames/0/<world id>/,
		// so it needs discovery.
		FindSaveDir: findSaveDir,
		// One build exists, so name is ignored and no Profiles list is
		// declared — the kit offers no chooser. Palworld reads its port
		// from the ini/defaults, not the command line, so gamePort is
		// unused too.
		BuildProfile: func(_, _ string, _ int, args []string) agent.Profile {
			return agent.Profile{
				Name: "native", Label: "Native Linux build",
				Command:   "./" + defaultScript,
				Args:      args,
				Probe:     defaultScript,
				ConfigRel: filepath.FromSlash(configRelPath),
			}
		},
		// The flags every serious dedicated-server setup passes.
		DefaultArgs: func(int) []string {
			return []string{"-useperfthreads", "-NoAsyncLoadingThread", "-UseMultithreadForDS"}
		},
		PrepareRuntime: prepareRuntime,
	}
}

// findSaveDir locates the world save directory: the folder holding
// Level.sav under Pal/Saved/SaveGames/0/<world id>/. A fresh install (or
// one that hasn't booted yet) legitimately has none. With multiple worlds
// the most recently written Level.sav wins — that's the world the server
// is actually running.
func findSaveDir(installDir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(installDir, "Pal", "Saved", "SaveGames", "0", "*", "Level.sav"))
	if err != nil || len(matches) == 0 {
		return "", errors.New("no world save found under the install dir (has the server run yet?)")
	}
	best, bestMod := "", int64(-1)
	for _, m := range matches {
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if mod := info.ModTime().UnixNano(); mod > bestMod {
			best, bestMod = m, mod
		}
	}
	if best == "" {
		return "", errors.New("no readable world save found under the install dir")
	}
	return filepath.Dir(best), nil
}

// prepareRuntime covers what server images do before first launch: seed
// PalWorldSettings.ini from the game's shipped defaults, enforce the
// management interfaces, and put steamclient.so where the game looks for
// it.
func prepareRuntime(env agent.RuntimeEnv) {
	ini, id := env.ConfigPath, env.Identity
	if info, err := os.Stat(ini); err != nil || info.Size() == 0 {
		def := filepath.Join(env.InstallDir, "DefaultPalWorldSettings.ini")
		if data, err := os.ReadFile(def); err == nil {
			if os.MkdirAll(filepath.Dir(ini), 0o755) == nil {
				if os.WriteFile(ini, data, 0o644) == nil {
					env.Logger.Info("seeded PalWorldSettings.ini from defaults")
					// Identity seeds exactly once, here: the operator's
					// later settings-editor edits must stick.
					identity := map[string]string{}
					if id.ServerName != "" {
						identity["ServerName"] = id.ServerName
					}
					if id.ServerDesc != "" {
						identity["ServerDescription"] = id.ServerDesc
					}
					if len(identity) > 0 {
						if err := palconfig.Write(ini, identity); err != nil {
							env.Logger.Warn("could not seed server name/description", "error", err)
						}
					}
				}
			}
		}
	}

	// Palworld ships with REST and RCON disabled — a supervised server
	// left that way runs fine but is deaf to the dashboard. With an
	// admin password configured, manageability is enforced every start.
	if id.AdminPassword != "" {
		err := palconfig.Write(ini, map[string]string{
			"AdminPassword":  id.AdminPassword,
			"RCONEnabled":    "True",
			"RESTAPIEnabled": "True",
		})
		if err != nil {
			env.Logger.Warn("could not enforce management settings; the dashboard may not reach this server", "error", err)
		} else {
			env.Logger.Info("enforced RCON/REST enabled with the configured admin password")
		}
	}

	linkSteamClient(env)
}

// linkSteamClient links ~/.steam/sdk64/steamclient.so to SteamCMD's copy.
// Best-effort: absence only matters to Steam-networking features, and the
// game logs it loudly itself.
func linkSteamClient(env agent.RuntimeEnv) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	sdk := filepath.Join(home, ".steam", "sdk64", "steamclient.so")
	if _, err := os.Stat(sdk); err == nil {
		return
	}
	candidates, _ := filepath.Glob(filepath.Join(home, "*", "share", "Steam", "steamcmd", "linux64", "steamclient.so"))
	more, _ := filepath.Glob(filepath.Join(home, ".local", "share", "Steam", "steamcmd", "linux64", "steamclient.so"))
	candidates = append(candidates, more...)
	candidates = append(candidates, "/usr/lib/steamcmd/linux64/steamclient.so")
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			if os.MkdirAll(filepath.Dir(sdk), 0o755) == nil && os.Symlink(c, sdk) == nil {
				env.Logger.Info("linked steamclient.so", "from", c)
			}
			return
		}
	}
}
