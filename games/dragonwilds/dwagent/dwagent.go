// Package dwagent is Dragonwilds' half of the sidecar agent: the
// agent.Game spec wkagent runs with. Like esagent, it deliberately does
// NOT import games/dragonwilds (the console-side game module) — an agent
// binary has no business linking a console registry. The Steam app id is
// duplicated here as data, with the agreement test in games/dragonwilds
// keeping the copies honest.
//
// Dragonwilds is the game the kit's launch chooser exists for: two
// builds from one app id, and choosing between them is not cosmetic. The
// native Linux build is simple and modless — UE4SS is Windows-only, so
// its command verbs stay 501 forever. The Windows build under Wine can
// load UE4SS and therefore dwbridge, which is the entire reason the
// chooser exists. Verified pieces (dragonwilds-recon, "Phase 4
// unblocked"): the Windows server runs headless under plain Wine, UE4SS
// injects through a version.dll shim, and WINEDLLOVERRIDES="version=n,b"
// is *required* — without it Wine prefers its builtin version.dll and
// the mod silently never loads.
package dwagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/sampo/core/agent"
	"github.com/safwyls/sampo/games/dragonwilds/dwbridge"
	"github.com/safwyls/sampo/games/dragonwilds/dwconfig"
)

// AppID is the Dragonwilds dedicated-server tool.
const AppID = 4019830

// DefaultGamePort is the first of the game's UDP pair — it binds this
// and the port above it.
const DefaultGamePort = 7777

const (
	// ProfileNative is the native Linux dedicated server. No mod support.
	ProfileNative = "native"
	// ProfileWine is the Windows dedicated server under Wine, the only
	// build UE4SS — and therefore dwbridge — can attach to.
	ProfileWine = "wine"
)

// Default locations inside the install root. Both are overridable
// because a game update could rename either.
const (
	defaultNativeScript = "RSDragonwildsServer.sh"
	defaultWindowsExe   = "RSDragonwilds/Binaries/Win64/RSDragonwildsServer-Win64-Shipping.exe"
)

// Where each build keeps DedicatedServer.ini, relative to the install
// root. UE names the directory after the platform, so the Windows build
// under Wine writes WindowsServer/ — pointing the ini editor at
// LinuxServer/ for a Wine server would edit a file the game never reads.
var (
	linuxConfigRel   = filepath.Join("RSDragonwilds", "Saved", "Config", "LinuxServer", "DedicatedServer.ini")
	windowsConfigRel = filepath.Join("RSDragonwilds", "Saved", "Config", "WindowsServer", "DedicatedServer.ini")
)

// LaunchConfig is the per-profile tuning the environment can supply.
type LaunchConfig struct {
	// WineBin is the wine executable (default "wine").
	WineBin string
	// WinePrefix is WINEPREFIX for the game. Empty leaves Wine's default
	// (~/.wine), fine for a single-purpose container.
	WinePrefix string
	// GameExe overrides the Windows server exe path, relative to install.
	GameExe string
	// NativeScript overrides the Linux launcher, relative to install.
	NativeScript string
	// Profile is the initial selection for an install that has never
	// been told otherwise — the persisted choice wins, so redeploying
	// the container doesn't silently change which build runs.
	Profile string
	// BridgeKitDir is where a UE4SS+dwbridge kit ships inside this image,
	// ready to copy next to the server exe. Empty (the plain image) means
	// /v1/bridge/install honestly answers "no kit here".
	BridgeKitDir string
}

