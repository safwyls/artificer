package main

// The tray: the handle on a companion whose window is closed.
//
// Same menu the browser build has carried since it grew one — open,
// sync now, the status line, quit — with "Open companion page" becoming
// "Open Reliquary Companion", because this build has a window to raise
// rather than a browser tab to open. The icon is the same favicon.ico
// the page shows in its tab, embedded once with the frontend, so the
// tray, the taskbar and the browser tab cannot drift apart.
//
// It is the same library the browser build uses (fyne.io/systray),
// reached through Fyne's own tray API rather than by running systray
// directly: Fyne already owns that library's run loop inside this
// process, and a second one cannot be started beside it.

import (
	"io/fs"
	"log"
	"os"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/safwyls/artificer/companion"
	web "github.com/safwyls/artificer/web/companion"
)

// trayIcon is the page's favicon, embedded with the frontend.
func trayIcon() fyne.Resource {
	dist, err := web.Dist()
	if err != nil {
		return nil
	}
	data, err := fs.ReadFile(dist, "favicon.ico")
	if err != nil {
		// A tray with no icon is still a working tray; the menu is the
		// part that matters.
		log.Printf("tray icon: %v", err)
		return nil
	}
	return fyne.NewStaticResource("favicon.ico", data)
}

func (u *ui) setupTray() {
	u.loadNotifyPref()

	tray, ok := u.app.(desktop.App)
	if !ok {
		// A platform with no notification area. The window is still the
		// UI; there is simply nothing behind it to park in.
		log.Print("no system tray on this platform — closing the window will quit")
		u.win.SetCloseIntercept(nil)
		return
	}

	// A disabled item is the tray's read-out. It carries the same one-
	// glance custody state the browser build shows, from the same
	// StatusLine — state, not narrative, and never a filesystem path,
	// which is what used to stretch this menu across the screen.
	status := fyne.NewMenuItem(u.engine.StatusLine(), nil)
	status.Disabled = true

	open := fyne.NewMenuItem("Open Reliquary Companion", func() { u.showWindow() })
	syncNow := fyne.NewMenuItem("Sync now", func() {
		go func() {
			if !u.engine.SyncConfigured() {
				// Nothing to sync: the setup lives in the window.
				u.showWindow()
				return
			}
			u.engine.SyncRefresh()
			for _, id := range u.engine.LinkedWorldIDs() {
				u.engine.AdoptHandoff(id)
				u.engine.AutoCheckpoint(id)
			}
		}()
	})
	quit := fyne.NewMenuItem("Quit", func() { u.quit() })
	quit.IsQuit = false // routed through the busy check rather than Fyne's own quit

	menu := fyne.NewMenu(windowTitle,
		open,
		syncNow,
		fyne.NewMenuItemSeparator(),
		status,
		fyne.NewMenuItemSeparator(),
		quit,
	)
	tray.SetSystemTrayMenu(menu)
	if icon := trayIcon(); icon != nil {
		tray.SetSystemTrayIcon(icon)
	}

	// Restarting for an update has to give the tray icon up first: a
	// process that exits while it owns a notification-area entry leaves a
	// ghost behind until someone hovers over it.
	companion.ExitForRestart = func() {
		u.saveWindowSize()
		fyne.Do(func() { u.app.Quit() })
		time.Sleep(250 * time.Millisecond)
		os.Exit(0)
	}

	// The status line follows the app state; a menu the player only
	// glances at occasionally does not need to be fresher than this.
	go func() {
		for range time.Tick(5 * time.Second) {
			line := u.engine.StatusLine()
			fyne.Do(func() {
				status.Label = line
				menu.Refresh()
			})
		}
	}()
}
