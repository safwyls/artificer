// Command anvil runs the host provisioning service.
//
// One per machine. It holds the Docker socket so the game consoles don't
// have to, and serves them a shaped set of verbs over a bearer token.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/safwyls/anvil/internal/host"
)

// version is stamped at build time (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	healthz := flag.Bool("healthz", false, "probe the local service and exit 0 when healthy")
	flag.Parse()

	addr := envOr("ANVIL_ADDR", ":8820")
	if *healthz {
		os.Exit(probe(addr))
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	// Per-console registrations: each game dashboard gets its own token,
	// data root and (optionally) image allowlist. JSON, inline or from a
	// file — the file wins, and is the better habit for secrets:
	//   ANVIL_CLIENTS='[{"id":"wildskeeper","token":"...","dataRoot":"/mnt/tank/apps/dragonwilds-servers"},
	//                    {"id":"palcon","token":"...","dataRoot":"/mnt/tank/apps/palworld-servers"}]'
	clients, err := host.LoadClients(os.Getenv("ANVIL_CLIENTS"), os.Getenv("ANVIL_CLIENTS_FILE"))
	if err != nil {
		logger.Error("invalid client registrations", "error", err)
		os.Exit(1)
	}
	svc, err := host.New(host.Config{
		Clients:    clients,
		DockerHost: envOr("ANVIL_DOCKER_HOST", "unix:///var/run/docker.sock"),
		PublicHost: os.Getenv("ANVIL_PUBLIC_HOST"),
		// Comma-separated. Leaving it unset keeps the narrow default rather
		// than opening the host up, which is the right way round for a
		// service holding the docker socket.
		AllowedImagePrefixes: splitList(os.Getenv("ANVIL_ALLOWED_IMAGE_PREFIXES")),
		DefaultRunAs:         os.Getenv("ANVIL_DEFAULT_RUN_AS"),
		Version:              version,
		Logger:               logger,
	})
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           svc.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// A rebuild can outlive a slow image pull, so shutdown waits rather than
	// cutting one off half-done.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		logger.Info("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	logger.Info("anvil listening", "addr", addr, "version", version)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// probe backs the container healthcheck: the image ships no curl, so the
// binary checks itself.
func probe(addr string) int {
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + addr + "/v1/health")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	// 401 is a healthy service refusing an unauthenticated probe, which is
	// exactly what this request is — it proves the listener is up.
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusOK {
		return 0
	}
	return 1
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
