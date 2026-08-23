package main

// The window's fixed furniture — header, tabs, footer — and the screens
// that are not a tab.
//
// The redesign's rule about this chrome: **sync state is reported in
// exactly one place.** It used to be said three times over — a dot and
// "up to date" in the header, a status line above the footer, and the
// scan trail's own summary — which meant three things to keep in step
// and three chances to disagree. Now it is the header's dot and one
// relative time, and nothing else on screen restates it. The scan
// trail, the tried paths and the build versions were never sync state
// at all; they are diagnostics, and they have moved behind Settings.

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/safwyls/artificer/companion"
)

// tab is a page the player can choose.
//
// Two of them, and the reason there are not five is worth writing down.
// The design also drew Activity and Conflicts tabs; the engine has
// neither an activity feed (only one overwritten LastAction string) nor
// any notion of a conflict, so shipping those tabs would mean shipping
// two empty pages. A tab that is always empty teaches people not to
// look at tabs. Settings stays a dialog rather than becoming a tab,
// because it already is one and it works.
type tab int

const (
	tabWorlds tab = iota
	tabGames
)

func (t tab) label() string {
	if t == tabGames {
		return "Games"
	}
	return "Worlds"
}

// goTab switches page. The page a player is looking at is their
// decision, so it lives on the ui and survives every redraw the
// engine's polling causes.
func (u *ui) goTab(t tab) {
	u.tab = t
	u.redraw()
}

// header says the connection once, at the top: who this machine syncs
// as, how old what you are looking at is, and the two things you might
// want to do about it.
func (u *ui) header(st companion.State) fyne.CanvasObject {
	// The window's own name is the one place the serif is unambiguously
	// right: it is large, it is set once, and it is what makes the header
	// read as the vault rather than as a toolkit's title bar.
	who := "shared world saves, synced from this machine"
	if st.Sync.Configured {
		name := st.Sync.Username
		if name == "" {
			name = "…"
		}
		who = "this machine, syncing as " + name
	}
	title := container.NewVBox(
		serifText("Reliquary Companion", colGold, szTitle),
		text(who, colMist, szCaption),
	)

	right := container.NewHBox()
	if st.Sync.Configured {
		// The single sync report: a light and a measurement. The light
		// is green when the last poll landed, ember when it did not,
		// and the time beside it is how old what you are reading is.
		light, line := colOK, freshness(st.Sync.PolledAt)
		switch {
		case st.Sync.Busy:
			// What is moving matters more than how old the last poll is.
			line = "syncing…"
		case offline(st):
			light, line = colEmber, "offline"
		}
		right.Add(dot(light))
		right.Add(container.NewPadded(monoText(line, colMist, szMicro)))
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
	} else {
		right.Add(dot(colMist))
		right.Add(container.NewPadded(monoText("not connected", colMist, szMicro)))
	}
	right.Add(iconButton("", theme.SettingsIcon(), func() { u.showSettings() }))

	return inset(container.NewBorder(nil, nil, title, right))
}

// tabBar is the page switcher: the active tab in gold with a rule under
// it, the rest in mist.
//
// Drawn rather than using container.AppTabs because that widget paints
// its own chrome — its own underline colour, its own fill — and this
// bar has to sit on the header's edge rule the way the design draws it.
func (u *ui) tabBar(st companion.State) fyne.CanvasObject {
	row := container.NewHBox()
	for _, t := range []tab{tabWorlds, tabGames} {
		active := u.tab == t
		c := colMist
		if active {
			c = colGoldHi
		}
		label := text(t.label(), c, 14)
		// The 2 px gold rule under the active tab, in the same colour
		// the label is: one accent, said twice, is what makes a tab read
		// as selected rather than merely different.
		underline := canvas.NewRectangle(colGold)
		underline.SetMinSize(fyne.NewSize(1, 2))
		var mark fyne.CanvasObject = canvas.NewRectangle(colInk)
		if active {
			mark = underline
		}
		page := t
		cell := container.NewBorder(nil, mark, nil, nil,
			container.NewPadded(container.NewHBox(label)))
		row.Add(container.NewStack(cell, newTapArea(func() { u.goTab(page) })))
	}
	return container.NewPadded(row)
}

// footer is one line of context and the way into diagnostics. The
// version strings that used to sit here are diagnostics too, and have
// gone with the rest of them.
func (u *ui) footer(st companion.State) fyne.CanvasObject {
	summary := ""
	if st.Sync.Configured {
		summary = plural(len(st.Sync.Worlds), "world", "worlds") + " · " +
			plural(len(st.Links), "linked here", "linked here")
	}
	left := monoText(summary, colMist, szMicro)
	return container.NewPadded(container.NewBorder(nil, nil, left,
		linkText("Diagnostics", func() { u.showDiagnostics() })))
}

// say puts a line in the transient strip above the footer — the desktop
// stand-in for the web UI's toasts. It stays put rather than sliding
// away over the content, because a window has room for it and a message
// that vanishes while someone reads it is a message that was not said.
//
// This is *action* feedback, not sync state: it says what a button the
// player pressed did. The header's dot remains the only report of how
// the syncing itself is going.
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

// body chooses the screen. Two of the four states the design describes
// are derived rather than chosen — a machine with no linked worlds gets
// the first-run checklist whichever tab it is on, and offline is a
// variant of Worlds rather than a page of its own, so the Worlds tab
// stays active while offline.
func (u *ui) body(st companion.State) fyne.CanvasObject {
	switch {
	case !st.Sync.Configured:
		return u.connectScreen(st)
	case len(st.Links) == 0 && u.tab == tabWorlds:
		return u.firstRunScreen(st)
	case u.tab == tabGames:
		return u.gamesPage(st)
	default:
		return u.worldsPage(st)
	}
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
		monoText(up.Version, colMist, szMicro),
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
	return callout(colGold, container.NewBorder(nil, nil, nil, container.NewCenter(apply), lines))
}

