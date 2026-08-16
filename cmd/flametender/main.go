// Command flametender is Flametender: a self-hosted management console for
// Enshrouded dedicated servers.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/safwyls/flametender/internal/agentfiles"
	"github.com/safwyls/flametender/internal/api"
	"github.com/safwyls/flametender/internal/backup"
	"github.com/safwyls/flametender/internal/cfaccess"
	"github.com/safwyls/flametender/internal/collector"
	"github.com/safwyls/flametender/internal/config"
	"github.com/safwyls/flametender/internal/crypto"
	"github.com/safwyls/flametender/internal/db"
	"github.com/safwyls/flametender/internal/dockerctl"
	"github.com/safwyls/flametender/internal/ilmari"
	"github.com/safwyls/flametender/internal/notify"
	"github.com/safwyls/flametender/internal/sched"
	"github.com/safwyls/flametender/internal/store"
	"github.com/safwyls/flametender/internal/watchdog"
	"github.com/safwyls/flametender/web"

	// Populates the game registry. Without it every server row would
	// resolve to "unknown game" — see internal/games.
	_ "github.com/safwyls/flametender/internal/games"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	sqlDB, err := db.Open(cfg.DBPath())
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	box, err := crypto.New(cfg.EncryptionKey)
	if err != nil {
		return err
	}
	st := store.New(sqlDB, box)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := api.BootstrapAdmin(ctx, st, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		return err
	}

	distFS, err := web.Dist()
	if err != nil {
		return err
	}

	// Discord notifications: the collector reports reachability changes
	// and player joins/leaves through it, the scheduler restart notices.
	notifier := notify.New(st, logger)

	// Samples server health in the background so the dashboard charts have
	// history to draw, rather than only what's happened since page load.
	// Shutdown is awaited below: it closes out open play sessions.
	collectorDone := make(chan struct{})
	go func() {
		defer close(collectorDone)
		collector.New(st, notifier, logger).Run(ctx)
	}()

	// Resolves each server's save/config to a local path — a bind mount,
	// or a cache mirrored from its flameagent sidecar (phase 2).
	files := agentfiles.New(cfg.DataDir, logger)

	// For agent-backed servers this loop drives the save sync that backups
	// snapshot. The nil reader is honest: nothing parses Enshrouded's world
	// blob (no public schema exists), so only the sync runs. A metadata
	// reader for the save index is roadmap Phase 3.
	go collector.NewSaveRefresher(st, nil, files, logger).Run(ctx)

	// Optional: without DOCKER_HOST, power control is simply absent.
	var docker *dockerctl.Client
	if cfg.DockerHost != "" {
		docker, err = dockerctl.New(cfg.DockerHost)
		if err != nil {
			return fmt.Errorf("configuring docker control: %w", err)
		}
		logger.Info("docker control enabled", "endpoint", cfg.DockerHost)
	}

	// Runs scheduled restarts (warnings included) for every server.
	go sched.New(st, notifier, docker, logger).Run(ctx)

	// Crash watchdog: revives watched containers after an unclean exit.
	// Meaningless without docker control, so it only runs alongside it.
	if docker != nil {
		go watchdog.New(st, docker, notifier, logger).Run(ctx)
	}

	// Save backups: zip snapshots of the read-only save mount into the
	// data dataset, on each server's schedule.
	backups := backup.New(st, notifier, logger, cfg.DataDir, files)
	go backups.Run(ctx)

	apiServer := api.New(st, cfg.JWTSecret, logger, docker, notifier, backups, files)
	apiServer.CookieSecure = cfg.CookieSecure
	// Optional: single sign-on for a console behind a Cloudflare Tunnel.
	// Unset means the password form is the only way in — see
	// docs/cloudflare-access.md for what the verification does and does
	// not protect.
	if cfg.AccessEnabled() {
		verifier, err := cfaccess.New(cfg.AccessTeamDomain, cfg.AccessAUD)
		if err != nil {
			return fmt.Errorf("configuring cloudflare access: %w", err)
		}
		apiServer.Access = verifier
		apiServer.AccessAdminEmails = cfg.AccessAdminEmails
		logger.Info("cloudflare access sign-in enabled",
			"issuer", verifier.Issuer(), "adminEmails", len(cfg.AccessAdminEmails))
	}
	// Optional one-click provisioning, through Ilmari and only Ilmari —
	// this console holds no Docker rights of its own (the ilmari repo's
	// README is the contract). Without ILMARI_URL the Raise-a-server
	// wizard is simply absent and servers are registered by hand.
	if cfg.IlmariURL != "" {
		client, err := ilmari.New(cfg.IlmariURL, cfg.IlmariToken)
		if err != nil {
			return fmt.Errorf("configuring ilmari: %w", err)
		}
		apiServer.Provisioner = api.NewIlmariProvisioner(client)
		logger.Info("provisioner enabled", "endpoint", cfg.IlmariURL, "via", "ilmari")
	}
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiServer.Routes(distFS),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		err := httpServer.Shutdown(shutdownCtx)
		// The collector ends the sessions of whoever is still online on its
		// way out. Exiting without waiting strands those joins, and an
		// unclosed join reads as a session that never ended.
		select {
		case <-collectorDone:
		case <-shutdownCtx.Done():
			logger.Warn("collector did not finish closing open sessions")
		}
		return err
	case err := <-errCh:
		return err
	}
}
