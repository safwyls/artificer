//go:build !windows

package main

// runUI on non-Windows platforms is a plain foreground process: the game
// client only exists on Windows, so anything else running this is a
// developer with a terminal.
func runUI(a *app, url string) {
	select {}
}
