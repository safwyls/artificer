package main

// The Worlds page: the whole window, grouped by what you can do with
// each world.
//
// The redesign's structure lives here. Worlds used to be one small card
// above an installed-games grid that took most of the screen; now the
// games are their own tab and this is the page. Three things follow from
// that, and each is load-bearing:
//
//   - **Grouping replaces status prose.** A world's section says which
//     custody state it is in, so the row does not have to. "nobody holds
//     this world" under a heading that already says "Free to take" is a
//     sentence nobody needed to read.
//   - **One primary action per row**, one quiet second, and everything
//     rare or destructive behind a 3-dot menu. The old row offered four
//     equal-weight buttons with no way to tell "Check out" from "Check
//     out & play".
//   - **Chip and primary come from one custodyOf call.** They are read
//     off the same custodyInfo value, so a row that says Free cannot
//     also offer Check in. This is the design system's rule and the main
//     invariant this file exists to keep.

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/safwyls/artificer/companion"
)

// coverThumbWidth is the world row's cover: 54 wide, and 72 tall at the
// design's 3:4, pinned by coverTile so no row width can squash it.
const coverThumbWidth = 54

// custodyChip is the state in a word, in the vault's own colours.
//
// Gold means *you* have it; grey means someone else does. That
// distinction is the reason a held-by-someone-else chip is not simply
// the gold one with a different name: across the room, gold on this page
// means "mine".
func custodyChip(c custodyInfo) fyne.CanvasObject {
	switch c.State {
	case custodyFree:
		return chip("Free", colOK, colOK, colFillFree)
	case custodyExpired:
		return chip("Hold expired", colEmber, colEmber, colFillExpired)
	case custodyGone:
		return chip("Not on the service", colEmber, colEmber, colFillExpired)
	case custodyMine:
		return chip("Yours", colGoldHi, colGold, colFillHeld)
	case custodyFetching:
		return chip("Yours — fetching", colGoldHi, colGold, colFillHeld)
	default:
		return chip("Held", colMist, colMist, colWell)
	}
}

// worldsPage is the whole Worlds tab: the groups that have worlds in
// them, in custody order, and the one pointer to the games library.
func (u *ui) worldsPage(st companion.State) fyne.CanvasObject {
	now := time.Now()
	groups := worldsOf(st, now)
	off := offline(st)

	page := container.NewVBox()
	if off {
		page.Add(u.offlineBanner(st))
	}
	if banner := u.updateBanner(st); banner != nil {
		page.Add(banner)
	}

	// Yours first, always: it is the world you are most likely here
	// about, and the gold border makes it findable without reading.
	if rows := groups[groupMine]; len(rows) > 0 {
		label := "Checked out to you"
		sub := "locked to this machine until you check it in"
		if off {
			// The whole point of the offline state, said in the heading.
			label, sub = "Checked out to you — playable offline", ""
		}
		page.Add(u.group(st, sectionLabel(label, colGold, sub), dim(colGold, 0x73), rows, now, off))
	}

	// Offline, custody cannot be confirmed, so the worlds this machine
	// does *not* hold are hidden rather than shown with actions that
	// might collide with someone else. They stay one tap away.
	hideOthers := off && !u.showOfflineWorlds
	others := len(groups[groupFree]) + len(groups[groupHeld]) + len(groups[groupGone])
	if hideOthers && others > 0 {
		page.Add(quietWell(container.NewVBox(
			wrapped("The other "+plural(others, "world is", "worlds are")+" hidden while offline — custody can't be confirmed, so checking one out could collide with someone else.", colMist),
			linkText("Show them read-only", func() {
				u.showOfflineWorlds = true
				u.redraw()
			}),
		)))
	} else {
		if rows := groups[groupFree]; len(rows) > 0 {
			page.Add(u.group(st, sectionLabel("Free to take · "+itoa(int64(len(rows))), colMist, ""), colEdge, rows, now, off))
		}
		if rows := groups[groupHeld]; len(rows) > 0 {
			page.Add(u.group(st, sectionLabel("Held by someone else · "+itoa(int64(len(rows))), colMist, ""), colEdge, rows, now, off))
		}
		if rows := groups[groupGone]; len(rows) > 0 {
			page.Add(u.group(st,
				sectionLabel("No longer on the service · "+itoa(int64(len(rows))), colEmber, "unlink these, or ask whoever runs the vault"),
				colEdge, rows, now, off))
		}
	}

	if len(st.Links) == 0 {
		page.Add(quietWell(wrapped(
			"Nothing linked yet — open the games library and link an installed game, or ask whoever runs your vault which world to join.", colMist)))
	}

	page.Add(u.libraryFooter(st))
	return page
}

// group is one custody section: its label above a single bordered panel
// whose rows are divided by edge lines.
func (u *ui) group(st companion.State, label fyne.CanvasObject, border color.Color, rows []worldRowData, now time.Time, off bool) fyne.CanvasObject {
	built := make([]fyne.CanvasObject, 0, len(rows))
	for _, d := range rows {
		built = append(built, u.worldRow(st, d, now, off))
	}
	return container.NewVBox(label, groupPanel(border, built...))
}