// --- first run, in two parts ---

// firstRunScreen is the checklist a connected machine with no linked
// worlds sees: what is already true, and the one thing left to do.
//
// It is keyed on "no linked worlds" rather than on "not configured",
// which is the design's definition — a machine can be perfectly
// connected and still have nothing to sync, and that machine needs
// telling what to do next rather than an empty Worlds page.
func (u *ui) firstRunScreen(st companion.State) fyne.CanvasObject {
	u.mu.Lock()
	hintsOK, hintsKnown := u.hintsOK, u.hintsKnown
	u.mu.Unlock()

	games := 0
	for _, g := range st.Discovered.Games {
		if !g.Hidden {
			games++
		}
	}

	intro := container.NewVBox(
		centered(serifText("No worlds on this machine yet", colGold, 21)),
		wrappedCenter("A world is one save folder the vault holds for your group. Link a game's save folder and Reliquary starts keeping its history.", colMist),
	)

	// Step 1 — connected. The engine knows the account name and how
	// many worlds the vault holds; it does not know an email address or
	// how many people are in the group, so those are not claimed.
	who := st.Sync.Username
	if who == "" {
		who = "this vault"
	}
	vault := plural(len(st.Sync.Worlds), "world in this vault", "worlds in this vault")

	// Step 2 — the scan. Libraries and the save-location count are both
	// facts the engine has; the trail that explains them is one click
	// away in Diagnostics rather than sprawled across this screen.
	scanSub := "Across " + plural(len(st.Discovered.Libraries), "library", "libraries") + "."
	if hintsOK {
		scanSub += " Save locations known for " + itoa(int64(hintsKnown)) + " of them."
	}

	steps := groupPanel(colEdge,
		u.step(stepMarker("✓", colOK), "Connected as "+who, vault, nil, false),
		u.step(stepMarker("✓", colOK), plural(games, "installed game found", "installed games found"), scanSub,
			quietButton("Review", func() { u.goTab(tabGames) }), false),
		u.step(stepMarker("3", colGold), "Link a game to make your first world",
			"Pick the game and Reliquary suggests the save folder. You confirm it.",
			primaryButton("Choose a game", func() { u.goTab(tabGames) }), true),
	)

	col := container.NewVBox(intro, steps)
	// Only offered when there is actually a world to join: an invitation
	// to join nothing is worse than no invitation.
	if u.joinableWorlds(st) > 0 {
		col.Add(centered(container.NewHBox(
			text("Someone in your group already made worlds?", colMist, szCaption),
			linkText("Join one instead", func() { u.showLinkGame(companion.Game{}, true) }),
		)))
	}
	return container.NewPadded(container.NewCenter(container.NewGridWrap(fyne.NewSize(620, col.MinSize().Height), col)))
}

// joinableWorlds is how many worlds the vault holds that this machine
// has not linked — the population "Join one instead" would draw from.
func (u *ui) joinableWorlds(st companion.State) int {
	n := 0
	for _, w := range st.Sync.Worlds {
		taken := false
		for _, l := range st.Links {
			if l.WorldID == w.World.ID {
				taken = true
			}
		}
		if !taken {
			n++
		}
	}
	return n
}

// step is one row of the first-run checklist.
func (u *ui) step(marker fyne.CanvasObject, title, sub string, action fyne.CanvasObject, current bool) fyne.CanvasObject {
	lines := container.NewVBox(text(title, colParchment, 14))
	if sub != "" {
		lines.Add(text(sub, colMist, szCaption))
	}
	row := container.NewBorder(nil, nil, container.NewCenter(marker), nil, lines)
	if action != nil {
		row = container.NewBorder(nil, nil, container.NewCenter(marker),
			container.NewCenter(action), lines)
	}
	body := container.NewPadded(row)
	if !current {
		return body
	}
	// The step still to do sits on the well, so the eye lands on it
	// without anything having to say "you are here".
	bg := canvas.NewRectangle(colWell)
	return container.NewStack(bg, body)
}

// connectScreen is what an unconfigured companion shows. It is a
// different thing from the first-run checklist above: until this app can
// reach a vault, there is no account to greet and no worlds to count,
// and the two things that have to be true — it can reach a vault, and it
// can find your games — are the whole screen.
func (u *ui) connectScreen(st companion.State) fyne.CanvasObject {
	url := widget.NewEntry()
	url.SetPlaceHolder("https://vault.example.com")
	url.SetText(st.Config.ServerURL)
	token := widget.NewPasswordEntry()
	token.SetPlaceHolder("paste the token from the service's page")

	status := monoText(statusWord(st), colMist, szMicro)

	// The one place a full-width primary is right: this is a narrow card
	// in a two-column layout, and a button spanning it reads as the
	// card's own call to action. Everywhere else a primary is compact.
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
		text("Steam folder (blank = auto-detect)", colMist, szCaption),
		container.NewBorder(nil, nil, nil, u.folderPickerButton(steam), steam),
		actions(quietButton("Save folder & rescan", func() { u.saveSteamDir(steam.Text) })),
		// The trail stays on this one screen, because here "no games
		// found" has nowhere else to explain itself yet.
		inset(u.scanTrail(st, true)),
	))

	return container.NewPadded(container.NewGridWithColumns(2, connectCard, gamesCard))
}

// centered puts one object in the middle of whatever width it is given.
func centered(o fyne.CanvasObject) fyne.CanvasObject {
	return container.NewCenter(o)
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
