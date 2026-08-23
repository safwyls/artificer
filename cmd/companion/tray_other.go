//go:build !windows

package main

import "github.com/safwyls/artificer/companion"

// runUI on non-Windows platforms is a plain foreground process: the game
// client only exists on Windows, so anything else running this is a
// developer with a terminal. The default companion.ExitForRestart (a
// plain exit) is right here — nothing to tear down.
func runUI(a *companion.App, url string) {
	select {}
}
