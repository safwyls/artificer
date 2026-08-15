// Command flameagent is the per-server sidecar agent: it sits next to one
// Dragonwilds game server (or supervises it directly), holding the install
// volume and SteamCMD, and exposes a narrow authenticated API for flamekeeper to
// drive. See docs/sidecar-agent.md for the design.
package main

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/safwyls/flamekeeper/internal/flameagent"
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
		if err != nil || n < 1 || n > 65534 {
			// 65535 is excluded on purpose: the game also uses port+1.
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
		InstallDir:  envOr("FLAMEAGENT_INSTALL_DIR", "/dragonwilds"),
		SteamCmd:    envOr("FLAMEAGENT_STEAMCMD", "steamcmd"),
		AppID:       appID,
		Mode:        envOr("FLAMEAGENT_MODE", "companion"),
		GameCommand: os.Getenv("FLAMEAGENT_GAME_CMD"),
		GameArgs:    strings.Fields(os.Getenv("FLAMEAGENT_GAME_ARGS")),
		Launch: flameagent.LaunchConfig{
			// The initial selection only applies to an install that has
			// never been told otherwise — the persisted choice wins, so
			// redeploying the container doesn't silently change which build
			// the server runs.
			Profile:      envOr("FLAMEAGENT_LAUNCH_PROFILE", flameagent.ProfileNative),
			WineBin:      os.Getenv("FLAMEAGENT_WINE_BIN"),
			WinePrefix:   os.Getenv("FLAMEAGENT_WINE_PREFIX"),
			GameExe:      os.Getenv("FLAMEAGENT_GAME_EXE"),
			NativeScript: os.Getenv("FLAMEAGENT_NATIVE_SCRIPT"),
		},
		BridgeKitDir:    os.Getenv("FLAMEAGENT_BRIDGE_KIT_DIR"),
		GamePort:        gamePort,
		StopGrace:       stopGrace,
		AdminPassword:   os.Getenv("FLAMEAGENT_ADMIN_PASSWORD"),
		OwnerID:         os.Getenv("FLAMEAGENT_OWNER_ID"),
		ServerName:      os.Getenv("FLAMEAGENT_SERVER_NAME"),
		WorldName:       os.Getenv("FLAMEAGENT_WORLD_NAME"),
		DockerHost:      os.Getenv("FLAMEAGENT_DOCKER_HOST"),
		DataRoot:        os.Getenv("FLAMEAGENT_DATA_ROOT"),
		PublicHost:      os.Getenv("FLAMEAGENT_PUBLIC_HOST"),
		DefaultRunAs:    os.Getenv("FLAMEAGENT_DEFAULT_RUN_AS"),
		DefaultImageTag: os.Getenv("FLAMEAGENT_DEFAULT_IMAGE_TAG"),
		Autostart:       autostart,
		Version:         version,
		Logger:          logger,
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
