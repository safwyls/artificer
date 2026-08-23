// Package companion is the Artificer Companion's engine — the save-sync
// client logic that runs on a player's own machine
// (docs/save-sync-architecture.md). It finds the games installed here,
// links their save folders to shared worlds on the save-sync service
// (reliquary), and moves the saves.
//
// Two entrypoints share it: cmd/companion, the original tray-and-browser
// build, and cmd/companiond, the headless daemon an Electron shell
// spawns (companion-cutover.md). Both wire the same App, HTTP routes and
// loops; only the process shell differs. The in-process facade in
// facade.go stays for a shell that lives in this process; the daemon's
// shell is out of process and reads the same state over the routes and
// the SSE stream.
package companion

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Version is stamped by the entrypoint from its own build stamp
// (-X main.version=<sha>); "dev" means a local build.
var Version = "dev"

// ExitForRestart ends this process so the replacement started by
// restartSelf takes over. The default is a plain exit; an entrypoint
// with a tray or a window to tear down first replaces it (the systray
// icon lingers as a ghost if the process exits without giving it up).
var ExitForRestart = func() { os.Exit(0) }

// WatchLoop is the whole engine: a custody poll against the service,
// handoff adoption when a queued claim came through, and the automatic
// checkpoint pushes (sync.go).
func (a *App) WatchLoop() {
	const tickEvery = 15 * time.Second
	for {
		a.SyncTick()
		time.Sleep(tickEvery)
	}
}

// SetupLogging mirrors logs into a file beside the config: a windowed
// build has no console, and "why didn't it sync" must be answerable
// after the fact.
func SetupLogging(cfgPath string) {
	logPath := filepath.Join(filepath.Dir(cfgPath), "companion.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(os.Stdout, f))
}

// AlreadyRunning checks whether the listen address is a live companion.
func AlreadyRunning(addr string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/api/state", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
