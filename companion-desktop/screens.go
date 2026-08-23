package main

// The window's fixed furniture and its two top-level screens.
//
// web/companion/src/components/HeaderBar.tsx and App.tsx are the
// behavioural spec: the same header, the same footer, the same choice
// between a first-run screen and the worlds-then-shelf body, and the
// same four kinds of error surface.

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/safwyls/artificer/companion"
)

// header says the connection once, at the top: whether the vault is
// reachable, as whom, how old what you are looking at is, and the two
// things you might want to do about it.
func (u *ui) header(st companion.State) fyne.CanvasObject {
	// The window's own name is the one place the serif is unambiguously
	// right: it is large, it is set once, and it is what makes the header
	// read as the vault rather than as a toolkit's title bar.
	title := container.NewVBox(
		serifText("Reliquary Companion", colGold, szTitle),
		text("shared world saves, synced from this machine", colMist, szCaption),
	)

	light := colMist
	who := text("Not connected", colMist, szCaption)
	switch {
	case st.Sync.LastError != "":
		light = colEmber
	case st.Sync.Configured:
		light = colOK
	}
	if st.Sync.Configured {
		name := st.Sync.Username
		if name == "" {
			name = "…"
		}
		who = text("Connected as "+name, colParchment, szCaption)
	}

	right := container.NewHBox(dot(light), who)
	if st.Sync.Configured {
		// The freshness line is mono because it is a measurement, and it
		// yields to the transfer while one is running: what is moving
		// matters more than how old the last poll is.
		line := freshness(st.Sync.PolledAt)
		if st.Sync.Busy {
			line = "transfer in progress…"
		}
		// Padded so the measurement does not run straight into the
		// account name beside it — an HBox packs its children edge to
		// edge, and "Connected as safwyls" and "up to date" read as one
		// run-on line without this.
		right.Add(container.NewPadded(monoText(line, colMist, szCaption)))
		right.Add(iconButton("Sync now", theme.ViewRefreshIcon(), func() {
			go func() {
				worlds, err := u.engine.SyncNow()
				if err != nil {
					u.say(err.Error(), true)
					return
				}
				u.say("synced — "+plural(worlds, "world", "worlds")+" on the service", false)
			}()
		}))
	}
	right.Add(iconButton("", theme.SettingsIcon(), func() { u.showSettings() }))

	bar := container.NewBorder(nil, nil, title, right)
	rows := container.NewVBox(bar)
	// The standing error band: a poll that failed says so across the
	// whole header, not in a corner.
	if st.Sync.LastError != "" {
		rows.Add(wrapped(st.Sync.LastError, colEmber))
	}
	return inset(rows)
}

// footer names both builds side by side: which companion, and which
// service it is talking to. A save-sync report that names one half names
// nothing.
func (u *ui) footer(st companion.State) fyne.CanvasObject {
	versions := "companion " + st.Version
	switch {
	case st.Sync.ServerVersion != "":
		versions += " · service " + st.Sync.ServerVersion
	case st.Sync.Configured:
		versions += " · service version unknown"
	}
	row := container.NewHBox(monoText(versions, colMist, szCaption))
	if st.Sync.LastAction != "" {
		row = container.NewBorder(nil, nil, row,
			monoText("last action: "+st.Sync.LastAction, colMist, szCaption))
	}
	return container.NewPadded(row)
}

// say puts a line in the transient strip above the footer — the desktop
// stand-in for the web UI's toasts. It stays put rather than sliding
// away over the content, because a window has room for it and a message
// that vanishes while someone reads it is a message that was not said.
func (u *ui) say(msg string, bad bool) {
	c := colMist
	if bad {
		c = colEmber
	}
	fyne.Do(func() {
		u.statusBox.Objects = []fyne.CanvasObject{
			container.NewPadded(wrapped(msg, c)),
		}
		u.statusBox.Refresh()
	})
}

// run performs an action and reports what came of it, then lets the
// engine's own nudge redraw. Every verb in this app goes through here,
// so a failure is never silent and never closes what the player was
// looking at.
func (u *ui) run(fn func() error, okMsg string) {
	go func() {
		if err := fn(); err != nil {
			u.say(err.Error(), true)
			return
		}
		if okMsg != "" {
			u.say(okMsg, false)
		}
	}()
}

// mainScreen is the connected body: the update offer, the worlds this
// machine has linked, and the shelf of installed games.
func (u *ui) mainScreen(st companion.State) fyne.CanvasObject {
	rows := container.NewVBox()
	if banner := u.updateBanner(st); banner != nil {
		rows.Add(banner)
	}

	rows.Add(sectionHeader("Your worlds", ""))
	if len(st.Links) == 0 {
		rows.Add(italicText(
			"Nothing linked yet — link an installed game below, or ask whoever runs your sync service which world to join.", colMist, szCaption))
	}
	for _, link := range st.Links {
		rows.Add(u.worldRow(st, link))
	}

	rows.Add(widget.NewSeparator())
	rows.Add(u.shelf(st))
	return rows
}

