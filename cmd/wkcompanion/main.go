// wkcompanion is the player-side companion for RuneScape: Dragonwilds.
//
// The game keeps each character's record — name, skills, inventory,
// vitals — on the player's own machine (games/dragonwilds/docs/recon.md,
// "Where player state lives"), so a dedicated server's console can never
// show more than guid and position on its own. This program runs where
// the data actually is: it watches the local SaveCharacters directory,
// shows the character sheet in a local browser page, and — only when the
// player configures it — relays the record to a wildskeeper console using
// a companion token its admin minted.
//
// Design notes, in the repo's spirit:
//   - Local-first: with no console configured it is a character viewer
//     and nothing leaves the machine.
//   - The relay pushes the record verbatim; the console re-parses it with
//     the same dwsave code this program uses, so there is exactly one
//     parser to be wrong in.
//   - No installer, no service: one binary, a browser tab, a config file
//     under the user's config directory.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

	"os/exec"
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

	app := newApp(cfg, cfgPath)
	go app.watchLoop()

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatalf("listening on %s: %v", *listen, err)
	}
	url := fmt.Sprintf("http://%s/", ln.Addr())
	fmt.Printf("wkcompanion — your character sheet is at %s\n", url)
	fmt.Printf("config: %s\n", cfgPath)
	if !*noBrowser {
		openBrowser(url)
	}
	log.Fatal(http.Serve(ln, app.routes()))
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
// its restarts.
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
		time.Sleep(scanEvery)
	}
}
