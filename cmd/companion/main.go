// companion is the Artificer Companion — the Artificer app that runs on
// a player's own machine. It began life as wkcompanion, the Dragonwilds
// character relay, and that is still its first job: the game keeps each
// character's record on the player's machine
// (games/dragonwilds/docs/recon.md, "Where player state lives"), so this
// program watches the local SaveCharacters directory, shows the
// character sheet in a local browser page, and — only when the player
// configures it — relays the record to a wildskeeper console using a
// companion token its admin minted.
//
// Its second job is save-sync custody (docs/save-sync-architecture.md):
// checking a shared world out of the console to host it from this
// machine, pushing mid-session checkpoints, and checking it back in —
// authenticated by the player's personal sync token from the console's
// Worlds page. See sync.go.
//
// On Windows it lives in the system tray (build with
// -ldflags="-H windowsgui" so no console window opens): the tray menu
// opens the page, pushes on demand, and shows the sharing state.
// Elsewhere it runs as a plain console process — development platforms,
// not player machines.
//
// Design notes, in the repo's spirit:
//   - Local-first: with no console configured it is a character viewer
//     and nothing leaves the machine.
//   - The relay pushes the record verbatim; the console re-parses it with
//     the same dwsave code this program uses, so there is exactly one
//     parser to be wrong in.
//   - No installer, no service: one binary, a tray icon, a config file
//     under the user's config directory.
//   - One app for every game Artificer runs, not a tray binary per game:
//     game modules contribute their client-side knowledge to this
//     command the way they contribute modules to the consoles.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8377", "local address for the companion page (loopback only by design)")
	dir := flag.String("dir", "", "SaveCharacters directory (default: auto-detect the game's)")
	noBrowser := flag.Bool("no-browser", false, "do not open the companion page on start")
	flag.Parse()

	cfg, cfgPath, err := loadConfig()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}
	if *dir != "" {
		cfg.SaveDir = *dir
	}
	setupLogging(cfgPath)

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		// A second launch is a normal user action, not an error: hand over
		// to the instance already running and bow out.
		if alreadyRunning(*listen) {
			fmt.Printf("the companion is already running — opening http://%s/\n", *listen)
			openBrowser("http://" + *listen + "/")
			return
		}
		log.Fatalf("listening on %s: %v", *listen, err)
	}
	url := fmt.Sprintf("http://%s/", ln.Addr())

	app := newApp(cfg, cfgPath)
	go app.watchLoop()
	go func() {
		if err := http.Serve(ln, app.routes()); err != nil {
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

// setupLogging mirrors logs into a file beside the config: a
// -H=windowsgui build has no console, and "why didn't it push" must be
// answerable after the fact.
func setupLogging(cfgPath string) {
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

// alreadyRunning checks whether the listen address is a live companion.
func alreadyRunning(addr string) bool {
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

// openBrowser is best-effort: the printed URL is the real interface.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
	go func() {
		_ = cmd.Wait()
	}()
}

// watchLoop is the whole engine: scan the save directory, keep the state
// fresh for the page, and relay changes when a console is configured. A
// steady heartbeat re-push keeps the console's in-memory inbox warm across
// its restarts. The save-sync side rides the same loop: a status poll,
// hold adoption when a queued claim came through, and the checkpoint
// pushes (sync.go).
func (a *app) watchLoop() {
	const (
		scanEvery      = 15 * time.Second
		heartbeatEvery = 10 * time.Minute
	)
	lastHeartbeat := time.Time{}
	for {
		a.scan()
		force := time.Since(lastHeartbeat) >= heartbeatEvery
		if a.relayConfigured() {
			if a.pushChanged(force) && force {
				lastHeartbeat = time.Now()
			}
		}
		a.syncTick()
		time.Sleep(scanEvery)
	}
}