// winePath converts a Linux absolute path to the Windows path the game
// sees. Wine maps Z: to /, so /dragonwilds/dwbridge is
// Z:\dragonwilds\dwbridge. The mod reads DWBRIDGE_DIR with Windows
// semantics, so handing it a Linux path is the difference between a
// working bridge and a silently idle one.
func winePath(p string) string {
	return "Z:" + strings.ReplaceAll(p, "/", `\`)
}

// Game assembles Dragonwilds' agent.Game spec.
func Game(cfg LaunchConfig) agent.Game {
	return agent.Game{
		AgentName:         "wkagent",
		AppID:             AppID,
		DefaultGamePort:   DefaultGamePort,
		ConfigRelPath:     linuxConfigRel,
		ConfigContentType: "text/plain; charset=utf-8",
		// No upload validator: the ini grammar is the game's own and a
		// rejected server never regenerates a permissive default the way
		// Enshrouded does — the never-add write policy in dwconfig is the
		// guard that matters.
		SaveDirName: filepath.Join("RSDragonwilds", "Saved", "SaveGames"),
		// The save dir needs discovery: two spellings exist in the wild.
		FindSaveDir: findSaveDir,
		Profiles:    []string{ProfileNative, ProfileWine},
		// Native is what this agent has always run; Wine is opt-in.
		DefaultProfile: defaultProfile(cfg.Profile),
		// The game takes its port on the command line.
		DefaultArgs: func(gamePort int) []string {
			return []string{"-log", fmt.Sprintf("-Port=%d", gamePort)}
		},
		BuildProfile: func(name, installDir string, gamePort int, args []string) agent.Profile {
			return buildProfile(name, cfg, installDir, args)
		},
		PrepareRuntime: prepareRuntime,
		HealthExtras: func(a *agent.Agent) map[string]any {
			if st := dwbridge.New(a.InstallDir()).Status(); st != nil {
				return map[string]any{"bridge": st}
			}
			return nil
		},
		LaunchExtras: func(a *agent.Agent) map[string]any {
			return map[string]any{
				"bridgeKit":       bridgeKitPresent(cfg.BridgeKitDir),
				"bridgeInstalled": bridgeInstalled(a.InstallDir()),
			}
		},
		Routes: func(r chi.Router, a *agent.Agent) {
			mountBridgeRoutes(r, a, cfg.BridgeKitDir)
		},
	}
}

func defaultProfile(configured string) string {
	if configured == ProfileWine {
		return ProfileWine
	}
	return ProfileNative
}

// findSaveDir locates the world save directory, globbing because two
// spellings exist in the wild and a fresh install legitimately has none.
func findSaveDir(installDir string) (string, error) {
	for _, dir := range []string{"SaveGames", "Savegames"} {
		full := filepath.Join(installDir, "RSDragonwilds", "Saved", dir)
		matches, err := filepath.Glob(filepath.Join(full, "*.sav"))
		if err == nil && len(matches) > 0 {
			return full, nil
		}
	}
	return "", errors.New("no world save found under the install dir (has the server run yet?)")
}

// buildProfile assembles the named build.
func buildProfile(name string, cfg LaunchConfig, installDir string, args []string) agent.Profile {
	switch name {
	case ProfileWine:
		exe := cfg.GameExe
		if exe == "" {
			exe = defaultWindowsExe
		}
		wine := cfg.WineBin
		if wine == "" {
			wine = "wine"
		}
		env := []string{
			// Required, not optional: without it Wine loads its builtin
			// version.dll and the UE4SS shim beside the exe is ignored.
			"WINEDLLOVERRIDES=version=n,b",
			// The mod finds the shared directory here, in Windows form.
			"DWBRIDGE_DIR=" + winePath(filepath.Join(installDir, dwbridge.DirName)),
			// A headless server has no use for Wine's chatter, and it
			// would otherwise interleave with the game log the player
			// list is parsed from.
			"WINEDEBUG=-all",
		}
		if cfg.WinePrefix != "" {
			env = append(env, "WINEPREFIX="+cfg.WinePrefix)
		}
		return agent.Profile{
			Name: ProfileWine, Label: "Windows build under Wine (mods)",
			Command: wine,
			// The exe is passed absolute so the working directory is
			// free to be the one the feasibility run used.
			Args:          append([]string{filepath.Join(installDir, exe)}, args...),
			Env:           env,
			Dir:           filepath.Dir(exe),
			Probe:         exe,
			ConfigRel:     windowsConfigRel,
			SteamPlatform: "windows",
			Mods:          true,
		}
	default:
		script := cfg.NativeScript
		if script == "" {
			script = defaultNativeScript
		}
		return agent.Profile{
			Name: ProfileNative, Label: "Native Linux build",
			Command:   "./" + strings.TrimPrefix(script, "./"),
			Args:      args,
			Probe:     script,
			ConfigRel: linuxConfigRel,
		}
	}
}

// prepareRuntime seeds or enforces DedicatedServer.ini, stands the
// bridge directory up for mod-capable profiles, and links steamclient.so
// where the game looks for it.
func prepareRuntime(env agent.RuntimeEnv) {
	ini := env.ConfigPath
	if _, err := os.Stat(ini); err != nil {
		seedConfig(env)
	} else {
		enforce := map[string]string{}
		if env.Identity.ServerName != "" {
			enforce["ServerName"] = env.Identity.ServerName
		}
		if env.Identity.AdminPassword != "" {
			enforce["AdminPassword"] = env.Identity.AdminPassword
		}
		if env.Identity.OwnerID != "" {
			enforce["OwnerId"] = env.Identity.OwnerID
		}
		if len(enforce) > 0 {
			// Never-add policy: a key the game hasn't written yet is a
			// warn, not an append — inventing keys risks an unbootable ini.
			if err := dwconfig.Write(ini, enforce); err != nil {
				env.Logger.Warn("could not enforce identity settings", "error", err)
			}
		}
	}

	// The bridge directory is the file-IPC rendezvous with the dwbridge
	// mod (DWBRIDGE_DIR). Neither side creates it: the mod's io.open
	// fails silently without it, and the agent reads a missing dir as
	// "no bridge expected" — so a modded profile without this mkdir has
	// a mod that heartbeats into the void (found the hard way on the
	// first real Wine deployment). Only for mod-capable profiles, so the
	// missing-dir semantics stay meaningful on the native build.
	if env.Profile.Mods {
		if err := os.MkdirAll(filepath.Join(env.InstallDir, dwbridge.DirName), 0o755); err != nil {
			env.Logger.Warn("could not create the bridge directory", "error", err)
		}
	}

	linkSteamClient(env)
}

// linkSteamClient links ~/.steam/sdk64/steamclient.so to SteamCMD's
// copy. Best-effort: absence only matters to Steam-networking features,
// and the game logs it loudly itself.
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

// seedConfig writes a minimal DedicatedServer.ini so a freshly installed
// server can reach its first successful start.
//
// Without an owner id there is nothing worth writing — the game rejects
// a config that has none, so a seed would only replace one unbootable
// state with another. That case logs the reason instead, because
// "installed but won't start" is otherwise a silent, confusing outcome.
func seedConfig(env agent.RuntimeEnv) {
	ini, id := env.ConfigPath, env.Identity
	if id.OwnerID == "" {
		env.Logger.Warn("no owner id configured: the game will write its own DedicatedServer.ini and refuse to start until OwnerId is filled in",
			"path", ini, "fix", "set WKAGENT_OWNER_ID (in-game: Settings, bottom-left 'My Player ID')")
		return
	}
	if err := os.MkdirAll(filepath.Dir(ini), 0o755); err != nil {
		env.Logger.Warn("could not create the config directory", "error", err)
		return
	}
	var b strings.Builder
	b.WriteString("; Seeded by wkagent on first install. The game owns this file from\n")
	b.WriteString("; here — edit it from the dashboard's Configuration view.\n")
	b.WriteString("[/Script/Dominion.DedicatedServerSettings]\n")
	b.WriteString("OwnerId=" + id.OwnerID + "\n")
	if id.ServerName != "" {
		b.WriteString("ServerName=" + id.ServerName + "\n")
	}
	if id.AdminPassword != "" {
		b.WriteString("AdminPassword=" + id.AdminPassword + "\n")
	}
	if id.WorldName != "" {
		b.WriteString("DefaultWorldName=" + id.WorldName + "\n")
	}
	if err := os.WriteFile(ini, []byte(b.String()), 0o644); err != nil {
		env.Logger.Warn("could not seed DedicatedServer.ini", "error", err)
		return
	}
	env.Logger.Info("seeded DedicatedServer.ini", "path", ini)
}

// gameBinDir is where the mod kit lands: the directory holding the
// Windows server exe, since UE4SS injects from beside the binary it
// hooks.
func gameBinDir(installDir string) string {
	return filepath.Join(installDir, filepath.FromSlash(filepath.Dir(defaultWindowsExe)))
}

// bridgeModDirName is the directory whose presence means "a UE4SS
// install exists here".
const bridgeModDirName = "ue4ss"

func bridgeKitPresent(kitDir string) bool {
	if kitDir == "" {
		return false
	}
	fi, err := os.Stat(kitDir)
	return err == nil && fi.IsDir()
}

// bridgeInstalled reports whether a UE4SS install already sits next to
// the exe. Version-blind on purpose: whatever is there is the operator's.
func bridgeInstalled(installDir string) bool {
	fi, err := os.Stat(filepath.Join(gameBinDir(installDir), bridgeModDirName))
	return err == nil && fi.IsDir()
}

// mountBridgeRoutes contributes the dwbridge verbs under /v1.
func mountBridgeRoutes(r chi.Router, a *agent.Agent, kitDir string) {
	channel := func() *dwbridge.Bridge { return dwbridge.New(a.InstallDir()) }

	r.Post("/bridge/command", func(w http.ResponseWriter, req *http.Request) {
		if !supervised(a) {
			agent.WriteError(w, http.StatusBadRequest, "agent is not supervising a game — the dwbridge channel needs supervisor mode")
			return
		}
		var in struct {
			Command string            `json:"command"`
			Args    map[string]string `json:"args"`
		}
		if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
			agent.WriteError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if in.Command == "" {
			agent.WriteError(w, http.StatusBadRequest, "command is required")
			return
		}
		data, err := channel().Command(req.Context(), in.Command, in.Args)
		switch {
		case errors.Is(err, dwbridge.ErrUnavailable):
			agent.WriteError(w, http.StatusServiceUnavailable, err.Error())
			return
		case err != nil:
			agent.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		agent.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
	})

	r.Get("/bridge/state", func(w http.ResponseWriter, req *http.Request) {
		if !supervised(a) {
			agent.WriteError(w, http.StatusBadRequest, "agent is not supervising a game — the dwbridge channel needs supervisor mode")
			return
		}
		agent.WriteJSON(w, http.StatusOK, channel().State())
	})

	// One-click UE4SS+dwbridge install next to the exe (bridgekit).
	// Deliberately an explicit operator act with only-when-absent
	// semantics: an existing ue4ss/ directory, whatever its version, is
	// the operator's and is never overwritten.
	r.Post("/bridge/install", func(w http.ResponseWriter, req *http.Request) {
		p, ok := a.SupervisedProfile()
		if !ok {
			agent.WriteError(w, http.StatusBadRequest, "agent is not supervising a game — mod install is supervisor mode only")
			return
		}
		if !bridgeKitPresent(kitDir) {
			agent.WriteError(w, http.StatusNotImplemented,
				"this agent's image carries no mod kit — mods need the Wine image (wkagent:*-wine)")
			return
		}
		if !p.Mods {
			agent.WriteError(w, http.StatusBadRequest,
				"the selected build cannot load mods — switch to the Windows build first")
			return
		}
		if _, err := os.Stat(filepath.Join(a.InstallDir(), p.Probe)); err != nil {
			agent.WriteError(w, http.StatusBadRequest,
				"the Windows build is not installed yet — run an update first, so the kit has an exe to sit beside")
			return
		}
		if bridgeInstalled(a.InstallDir()) {
			agent.WriteError(w, http.StatusConflict,
				"a ue4ss/ directory already exists next to the exe — not overwriting it; remove it first if you want this kit")
			return
		}
		if err := copyTree(kitDir, gameBinDir(a.InstallDir())); err != nil {
			agent.WriteError(w, http.StatusInternalServerError, "copying the kit: "+err.Error())
			return
		}
		// The file-IPC rendezvous, so the mod has somewhere to heartbeat
		// the moment it loads — same mkdir prepareRuntime does at start.
		if err := os.MkdirAll(filepath.Join(a.InstallDir(), dwbridge.DirName), 0o755); err != nil {
			a.LoggerHandle().Warn("could not create the bridge directory", "error", err)
		}
		a.LoggerHandle().Info("bridge kit installed", "from", kitDir, "to", gameBinDir(a.InstallDir()))
		agent.WriteJSON(w, http.StatusOK, map[string]any{
			"installed":       true,
			"restartRequired": a.SupervisedRunning(),
		})
	})
}

func supervised(a *agent.Agent) bool {
	_, ok := a.SupervisedProfile()
	return ok
}

// copyTree copies src into dst (which must exist), merging directories
// and never following symlinks — the kit is plain files and directories.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
