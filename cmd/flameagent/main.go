// Command flameagent is the per-server sidecar agent: it sits next to one
// Enshrouded game server (or supervises it directly), holding the install
// volume and SteamCMD, and exposes a narrow authenticated API for
// flametender to drive. See docs/sidecar-agent.md for the design.
package main

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/safwyls/flametender/internal/flameagent"
)

// version is stamped by the release build via
// -ldflags "-X main.version=v0.x.y"; "dev" means a local build.
var version = "dev"

func main() {
	// Container healthcheck mode: probe our own /healthz and exit. The
	// runtime image (steamcmd base) ships neither wget nor curl, so the
	// binary is its own probe.
	if len(os.Args) > 1 && os.Args[1] == "-healthz" {
		addr := envOr("FLAMEAGENT_ADDR", ":8811")
		if strings.HasPrefix(addr, ":") {
			addr = "127.0.0.1" + addr
		}
		resp, err := http.Get("http://" + addr + "/healthz")
		if err != nil || resp.StatusCode != http.StatusNoContent {
			os.Exit(1)
		}
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	appID := 0
	if v := os.Getenv("FLAMEAGENT_APP_ID"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			logger.Error("invalid FLAMEAGENT_APP_ID", "value", v)
			os.Exit(1)
		}
		appID = n
	}

	var stopGrace time.Duration
	if v := os.Getenv("FLAMEAGENT_STOP_GRACE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			logger.Error("invalid FLAMEAGENT_STOP_GRACE", "value", v)
			os.Exit(1)
		}
		stopGrace = d
	}
	gamePort := 0
	if v := os.Getenv("FLAMEAGENT_GAME_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			logger.Error("invalid FLAMEAGENT_GAME_PORT", "value", v)
			os.Exit(1)
		}
		gamePort = n
	}

	var autostart *bool
	if v := os.Getenv("FLAMEAGENT_AUTOSTART"); v != "" {
		b := v == "true" || v == "1"
		autostart = &b
	}

	agent, err := flameagent.New(flameagent.Config{
		Token:       os.Getenv("FLAMEAGENT_TOKEN"),
		InstallDir:  envOr("FLAMEAGENT_INSTALL_DIR", "/enshrouded"),
		SteamCmd:    envOr("FLAMEAGENT_STEAMCMD", "steamcmd"),
		AppID:       appID,
		Mode:        envOr("FLAMEAGENT_MODE", "companion"),
		GameCommand: os.Getenv("FLAMEAGENT_GAME_CMD"),
		GameArgs:    strings.Fields(os.Getenv("FLAMEAGENT_GAME_ARGS")),
		Launch: flameagent.LaunchConfig{
			// The initial selection only applies to an install that has
			// never been told otherwise — the persisted choice wins, so
			// redeploying the container doesn't silently change how the
			// server runs.
			Profile:    envOr("FLAMEAGENT_LAUNCH_PROFILE", flameagent.ProfileWine),
			WineBin:    os.Getenv("FLAMEAGENT_WINE_BIN"),
			WinePrefix: os.Getenv("FLAMEAGENT_WINE_PREFIX"),
			GameExe:    os.Getenv("FLAMEAGENT_GAME_EXE"),
		},
		GamePort:      gamePort,
		StopGrace:     stopGrace,
		AdminPassword: os.Getenv("FLAMEAGENT_ADMIN_PASSWORD"),
		JoinPassword:  os.Getenv("FLAMEAGENT_JOIN_PASSWORD"),
		ServerName:    os.Getenv("FLAMEAGENT_SERVER_NAME"),
		Autostart:     autostart,
		Version:       version,
		Logger:        logger,
	})
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	// Supervisor boot: install if missing, then start per desired state.
	go agent.Run()

	addr := envOr("FLAMEAGENT_ADDR", ":8811")
	logger.Info("flameagent listening", "addr", addr, "version", version, "apiVersion", flameagent.APIVersion)
	if err := http.ListenAndServe(addr, agent.Handler()); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