// worldRow is one world: cover, name and game, chip and line, the mono
// head meta, and its actions.
func (u *ui) worldRow(st companion.State, d worldRowData, now time.Time, off bool) fyne.CanvasObject {
	game := d.gameTitle()

	// A held-by-someone-else cover is dimmed: the row is information,
	// not an invitation.
	dimmed := d.custody.group() == groupHeld
	thumb := coverTile(u.cover(d.link.AppID, game, u.redraw), game, coverThumbWidth, dimmed)

	head := container.NewHBox(serifBold(d.name(), colParchment, szSubhead))
	if game != "" {
		// The game tag is rune-coloured everywhere in this app; it is a
		// label the engine supplied, never something the UI decided.
		head.Add(text(game, colRune, szMicro))
	}

	line := container.NewHBox(
		custodyChip(d.custody),
		text(custodyLine(d.custody, d.link, off, now), colMist, szCaption),
	)
	// Hold pressure is shown only when it is pressure: a countdown on
	// somebody else's hold appears inside the last three hours and not
	// before, so it means "soon" rather than being furniture.
	if d.custody.group() == groupHeld && d.custody.urgent(now) {
		line.Add(badge(fmtDuration(d.custody.holdLeft(now))+" left", colEmber))
	}

	body := container.NewVBox(head, line)

	right := container.NewHBox()
	if meta := headMeta(d.world, now); meta != "" {
		right.Add(monoText(meta, colMist, szMicro))
	}
	right.Add(u.rowActions(st, d, off))

	// The name column is left-aligned against the cover, not centred in
	// whatever width is left over: a Border's centre child fills the
	// gap, and centring inside it pushed every world's name into the
	// middle of the row with a hand's width of nothing beside its cover.
	// VBox keeps it hard against the cover at every window width.
	// Inside an HBox a child is given the row's full height but only its
	// own minimum width, so NewCenter here centres the name block
	// vertically against the 72 px cover while keeping it hard against
	// that cover horizontally. Handing it to the Border as the centre
	// child instead — which the first cut did — let it fill the leftover
	// width and centred every world's name in the middle of the row,
	// a hand's width away from its own cover.
	return container.NewPadded(container.NewBorder(nil, nil,
		container.NewHBox(thumb, container.NewCenter(body)), right))
}

// rowActions is the verb matrix, reduced to the design's shape: one
// primary, one quiet, and a 3-dot menu.
//
// Every branch reads d.custody and nothing else — which is what makes
// "the chip and the primary cannot disagree" true rather than merely
// intended.
func (u *ui) rowActions(st companion.State, d worldRowData, off bool) fyne.CanvasObject {
	row := container.NewHBox()
	link, world := d.link, d.world

	// "& play" only when both halves are true: the setting is on, and
	// this world has something to start. A world linked by hand from a
	// folder has no app id, so the button goes back to promising the
	// save alone.
	willPlay := st.Config.LaunchOnCheckout && launchable(link)

	switch d.custody.State {
	case custodyMine:
		// Play is the primary when there is something to play. When
		// there is not, checking in is the only verb this row has, so it
		// is promoted rather than left as a quiet button beside nothing.
		if launchable(link) {
			row.Add(iconPrimary("Play", theme.MediaPlayIcon(), func() {
				u.run(func() error { return u.engine.Launch(link.WorldID) }, "starting the game")
			}))
			row.Add(quietButton("Check in", func() { u.checkin(link) }))
		} else {
			row.Add(primaryButton("Check in", func() { u.checkin(link) }))
		}

	case custodyFree:
		if off {
			// Custody cannot be taken without the vault's word on it.
			break
		}
		label := "Check out"
		if willPlay {
			label = "Check out & play"
		}
		row.Add(iconPrimary(label, theme.MediaPlayIcon(), func() { u.checkout(link, false, true) }))
		if willPlay {
			// The two labels are deliberately explicit: one launches the
			// game, one only takes custody.
			row.Add(quietButton("Check out only", func() { u.checkout(link, false, false) }))
		}

	case custodyExpired:
		if off {
			break
		}
		row.Add(dangerButton("Take over expired hold", func() {
			u.confirm(
				"Take over the expired hold?",
				"The old holder's late check-in is kept and flagged, not lost.",
				"Take over",
				func() { u.checkout(link, true, true) },
			)
		}))
	}

	// Queueing behind the current holder is the one thing a held world
	// can honestly offer, and only while nobody is already queued.
	if !off && (d.custody.State == custodyHeld || d.custody.State == custodyExpired) && d.custody.NextClaim == "" {
		row.Add(quietButton("Ask to be next", func() {
			u.run(func() error { return u.engine.Claim(link.WorldID) },
				"you're next — the world downloads automatically when it frees up")
		}))
	}

	row.Add(u.overflow(link, world))
	return row
}