// updateBanner offers a new build rather than imposing one. It says "a
// different build" rather than "a newer version" on purpose: every
// release is stamped with a commit SHA, SHAs have no order, and the
// honest question is whether the release ships the build you are
// running.
func (u *ui) updateBanner(st companion.State) fyne.CanvasObject {
	up := st.Update
	if !up.Available {
		return nil
	}
	lines := container.NewVBox(
		text("A different companion build is available.", colParchment, szBody),
		monoText(up.Version, colMist, szCaption),
	)
	if !up.Supported {
		// Offering a button that cannot work is worse than saying why.
		lines.Add(italicText(up.Why, colMist, szCaption))
		return callout(colGold, lines)
	}
	lines.Add(text("It replaces this one and restarts.", colMist, szCaption))
	apply := primaryButton("Update now", func() {
		u.say("updating — the companion will restart", false)
		go func() {
			if err := u.engine.ApplyUpdate(u.ctx()); err != nil {
				u.say(err.Error(), true)
				return
			}
			// The replacement is in place; hand the tray back before the
			// process ends, or a ghost icon is left in the notification
			// area until someone hovers over it.
			if err := u.engine.RestartAfterUpdate(); err != nil {
				u.say("updated, but restarting failed: "+err.Error(), true)
			}
		}()
	})
	if up.Applying {
		apply.Disable()
	}
	return callout(colGold, container.NewBorder(nil, nil, nil, apply, lines))
}

// firstRunScreen is a deliberate state, not an empty page with forms at
// the bottom. Until this companion is connected, the two things that
// have to be true — it can reach a vault, and it can find your games —
// are the whole screen, side by side, with the scan trail already
// visible so "no games found" always names its own cause.
func (u *ui) firstRunScreen(st companion.State) fyne.CanvasObject {
	url := widget.NewEntry()
	url.SetPlaceHolder("https://vault.example.com")
	url.SetText(st.Config.ServerURL)
	token := widget.NewPasswordEntry()
	token.SetPlaceHolder("paste the token from the service's page")

	status := text(statusWord(st), colMist, 12)
	status.TextStyle = fyne.TextStyle{Monospace: true}

	// The one place a full-width primary is right: this is a narrow card
	// in a two-column first-run layout, and a button spanning it reads as
	// the card's own call to action. Everywhere else a primary is compact
	// and goes through actions().
	connect := wideButton("Save & connect", func() {
		serverURL := url.Text
		go func() {
			fyne.Do(func() {
				status.Text = "connecting…"
				status.Refresh()
			})
			// A completed connection is proven with a status poll: a
			// typo'd token should fail here, not silently every minute
			// forever. An empty token keeps the saved one.
			err := u.engine.SetConfig(companion.ConfigUpdate{ServerURL: &serverURL, Token: token.Text})
			fyne.Do(func() {
				token.SetText("")
				if err != nil {
					status.Text = "not connected"
					status.Color = colEmber
				} else {
					status.Text = "connected"
					status.Color = colOK
				}
				status.Refresh()
			})
			if err != nil {
				u.say(err.Error(), true)
				return
			}
			u.say("connected", false)
		}()
	})

	connectCard := panelCard(container.NewVBox(
		serifBold("Connect to your vault", colGold, szSubhead),
		wrapped("Nothing leaves this machine until you connect. Ask whoever runs your group's sync service for the address, and mint your token on its page.", colMist),
		text("Save-sync service URL", colMist, szCaption),
		url,
		text("Your sync token", colMist, szCaption),
		token,
		connect,
		container.NewPadded(status),
	))

	steam := widget.NewEntry()
	steam.SetPlaceHolder(`e.g. D:\SteamLibrary or D:\Steam\steamapps\common`)
	steam.TextStyle = fyne.TextStyle{Monospace: true}
	if len(st.Config.SteamDirs) > 0 {
		steam.SetText(st.Config.SteamDirs[0])
	}

	gamesCard := panelCard(container.NewVBox(
		serifBold("Finding your games", colGold, szSubhead),
		wrapped("Steam is detected automatically — the registry, then the usual install paths. Set a folder only if the scan misses a library.", colMist),
		text("Steam folder (blank = auto-detect)", colMist, 11),
		container.NewBorder(nil, nil, nil, u.folderPickerButton(steam), steam),
		actions(quietButton("Save folder & rescan", func() { u.saveSteamDir(steam.Text) })),
		inset(u.scanTrail(st, true)),
		italicText(`"No games found" always names its own cause here.`, colMist, szCaption),
	))

	return container.NewPadded(container.NewGridWithColumns(2, connectCard, gamesCard))
}

func statusWord(st companion.State) string {
	if st.Config.TokenSet {
		return "token saved"
	}
	return "not configured"
}

// saveSteamDir writes the discovery override and rescans. Both screens
// that offer it share this, so the two cannot drift.
func (u *ui) saveSteamDir(dir string) {
	dirs := []string{}
	if trimmed := trim(dir); trimmed != "" {
		dirs = []string{trimmed}
	}
	msg := "override cleared — rescanned"
	if len(dirs) > 0 {
		msg = "folder saved — rescanned"
	}
	u.run(func() error { return u.engine.SetConfig(companion.ConfigUpdate{SteamDirs: &dirs}) }, msg)
}
