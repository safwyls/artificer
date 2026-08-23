package main

// One linked world per row: what it is, who holds it, the folder on this
// machine, and the actions its custody state calls for.
//
// web/companion/src/components/WorldRow.tsx is the spec, and the verb
// matrix below is ported from it rule for rule — including the one that
// looks like an omission: a hold belonging to this account but to a
// *different* session ("fetching") offers nothing at all, because the
// download is still on its way here and every verb would act on a save
// that has not arrived.

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/safwyls/artificer/companion"
)

// custodyChip is the state in a word, in the vault's own colours: free
// green, held gold, an expired hold ember. Six states, and each has to
// be distinguishable at a glance across the room — which is why "yours
// — fetching" wears the held colours rather than the free ones: it is
// not yours to play yet.
func custodyChip(c custody) fyne.CanvasObject {
	switch c {
	case custodyFree:
		return chip("Free", colOK, colOK, colFillFree)
	case custodyExpired:
		return chip("Hold expired", colEmber, colEmber, colFillExpired)
	case custodyGone:
		return chip("Not on the service", colEmber, colEmber, colFillExpired)
	case custodyMine:
		return chip("You hold this world", colGoldHi, colGold, colFillHeld)
	case custodyFetching:
		return chip("Yours — fetching", colGoldHi, colGold, colFillHeld)
	default:
		return chip("Held", colGoldHi, colGold, colFillHeld)
	}
}

func (u *ui) worldRow(st companion.State, link companion.WorldLink) fyne.CanvasObject {
	world := worldFor(st, link.WorldID)
	me := st.Sync.Username
	c := custodyOf(link, world, me, st.Sync.Configured)

	title := link.GameTitle
	if title == "" && world != nil {
		title = world.World.Name
	}
	name := "world #" + itoa(link.WorldID)
	if world != nil {
		name = world.World.Name
	}

	thumb := coverTile(u.cover(link.AppID, title, u.redraw), title, 56, false)

	head := container.NewHBox(boldText(name, colParchment, 16))
	if link.GameTitle != "" {
		// The game tag is rune-coloured everywhere in this app; it is a
		// label the engine supplied, never something the UI decided.
		head.Add(text(link.GameTitle, colRune, 12))
	}

	lines := container.NewVBox(
		container.NewBorder(nil, nil, head, custodyChip(c)),
		text(custodyLine(c, link, world, me), colMist, 13),
		// The folder, in full: the one thing in this row that is about
		// this machine rather than the world, and the thing a player
		// checks when a save goes to the wrong place.
		monoText(link.Dir, colMist, 11),
		u.worldActions(st, link, world, c),
	)
	return panelCard(container.NewBorder(nil, nil, thumb, nil, lines))
}

// worldActions is the verb matrix. Each state offers exactly what it can
// honestly do, and nothing it cannot.
func (u *ui) worldActions(st companion.State, link companion.WorldLink, world *companion.World, c custody) fyne.CanvasObject {
	row := container.NewHBox()

	// "& play" only when both halves are true: the setting is on, and
	// this world has something to start. A world linked by hand from a
	// folder has no app id, so the button goes back to promising the
	// save alone.
	willPlay := st.Config.LaunchOnCheckout && launchable(link)

	switch c {
	case custodyFree:
		label := "Check out & host"
		if willPlay {
			label = "Check out & play"
		}
		row.Add(primaryButton(label, func() { u.checkout(link, false, true) }))
		if willPlay {
			// The save alone, no launch — for taking custody without
			// starting anything, regardless of the setting.
			row.Add(widget.NewButton("Check out", func() { u.checkout(link, false, false) }))
		}

	case custodyMine:
		row.Add(primaryButton("Check in", func() {
			u.run(func() error { return u.engine.Checkin(link.WorldID) }, "checked in — the world is free")
		}))
		// A checkpoint never moves the head; the service only keeps them
		// for worlds that asked for them.
		if world != nil && world.World.Checkpoints {
			row.Add(widget.NewButton("Checkpoint now", func() {
				u.run(func() error { return u.engine.Checkpoint(link.WorldID) }, "checkpoint pushed")
			}))
		}
		row.Add(widget.NewButton("Renew hold", func() {
			u.run(func() error { return u.engine.Renew(link.WorldID) }, "hold renewed")
		}))
		// The world is already here; this is for coming back to it later
		// in the same hold, without checking anything out.
		if launchable(link) {
			row.Add(widget.NewButton("Play", func() {
				u.run(func() error { return u.engine.Launch(link.WorldID) }, "starting the game")
			}))
		}

	case custodyExpired:
		row.Add(dangerButton("Take over expired hold", func() {
			u.confirm(
				"Take over the expired hold?",
				"The old holder's late check-in is kept and flagged, not lost.",
				"Take over",
				func() { u.checkout(link, true, true) },
			)
		}))
	}

	if (c == custodyHeld || c == custodyExpired) && (world == nil || world.ClaimedBy == "") {
		row.Add(widget.NewButton("Claim next", func() {
			u.run(func() error { return u.engine.Claim(link.WorldID) },
				"you're next — the world downloads automatically when it frees up")
		}))
	}

	// Edit and Unlink are offered in every state, including "gone": a
	// world that has left the service is exactly the one a player needs
	// to be able to unlink.
	row.Add(widget.NewButton("Edit", func() { u.showEditWorld(link, world) }))
	row.Add(widget.NewButton("Unlink", func() {
		u.confirm("Unlink this world from its folder?", "Nothing is deleted.", "Unlink", func() {
			u.run(func() error { return u.engine.Unlink(link.WorldID) }, "unlinked")
		})
	}))
	return row
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