// overflow is the 3-dot menu: the rare verbs and the debug facts.
//
// The raw save path lives in here rather than in a well on the row,
// because it is a thing you look up when something is wrong, not a
// thing you read every time you check a world out.
func (u *ui) overflow(link companion.WorldLink, world *companion.World) fyne.CanvasObject {
	var btn *vaultButton
	btn = iconButton("", theme.MoreVerticalIcon(), func() {
		items := []*fyne.MenuItem{
			fyne.NewMenuItem("Open save folder", func() {
				u.run(func() error { return companion.OpenURI(link.Dir) }, "")
			}),
			fyne.NewMenuItem("Copy save path", func() {
				u.app.Clipboard().SetContent(link.Dir)
				u.say("save path copied", false)
			}),
			fyne.NewMenuItem("Rename…", func() { u.showRenameWorld(link, world) }),
			fyne.NewMenuItem("Edit link…", func() { u.showEditWorld(link, world) }),
			fyne.NewMenuItemSeparator(),
		}
		// The confirm dialog is what marks this as destructive — Fyne's
		// menu item has no danger styling to set.
		items = append(items, fyne.NewMenuItem("Unlink", func() {
			u.confirm(
				"Unlink this world from its folder?",
				"This machine stops syncing it. The world stays on the vault and the save folder is left exactly as it is — nothing is deleted, and you can link it again later.",
				"Unlink",
				func() { u.run(func() error { return u.engine.Unlink(link.WorldID) }, "unlinked") },
			)
		}))

		menu := widget.NewPopUpMenu(fyne.NewMenu("", items...), u.win.Canvas())
		at := fyne.CurrentApp().Driver().AbsolutePositionForObject(btn)
		// Right-aligned to the button. The overflow is the last thing in
		// the row, so a menu hung from its left edge runs off the side
		// of the window; hung from its right edge it opens inwards.
		x := at.X + btn.Size().Width - menu.MinSize().Width
		if x < 0 {
			x = 0
		}
		menu.ShowAtPosition(fyne.NewPos(x, at.Y+btn.Size().Height+2))
	})
	return btn
}

// libraryFooter is the only pointer from Worlds to the Games tab, and
// the only place the installed-game count is mentioned on this page.
func (u *ui) libraryFooter(st companion.State) fyne.CanvasObject {
	games, linked := 0, 0
	for _, g := range st.Discovered.Games {
		if g.Hidden {
			continue
		}
		games++
		if linkFor(g, st.Links) != nil {
			linked++
		}
	}
	return quietWell(container.NewHBox(
		text(plural(games, "game", "games")+" installed on this machine, "+
			itoa(int64(linked))+" linked to a world.", colMist, szCaption),
		linkText("Open the games library", func() { u.goTab(tabGames) }),
	))
}

// offlineBanner says the vault is unreachable and that it does not
// stop you playing what you already hold.
//
// The engine has no queue of unsent work to list, so this says what is
// true — the hold stands, work is kept locally — and does not invent a
// manifest of it.
func (u *ui) offlineBanner(st companion.State) fyne.CanvasObject {
	// The banner says the state; it does not print the transport error.
	// That string is a diagnostic — it is in Diagnostics, under Vault —
	// and putting it here made the window report the same failure twice
	// over, once as prose and once as a stack of Go error text.
	lines := container.NewVBox(
		text("Working offline — the vault is unreachable", colParchment, szBody),
		wrapped("Keep playing the world you already hold. The companion keeps retrying on its own, and picks up where it left off when the vault answers again.", colMist),
	)
	retry := quietButton("Retry now", func() {
		go func() {
			if _, err := u.engine.SyncNow(); err != nil {
				u.say(err.Error(), true)
				return
			}
			u.say("the vault answered — back online", false)
		}()
	})
	return callout(colEmber, container.NewBorder(nil, nil, nil, container.NewCenter(retry), lines))
}

// checkin is its own method because two rows can offer it — the one
// with something to play and the one without.
func (u *ui) checkin(link companion.WorldLink) {
	u.run(func() error { return u.engine.Checkin(link.WorldID) }, "checked in — the world is free")
}

// checkout is two halves of one intention: fetch the save, then play.
// The engine does them in that order and reports both, because a save on
// disk with a game that would not start is a real outcome — the custody
// half succeeded, and the player needs to know the other half did not
// without being told the whole thing failed.
func (u *ui) checkout(link companion.WorldLink, takeover, play bool) {
	go func() {
		// So the hold that lands is recognised as this window's doing
		// rather than a queued claim coming through on its own.
		u.selfCheckout.Store(true)
		defer u.selfCheckout.Store(false)

		out, err := u.engine.Checkout(link.WorldID, takeover, play)
		switch {
		case err != nil:
			u.say(err.Error(), true)
		case out.LaunchError != nil:
			u.say("checked out, but the game did not start: "+out.LaunchError.Error(), true)
		case out.Launched:
			u.say("checked out — the save is on this machine, and the game is starting", false)
		default:
			u.say("checked out — the save is on this machine", false)
		}
	}()
}
