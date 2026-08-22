package main

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Starting the game once the save is in place.
//
// Checking a world out is two halves of one intention: fetch the save,
// then play. The companion already did the first half and left the
// player to alt-tab and find the game themselves, which is the moment
// the save is most likely to be opened by the *wrong* copy — the one
// that was already running with the old world loaded.
//
// Order matters and is not negotiable: the save is installed first and
// the game started after. Launching first would have the game read the
// stale save and write over it at its first autosave, which is the one
// failure this whole system exists to prevent.

// launchTarget is what the OS is asked to open for a linked world: the
// player's own override if they set one, else Steam's run URI built from
// the app id discovery recorded. Empty means this world cannot be
// launched — a game linked by hand from a folder, with nothing saying
// what starts it.
func launchTarget(l *WorldLink) string {
	if l == nil {
		return ""
	}
	if t := strings.TrimSpace(l.LaunchTarget); t != "" {
		return t
	}
	if l.AppID != "" {
		// rungameid rather than steam://run: it is the form Steam's own
		// shortcuts use, and it works for games whose app id is not the
		// id of the executable Steam runs.
		return "steam://rungameid/" + l.AppID
	}
	return ""
}

// launchable reports whether this world has anything to start.
func launchable(l *WorldLink) bool { return launchTarget(l) != "" }

// launch opens the world's launch target with the OS's own handler,
// which is what makes one field cover every case worth covering: a
// steam:// URI, a game's .exe, a .lnk shortcut, or another launcher's
// URI scheme all open the same way. It deliberately does not parse a
// command line — quoting a Windows path with spaces into arguments is a
// bug generator, and a shortcut carries the arguments already.
func (a *app) launch(worldID int64) error {
	l := a.link(worldID)
	if l == nil {
		return errors.New("link this world to a save folder first")
	}
	target := launchTarget(l)
	if target == "" {
		return errors.New("this world has no Steam app id recorded, so the companion does not know what starts it — set a launch target for it, or start the game yourself")
	}
	if err := openLaunchURI(target); err != nil {
		return fmt.Errorf("starting %s: %w", target, err)
	}
	a.noteSync("started " + target)
	return nil
}

// openLaunchURI is the seam a test swaps: starting a real game from a
// unit test is not a thing to do by accident.
var openLaunchURI = openURI

// openURI hands a URI or path to the desktop's own opener. Best-effort
// and asynchronous: a launcher that takes ten seconds to show a window
// must not hold up the answer to the page.
func openURI(uri string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", uri)
	case "darwin":
		cmd = exec.Command("open", uri)
	default:
		cmd = exec.Command("xdg-open", uri)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}
