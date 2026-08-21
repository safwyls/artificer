//go:build windows

package main

import (
	_ "embed"
	"time"

	"fyne.io/systray"
)

//go:embed assets/icon.ico
var trayIcon []byte

// runUI parks the companion in the system tray: open the page, sync on
// demand, read the custody state at a glance, quit. The page is the UI —
// the tray is the handle.
func runUI(a *app, url string) {
	// A console-subsystem build (plain `go build`) double-clicked from
	// Explorer drags a console window along; close it once startup has
	// printed the URL. See console_windows.go.
	detachOwnConsole()
	systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTooltip("Artificer Companion")

		open := systray.AddMenuItem("Open companion page", "Your shared worlds, in the browser")
		syncNow := systray.AddMenuItem("Sync now", "Poll the service and push any due checkpoints")
		systray.AddSeparator()
		status := systray.AddMenuItem("Starting…", "Custody state")
		status.Disable()
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "Stop syncing")

		// The status line follows the app state; a menu the player only
		// glances at occasionally doesn't need to be fresher than this.
		ticker := time.NewTicker(5 * time.Second)
		update := func() { status.SetTitle(a.statusLine()) }
		update()

		go func() {
			for {
				select {
				case <-open.ClickedCh:
					openBrowser(url)
				case <-syncNow.ClickedCh:
					go func() {
						if a.syncConfigured() {
							a.syncRefresh()
							for _, id := range a.linkedWorldIDs() {
								a.adoptHandoff(id)
								a.autoCheckpoint(id)
							}
						} else {
							openBrowser(url) // nothing to sync: the setup lives on the page
						}
						update()
					}()
				case <-ticker.C:
					update()
				case <-quit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}, nil)
}
