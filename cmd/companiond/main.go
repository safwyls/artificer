// companiond is the headless build of the Artificer Companion engine:
// the same companion package cmd/companion runs, with no page opened, no
// tray, and no window — a child process an Electron shell spawns, talks
// to over loopback HTTP, and outlives nothing (companion-cutover.md,
// phases 1 and 2).
//
// The contract with the shell, which is the whole reason this binary is
// separate from cmd/companion:
//
//   - stdout is a handshake channel. Its first line is the resolved
//     listen address and nothing else is ever written to it, so a parent
//     can read one line and know where to connect. Logs go to stderr
//     (companion.SetupLoggingTo), and to the log file beside the config
//     as always.
//   - Every request must carry `Authorization: Bearer <token>` except
//     GET /healthz. The daemon refuses to start without a token, because
//     a loopback listener is reachable by every process on the machine
//     — a page in the player's browser included — and this one can move
//     save files around.
//   - stdin is the parent-death signal. When it closes the daemon shuts
//     down, so a crashed shell does not leave an orphan holding the port
//     and the save folders' file handles.
//
// The window belongs to the shell, so POST /api/raise answers 501 here
// naming where it lives; Electron main raises its own BrowserWindow over
// IPC.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/safwyls/artificer/companion"
)

// version is stamped by the release build (-X main.version=<sha>);
// "dev" means a local build. Same stamp cmd/companion carries.
var version = "dev"

// tokenEnv is where the shell hands the daemon its bearer token. An env
// var rather than a flag so the secret does not sit in the process
// table for every other user on the machine to read.
const tokenEnv = "COMPANIOND_TOKEN"

// shutdownGrace bounds the wait for in-flight requests once stdin has
// closed. The parent is gone; nothing is waiting on a reply that long.
const shutdownGrace = 3 * time.Second

func main() {
	addr := flag.String("port", "127.0.0.1:0", "local address to listen on (loopback only by design; port 0 picks a free one)")
	tokenFlag := flag.String("token", "", "bearer token to require, for standalone runs; prefer "+tokenEnv)
	flag.Parse()
	companion.Version = version

	token := *tokenFlag
	if token == "" {
		token = os.Getenv(tokenEnv)
	}
	if token == "" {
		// Refusing is the point: a tokenless daemon is one any process on
		// the box can drive, and it can move a player's saves.
		log.Fatalf("no auth token: set %s or pass --token (companiond will not serve unauthenticated)", tokenEnv)
	}

	cfg, cfgPath, err := companion.LoadConfig()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}
	// stderr, not stdout: stdout is the handshake channel.
	companion.SetupLoggingTo(cfgPath, os.Stderr)
	companion.ClearOldBinary()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listening on %s: %v", *addr, err)
	}

	app := companion.NewApp(cfg, cfgPath)
	app.Rescan()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go app.WatchLoop()
	// No update watcher here, deliberately. This daemon is one file
	// inside an installed application, and the application updates
	// itself: the Electron shell runs electron-updater, which is the
	// only thing that can replace what companiond lives inside. A second
	// checker would be a second answer to "is there an update", and two
	// readings of one fact drift.
	//
	// cmd/companion — the browser-and-tray build, which really is a
	// single exe that replaces itself — keeps the engine's watcher.

	srv := &http.Server{Handler: app.RoutesWithOptions(companion.ServerOptions{
		Token: token,
		// No Raise: this build has no window. The route answers 501 with
		// a reason rather than pretending.
	})}

	// The first line, before anything else can be written, and flushed
	// by the newline: the parent blocks on reading it.
	fmt.Println(ln.Addr().String())
	log.Printf("artificer companiond %s listening on %s (config: %s)", version, ln.Addr(), cfgPath)

	go func() {
		// EOF on stdin means the parent is gone.
		_, _ = io.Copy(io.Discard, os.Stdin)
		log.Printf("stdin closed — shutting down")
		stop()
	}()

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("local server: %v", err)
	}
}
