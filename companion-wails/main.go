// reliquary-companion is the Wails desktop shell around the companion
// engine (the importable companion package): the same save-sync client
// as cmd/companion, but living in its own native window (WebView2 on
// Windows) instead of a browser tab.
//
// It is being built out in parallel with the browser-and-tray build and
// will replace it once it is ready; until the cutover both exist, and
// they must never run at the same time — they share the config file and
// the custody state, so a second syncing instance is refused, exactly
// like a second cmd/companion launch is.
//
// What differs from cmd/companion:
//   - The window is the UI. The embedded web/companion frontend is
//     served straight to the webview by the asset server; /api requests
//     fall through to the same Routes handler — no TCP hop.
//   - The local page on 127.0.0.1:8377 stays up anyway: that address is
//     frozen API (docs/companion-ui-rebuild.md), and it keeps the
//     "open in a real browser" escape hatch working.
//   - Self-update follows its own release identity (reliquary-companion
//     assets under its own rolling tag), so the two builds can never
//     replace each other.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/safwyls/artificer/companion"
	web "github.com/safwyls/artificer/web/companion"
)

// version is stamped by the release build (-X main.version=<sha>);
// "dev" means a local build.
var version = "dev"

// listenAddr is the frozen local address every companion build serves
// its page on; it doubles as the single-instance lock.
const listenAddr = "127.0.0.1:8377"

func main() {
	companion.Version = version
	// This build ships under its own name and rolling tag, so an update
	// can never swap in (or be swapped out by) the browser build.
	companion.UpdateTag = "reliquary-companion-latest"
	companion.UpdateAssets = map[string]string{"windows": "reliquary-companion.exe"}
	log.Printf("reliquary companion %s", version)

	cfg, cfgPath, err := companion.LoadConfig()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}
	companion.SetupLogging(cfgPath)
	companion.ClearOldBinary()

	// One syncing companion per machine, whichever build it is: both
	// share the config and the custody state, and two auto-checkpoint
	// loops over the same links would fight each other.
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		if companion.AlreadyRunning(listenAddr) {
			log.Printf("a companion is already running at http://%s/ — this window would sync against the same worlds, so it is bowing out", listenAddr)
			_ = companion.OpenURI("http://" + listenAddr + "/")
			return
		}
		log.Fatalf("listening on %s: %v", listenAddr, err)
	}

	app := companion.NewApp(cfg, cfgPath)
	app.Rescan()
	go app.WatchLoop()
	go app.WatchUpdates(context.Background())

	api := app.Routes()
	go func() {
		if err := http.Serve(ln, api); err != nil {
			log.Fatalf("local server: %v", err)
		}
	}()
	fmt.Printf("reliquary companion — window opening; the page is also at http://%s/\n", listenAddr)
	fmt.Printf("config: %s\n", cfgPath)

	dist, err := web.Dist()
	if err != nil {
		log.Fatalf("embedded frontend: %v", err)
	}

	if err := wails.Run(&options.App{
		Title:  "Reliquary Companion",
		Width:  1120,
		Height: 780,
		AssetServer: &assetserver.Options{
			Assets: dist,
			// Anything the frontend asks for that is not an asset —
			// every /api call — lands on the same handler the local
			// page uses. One surface, two doors.
			Handler: api,
		},
	}); err != nil {
		log.Printf("window: %v", err)
		os.Exit(1)
	}
}
