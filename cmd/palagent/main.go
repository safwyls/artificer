// Command palagent is the per-server sidecar agent: it sits next to one
// Palworld game server container, holding the install volume and SteamCMD,
// and exposes a narrow authenticated API for palcon to drive. See
// docs/sidecar-agent.md for the design.
package main

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/safwyls/palcon/internal/palagent"
)

// version is stamped by the release build via
// -ldflags "-X main.version=v0.x.y"; "dev" means a local build.
var version = "dev"

func main() {
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

	agent, err := palagent.New(palagent.Config{
		Token:      os.Getenv("PALAGENT_TOKEN"),
		InstallDir: envOr("PALAGENT_INSTALL_DIR", "/palworld"),
		SteamCmd:   envOr("PALAGENT_STEAMCMD", "steamcmd"),
		AppID:      appID,
		Version:    version,
		Logger:     logger,
	})
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	addr := envOr("PALAGENT_ADDR", ":8811")
	logger.Info("palagent listening", "addr", addr, "version", version, "apiVersion", palagent.APIVersion)
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
