// reliquary-companion is the Artificer Companion as a desktop
// application: the same save-sync engine as cmd/companion (the
// importable companion package), rendered in a real window by a desktop
// toolkit rather than in a browser tab or a webview.
//
// It supersedes the Wails shell. That shell proved out the parallel-app
// structure, the separate release identity and the engine extraction,
// but it still drew the React UI inside WebView2. This one draws native
// widgets in-process with the engine: no HTTP hop for its own state, no
// serialization, direct method calls and callbacks.
//
// "Native" here means a real desktop application — its own window and
// event loop, OS folder dialogs, an OS tray, OS notifications, no
// browser engine anywhere — not stock Win32 controls. Fyne renders its
// own widgets, so the vault design language is carried by a theme
// (theme.go) and by a handful of small custom widgets, as closely as
// that allows.
//
// What is deliberately unchanged from every other companion build:
//
//   - The local page on 127.0.0.1:8377 stays up. That address is frozen
//     API (docs/companion-ui-rebuild.md), it is the "open it in a real
//     browser" escape hatch, and it is what an old build hands over to.
//   - One syncing companion per machine, whichever build: two of them
//     over one config file would fight over custody, so a second launch
//     raises the running window and bows out.
//   - Self-update follows this build's own release identity
//     (reliquary-companion assets under their own rolling tag), so it
//     and the browser build can never replace each other.
//   - The UI is game-blind. A game's name, cover and save quirks arrive
//     as data from the engine; nothing here branches on which game a
//     world belongs to.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"

	"github.com/safwyls/artificer/companion"
)

// version is stamped by the release build (-X main.version=<sha>);
// "dev" means a local build.
var version = "dev"

const (
	// listenAddr is the frozen local address every companion build
	// serves its page on; it doubles as the single-instance lock.
	listenAddr = "127.0.0.1:8377"
	// appID names this app to the OS — the notification source, and the
	// key Fyne files its own per-app storage under.
	appID = "com.artificer.reliquary-companion"
	// windowTitle is the product identity, carried over from the Wails
	// shell unchanged.
	windowTitle = "Reliquary Companion"
)

func main() {
	minimized := flag.Bool("minimized", false, "start hidden in the tray (what the autostart entry passes)")
	flag.Parse()

	companion.Version = version
	// This build ships under its own name and rolling tag, so an update
	// can never swap in (or be swapped out by) the browser build.
	companion.UpdateTag = "reliquary-companion-latest"
	companion.UpdateAssets = map[string]string{"windows": "reliquary-companion.exe"}
	log.Printf("reliquary companion %s", version)

	cfg, cfgPath, err := companion.LoadConfig()
	if err != nil {
		// Before the window exists there is nothing to show a dialog on,
		// and a windowed build has no console to print to — the log file
		// beside the config is the answer, which SetupLogging cannot yet
		// have opened. Fatal is honest here.
		log.Fatalf("loading config: %v", err)
	}
	companion.SetupLogging(cfgPath)
	// An update leaves the previous build beside this one, because a
	// running binary cannot delete itself. Startup is the first moment it
	// is no longer running.
	companion.ClearOldBinary()

	// One syncing companion per machine, whichever build it is: both
	// share the config and the custody state, and two auto-checkpoint
	// loops over the same links would fight each other.
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		if companion.AlreadyRunning(listenAddr) {
			log.Printf("a companion is already running at http://%s/ — this window would sync against the same worlds, so it is bowing out", listenAddr)
			handOverToRunningInstance()
			return
		}
		log.Fatalf("listening on %s: %v", listenAddr, err)
	}

	engine := companion.NewApp(cfg, cfgPath)
	engine.Rescan()
	go engine.WatchLoop()
	go engine.WatchUpdates(context.Background())

	u := &ui{
		engine:     engine,
		cfgPath:    cfgPath,
		covers:     map[string]fyne.Resource{},
		holdWarned: map[int64]time.Time{},
	}

	// The frozen local surface, plus this build's one additive route.
	// Additive is allowed; removing a route is not.
	go func() {
		if err := http.Serve(ln, u.routes()); err != nil {
			log.Fatalf("local server: %v", err)
		}
	}()
	fmt.Printf("%s — window opening; the page is also at http://%s/\n", windowTitle, listenAddr)
	fmt.Printf("config: %s\n", cfgPath)

	u.start(*minimized)
}

// handOverToRunningInstance raises the window of the companion already
// running, and falls back to opening its page when that instance is an
// older build with no raise route. Either way the player gets a UI,
// which is the whole point: launching the app twice is a normal thing to
// do, not an error to report.
func handOverToRunningInstance() {
	if raiseRunningInstance() {
		return
	}
	_ = companion.OpenURI("http://" + listenAddr + "/")
}

// start builds the window, wires the tray, and blocks until quit.
func (u *ui) start(minimized bool) {
	u.app = fyneapp.NewWithID(appID)
	u.app.Settings().SetTheme(vaultTheme{})

	u.win = u.app.NewWindow(windowTitle)
	if icon := trayIcon(); icon != nil {
		u.win.SetIcon(icon)
	}
	u.win.Resize(u.restoreWindowSize())
	u.win.SetContent(u.build())

	// Closing the window hides it: this window is a view over a resident
	// sync process, and shutting the view must not stop the syncing. Quit
	// lives in the tray menu, and confirms while a save is moving.
	u.win.SetCloseIntercept(func() { u.hideToTray() })

	u.setupTray()
	u.watchEngine()
	u.startPresence()

	// The window's title-bar close is the way out of the *window*; the
	// app quits from the tray. Fyne needs a master window all the same,
	// or the first window closing ends the process.
	u.win.SetMaster()

	if minimized {
		// The autostart entry passes --minimized: the companion should
		// come up syncing, in the tray, without a window appearing over
		// whatever the player was doing at login.
		u.visible.Store(false)
		u.app.Run()
		return
	}
	u.visible.Store(true)
	u.win.ShowAndRun()
}
