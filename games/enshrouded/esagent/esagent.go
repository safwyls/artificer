// Package esagent is Enshrouded's half of the sidecar agent: the
// agent.Game spec flameagent runs with. Everything here used to live
// inside flameagent itself; the kit (core/agent) now owns the shared
// machinery and this package owns the game.
//
// Deliberately does NOT import games/enshrouded (the console-side game
// module): that package registers into the game registry, and an agent
// binary has no business linking a console registry for one integer.
// The Steam app id is duplicated here as data, and the agreement test in
// games/enshrouded keeps the two honest — the same design flameagent
// documented.
package esagent

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/safwyls/sampo/core/agent"
	"github.com/safwyls/sampo/games/enshrouded/esconfig"
	"github.com/safwyls/sampo/games/enshrouded/esquery"
)

// AppID is the Enshrouded dedicated-server tool (the game client is
// 1203620) — see the package comment for why it is spelled here.
const AppID = 2278520

// DefaultQueryPort is Enshrouded's single UDP port: game traffic and the
// Steam A2S query share it.
const DefaultQueryPort = 15637

// defaultWindowsExe is the server binary inside the install root.
// SteamCMD installs it flat — no platform-suffixed directories.
const defaultWindowsExe = "enshrouded_server.exe"

// WineConfig is the launch tuning the environment can supply. Enshrouded
// ships exactly one dedicated-server build — Windows — so on Linux it
// runs under Wine and there is nothing to choose, only to tune.
type WineConfig struct {
	// WineBin is the wine executable (default "wine64", falling back to
	// "wine" when only that exists on PATH).
	WineBin string
	// WinePrefix is WINEPREFIX for the game. Empty defaults to
	// <install>/.wineprefix so the prefix lives in the install volume and
	// survives agent-container recreation.
	WinePrefix string
	// GameExe overrides the server exe path, relative to install.
	GameExe string
}

// Game assembles Enshrouded's agent.Game spec.
func Game(wine WineConfig) agent.Game {
	return agent.Game{
		AgentName:         "flameagent",
		AppID:             AppID,
		DefaultGamePort:   DefaultQueryPort,
		ConfigRelPath:     "enshrouded_server.json",
		ConfigContentType: "application/json; charset=utf-8",
		// A malformed json makes the server regenerate defaults — an
		// *open, password-less* server — far worse than a rejected edit.
		ValidateConfig: esconfig.Validate,
		SaveDirName:    "savegame",
		// SIGINT is Enshrouded's clean-shutdown signal, on which it saves
		// the world; a SIGTERM to the wine wrapper is not reliably
		// propagated. The game writes the world on the way down and
		// community images budget 60-90s for it, so the grace is generous.
		StopSignal: syscall.SIGINT,
		StopGrace:  120 * time.Second,
		BuildProfile: func(installDir string, gameArgs []string) agent.Profile {
			return wineProfile(wine, installDir, gameArgs)
		},
		PrepareRuntime: prepareRuntime,
		Routes:         routes,
	}
}

// resolveWineBin picks the wine binary when none is configured: wine64
// where it exists (the bare-metal guides' choice), plain wine otherwise.
func resolveWineBin(configured string) string {
	if configured != "" {
		return configured
	}
	if _, err := exec.LookPath("wine64"); err == nil {
		return "wine64"
	}
	return "wine"
}

// wineProfile assembles the one real launch profile. The port is
// deliberately not a launch argument: Enshrouded reads queryPort from
// enshrouded_server.json, which prepareRuntime enforces before each
// start.
func wineProfile(cfg WineConfig, installDir string, gameArgs []string) agent.Profile {
	exe := cfg.GameExe
	if exe == "" {
		exe = defaultWindowsExe
	}
	prefix := cfg.WinePrefix
	if prefix == "" {
		prefix = filepath.Join(installDir, ".wineprefix")
	}
	return agent.Profile{
		Name: "wine", Label: "Windows build under Wine",
		Command: resolveWineBin(cfg.WineBin),
		// The exe is passed absolute so the working directory stays free
		// to be the install root, where the server's relative save/log
		// directories belong.
		Args: append([]string{filepath.Join(installDir, exe)}, gameArgs...),
		Env: []string{
			// A headless server has no use for Wine's chatter, and it
			// would otherwise interleave with the game log the player
			// list is parsed from.
			"WINEDEBUG=-all",
			"WINEPREFIX=" + prefix,
		},
		Dir:           "",
		Probe:         exe,
		ConfigRel:     "enshrouded_server.json",
		SteamPlatform: "windows",
	}
}

// prepareRuntime makes a freshly installed server bootable, then keeps
// the operator's identity settings authoritative.
//
// When enshrouded_server.json is absent, the game would generate one
// with defaults on first start — but that default is an *open* server
// named "Enshrouded Server", so the seed writes a complete config first
// and the first start is already named and password-protected. When the
// file exists it belongs to the operator (or the game), and only the
// explicitly configured identity settings are enforced into it.
func prepareRuntime(cfgPath string, id agent.RuntimeIdentity) {
	e := esconfig.Enforcement{ServerName: id.ServerName, QueryPort: id.GamePort}
	if id.AdminPassword != "" {
		e.AdminPassword = &id.AdminPassword
	}
	if id.JoinPassword != "" {
		e.JoinPassword = &id.JoinPassword
	}

	if _, err := os.Stat(cfgPath); err != nil {
		_ = esconfig.Write(cfgPath, esconfig.Seed(e))
		return
	}
	doc, err := esconfig.Load(cfgPath)
	if err != nil {
		// A file the game can't parse either would be regenerated with
		// open defaults on boot; refuse to make it worse.
		return
	}
	if esconfig.Enforce(doc, e) {
		_ = esconfig.Write(cfgPath, doc)
	}
}

// queryTimeout bounds the whole query handler: the console calls this on
// its info path, so a server that has bound its port but isn't answering
// yet must fail fast rather than hold a dashboard request open.
const queryTimeout = 3 * time.Second

// routes mounts the game's own Steam query, run from inside the
// container — the only presence signal that isn't log inference. It goes
// to the loopback address, the one place it is known to work regardless
// of how the deployment NATs the published port.
func routes(r chi.Router, a *agent.Agent) {
	r.Get("/query", func(w http.ResponseWriter, req *http.Request) {
		st, supervised := a.SupervisedStatus()
		if !supervised {
			agent.WriteError(w, http.StatusBadRequest, "agent is in companion mode — it does not know the game's query port")
			return
		}
		if st.State != "running" {
			agent.WriteError(w, http.StatusServiceUnavailable, "the game process is "+st.State)
			return
		}

		ctx, cancel := context.WithTimeout(req.Context(), queryTimeout)
		defer cancel()
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(a.SupervisedGamePort()))

		info, err := esquery.QueryInfo(ctx, addr)
		if err != nil {
			agent.WriteError(w, http.StatusServiceUnavailable, "the game did not answer the Steam query: "+err.Error())
			return
		}
		// Players are a second round trip and a lesser prize — the count
		// is already in the info reply, and A2S player rows carry no
		// account id. A failure here costs the names, not the answer.
		players, perr := esquery.QueryPlayers(ctx, addr)
		res := map[string]any{"info": info, "players": players}
		if perr != nil {
			res["playersError"] = perr.Error()
		}
		agent.WriteJSON(w, http.StatusOK, res)
	})
}
