// companion is the Artificer Companion — the save-sync client that runs
// on a player's own machine (docs/save-sync-architecture.md). It finds
// the games installed here, links their save folders to shared worlds on
// the save-sync service (reliquary), and moves the saves: checkout to
// host, mid-session checkpoints as crash insurance, check-in when the
// hosting stretch ends, and automatic pickup when a queued claim comes
// through.
//
// It began life as wkcompanion, the Dragonwilds character relay; that
// job retired when the recon showed the dedicated server carries the
// character sheets itself (games/dragonwilds/docs/recon.md), and the
// app is now solely the custody client — deliberately game-blind, one
// binary for every game Artificer syncs.
//
// The engine — discovery, custody sync, the local server, in-place
// updates — lives in the importable companion package; this entrypoint
// is the browser-and-tray shell around it. The native desktop shell
// (companion-desktop, shipping as reliquary-companion) wraps the same
// package in its own window.
//
// On Windows it lives in the system tray (build with
// -ldflags="-H windowsgui" so no console window opens): the tray menu
// opens the page and shows the custody state. Elsewhere it runs as a
// plain console process — development platforms, not player machines.
//
// Design notes, in the repo's spirit:
//   - Local-first: with no service configured, nothing leaves the
//     machine and the app is a save-folder finder.
//   - No installer, no service: one binary, a tray icon, a config file
//     under the user's config directory (wkcompanion-era configs are
//     read as a fallback so upgrades keep their token).
//   - Save locations are discovered as candidates and confirmed by the
//     player, never followed blindly — most are heuristics.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/safwyls/artificer/companion"
)

// version is stamped by the release build (-X main.version=<sha>);
// "dev" means a local build.
var version = "dev"

func main() {
	listen := flag.String("listen", "127.0.0.1:8377", "local address for the companion page (loopback only by design)")
	noBrowser := flag.Bool("no-browser", false, "do not open the companion page on start")
	flag.Parse()
	companion.Version = version
	log.Printf("artificer companion %s", version)

	cfg, cfgPath, err := companion.LoadConfig()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}
	companion.SetupLogging(cfgPath)
	// An update leaves the previous build beside this one, because a
	// running binary cannot delete itself. Startup is the first moment
	// it is no longer running (update.go).
	companion.ClearOldBinary()

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		// A second launch is a normal user action, not an error: hand over
		// to the instance already running and bow out.
		if companion.AlreadyRunning(*listen) {
			fmt.Printf("the companion is already running — opening http://%s/\n", *listen)
			openBrowser("http://" + *listen + "/")
			return
		}
		log.Fatalf("listening on %s: %v", *listen, err)
	}
	url := fmt.Sprintf("http://%s/", ln.Addr())

	app := companion.NewApp(cfg, cfgPath)
	app.Rescan()
	go app.WatchLoop()
	go app.WatchUpdates(context.Background())
	go func() {
		if err := http.Serve(ln, app.Routes()); err != nil {
			log.Fatalf("local server: %v", err)
		}
	}()

	fmt.Printf("artificer companion — your page is at %s\n", url)
	fmt.Printf("config: %s\n", cfgPath)
	if !*noBrowser {
		openBrowser(url)
	}

	// Blocks until quit: the system tray on Windows, a plain wait
	// elsewhere. See tray_windows.go / tray_other.go.
	runUI(app, url)
}

// openBrowser is best-effort: the printed URL is the real interface.
// Same desktop opener the game launch uses (companion/launch.go).
func openBrowser(url string) { _ = companion.OpenURI(url) }
