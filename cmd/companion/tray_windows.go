//go:build windows

package main

import (
	_ "embed"
	"time"

	"fyne.io/systray"
)

//go:embed assets/icon.ico
var trayIcon []byte

// runUI parks the companion in the system tray: open the sheet, push on
// demand, read the sharing state at a glance, quit. The character sheet
// itself stays a browser page — the tray is the handle, not the UI.
func runUI(a *app, url string) {
	// A console-subsystem build (plain `go build`) double-clicked from
	// Explorer drags a console window along; close it once startup has
	// printed the URL. See console_windows.go.
	detachOwnConsole()
	systray.Run(func() {
		systray.SetIcon(trayIcon)
		systray.SetTooltip("Artificer Companion")

		open := systray.AddMenuItem("Open companion page", "Your characters and shared worlds, in the browser")
		push := systray.AddMenuItem("Push to console now", "Send your character sheet immediately")
		systray.AddSeparator()
		status := systray.AddMenuItem("Starting…", "Sharing state")
		status.Disable()
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "Stop watching and sharing")

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
				case <-push.ClickedCh:
					if !a.relayConfigured() {
						// Nothing to push to: the settings live on the page.
						openBrowser(url)
						continue
					}
					go func() {
						a.scan()
						a.pushChanged(true)
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
