// Command palagent is the per-server sidecar agent: it sits next to one
// Palworld game server (or supervises it directly), holding the install
// volume and SteamCMD, and exposes a narrow authenticated API for palcon
// to drive. The shared machinery is core/agent; the Palworld half is
// games/palworld/palagent.
//
// Provisioner mode is gone on purpose: placing containers is Anvil's
// job, so PALAGENT_DOCKER_HOST and its provisioning siblings
// (PALAGENT_DATA_ROOT, PALAGENT_PUBLIC_HOST, …) are no longer read.
package main

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/safwyls/artificer/core/agent"
	"github.com/safwyls/artificer/games/palworld/palagent"
)

// version is stamped by the release build via
// -ldflags "-X main.version=v0.x.y"; "dev" means a local build.
var version = "dev"

func main() {
	// Container healthcheck mode: probe our own /healthz and exit. The
	// runtime image (steamcmd base) ships neither wget nor curl, so the
	// binary is its own probe.
	if len(os.Args) > 1 && os.Args[1] == "-healthz" {
		addr := envOr("PALAGENT_ADDR", ":8811")
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
	if v := os.Getenv("PALAGENT_APP_ID"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			logger.Error("invalid PALAGENT_APP_ID", "value", v)
			os.Exit(1)
		}
		appID = n
	}
	var stopGrace time.Duration
	if v := os.Getenv("PALAGENT_STOP_GRACE"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			logger.Error("invalid PALAGENT_STOP_GRACE", "value", v)
			os.Exit(1)
		}
		stopGrace = d
	}
	gamePort := 0
	if v := os.Getenv("PALAGENT_GAME_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			logger.Error("invalid PALAGENT_GAME_PORT", "value", v)
			os.Exit(1)
		}
		gamePort = n
	}
	var autostart *bool
	if v := os.Getenv("PALAGENT_AUTOSTART"); v != "" {
		b := v == "true" || v == "1"
		autostart = &b
	}

	a, err := agent.New(agent.Config{
		Token:         os.Getenv("PALAGENT_TOKEN"),
		InstallDir:    envOr("PALAGENT_INSTALL_DIR", "/palworld"),
		SteamCmd:      envOr("PALAGENT_STEAMCMD", "steamcmd"),
		AppID:         appID,
		Mode:          envOr("PALAGENT_MODE", "companion"),
		GameCommand:   os.Getenv("PALAGENT_GAME_CMD"),
		GameArgs:      strings.Fields(os.Getenv("PALAGENT_GAME_ARGS")),
		Game:          palagent.Game(),
		GamePort:      gamePort,
		StopGrace:     stopGrace,
		AdminPassword: os.Getenv("PALAGENT_ADMIN_PASSWORD"),
		ServerName:    os.Getenv("PALAGENT_SERVER_NAME"),
		ServerDesc:    os.Getenv("PALAGENT_SERVER_DESC"),
		Autostart:     autostart,
		Version:       version,
		Logger:        logger,
	})
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	// Supervisor boot: install if missing, then start per desired state.
	go a.Run()

	addr := envOr("PALAGENT_ADDR", ":8811")
	logger.Info("palagent listening", "addr", addr, "version", version, "apiVersion", agent.APIVersion)
	if err := http.ListenAndServe(addr, a.Handler()); err != nil {
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
