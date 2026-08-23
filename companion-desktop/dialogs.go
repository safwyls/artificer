package main

// Every modal in the app.
//
// Specs: LinkGameDialog.tsx, LinkedGameDialog.tsx, EditWorldDialog.tsx,
// SettingsDialog.tsx and ConfirmDialog.tsx. One deliberate deletion —
// the in-app FolderBrowser and the /api/browse UI it drove are gone,
// replaced by the OS folder picker. That route still exists and still
// works for the browser page; this window simply has a better answer,
// and it is the one place the native app removes web UI rather than
// translating it.
//
// Everything the player types lives in the widgets of one dialog, so the
// engine's change nudges can rebuild the shelf underneath as often as
// they like without ever touching a half-filled form — the same property
// the web dialogs rely on under their five-second poll.

import (
	"context"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/safwyls/artificer/companion"
)

// Dialog sizing is relative to the window, not fixed.
//
// The first cut resized every dialog to a flat 620×620. On a window
// bigger than that it was fine; on anything smaller Fyne clamped it to
// the window's own bounds, so the settings sheet stopped being a card
// and became a full-width panel sliding over the app, with its lower
// controls — "Check for update", "Close" — pushed off the bottom edge
// where nothing could reach them. A modal has to be a card *on* the
// window at every window size, which means measuring the window.
const (
	// dialogMaxW and dialogMaxH are as large as a dialog is ever worth
	// being: past this it stops reading as a card and starts reading as
	// a second window.
	dialogMaxW = 620
	dialogMaxH = 640
	// dialogMinW and dialogMinH are the floor. Below this the content
	// cannot be laid out at all, and a scrollbar is a better answer than
	// a squeezed one.
	dialogMinW = 320
	dialogMinH = 240
	// dialogInset is how much of the window a dialog leaves showing
	// around itself, as a fraction. Some visible backdrop is what makes
	// it read as modal rather than as a new screen.
	dialogInset = 0.88
)

// dialogSize is the size for a dialog on the window as it is right now:
// a fraction of the window, capped both ways.
func (u *ui) dialogSize() fyne.Size {
	win := u.win.Canvas().Size()
	w, h := win.Width*dialogInset, win.Height*dialogInset
	if w > dialogMaxW {
		w = dialogMaxW
	}
	if h > dialogMaxH {
		h = dialogMaxH
	}
	if w < dialogMinW {
		w = dialogMinW
	}
	if h < dialogMinH {
		h = dialogMinH
	}
	return fyne.NewSize(w, h)
}

// confirmSize is the same for the one-question dialogs, which need much
// less room and should not be blown up to the full cap on a big window.
func (u *ui) confirmSize() fyne.Size {
	s := u.dialogSize()
	if s.Width > 460 {
		s.Width = 460
	}
	if s.Height > 260 {
		s.Height = 260
	}
	return s
}

// dialogBody is the wrapper every dialog's content goes through: a
// scroll container, so that whatever the content's natural height is, it
// can always be reached inside the size the dialog was given. Without
// it a dialog taller than its box simply clips, and the buttons at the
// bottom of a form are exactly what gets clipped.
func dialogBody(content fyne.CanvasObject) fyne.CanvasObject {
	return container.NewVScroll(container.NewPadded(content))
}

// show puts a dialog up, replacing whatever was there. Two stacked
// modals is never what anyone meant.
func (u *ui) show(d interface {
	Show()
	Hide()
	Resize(fyne.Size)
}) {
	if u.openDialog != nil {
		u.openDialog.Hide()
	}
	u.openDialog = d
	d.Resize(u.dialogSize())
	d.Show()
}

func (u *ui) dismiss() {
	if u.openDialog != nil {
		u.openDialog.Hide()
		u.openDialog = nil
	}
}

// confirm is the one-question dialog the destructive verbs stand behind:
// a takeover, an unlink, quitting mid-transfer.
func (u *ui) confirm(title, body, confirmLabel string, onConfirm func()) {
	fyne.Do(func() {
		// Built without Fyne's own button row so the two verbs wear the
		// vault's chrome rather than a solid primary-coloured slab, and
		// so the question itself sits in a scroll container — a long
		// refusal must wrap and stay reachable, not push the buttons out
		// of the box.
		var d *dialog.CustomDialog
		buttons := actions(
			dangerButton(confirmLabel, func() {
				d.Hide()
				onConfirm()
			}),
			quietButton("Cancel", func() { d.Hide() }),
		)
		content := container.NewBorder(nil, container.NewPadded(buttons), nil, nil,
			dialogBody(wrapped(body, colParchment)))
		d = dialog.NewCustomWithoutButtons(title, content, u.win)
		d.Resize(u.confirmSize())
		d.Show()
	})
}

// fatal is the startup-failure surface: something the app cannot work
// around, said in a box, with nothing to do but leave.
func (u *ui) fatal(msg string) {
	fyne.Do(func() {
		d := dialog.NewError(errString(msg), u.win)
		d.SetOnClosed(func() { u.app.Quit() })
		d.Show()
	})
}

// --- link a game ---

// showLinkGame is the form that ties a save folder here to a world on
// the service — either joining one that exists, or creating a new one.
// byHand is the path for a folder discovery never found: a blank game,
// no candidates, no hide button.
func (u *ui) showLinkGame(game companion.Game, byHand bool) {
	st := u.snapshot()

	dir := widget.NewEntry()
	dir.TextStyle = fyne.TextStyle{Monospace: true}
	dir.SetPlaceHolder(`C:\Users\you\AppData\Local\Game\Saved\SaveGames`)
	if len(game.SaveDirs) > 0 {
		dir.SetText(game.SaveDirs[0].Path)
	}

	name := widget.NewEntry()
	if !byHand {
		name.SetText(game.Name)
	}

	seed := widget.NewCheck("upload this folder's current save as the world's first version", nil)
	seed.SetChecked(true)

	// Only worlds this machine has not already linked: linking a world
	// twice on one machine is two folders claiming one save.
	free := make([]companion.World, 0, len(st.Sync.Worlds))
	for _, w := range st.Sync.Worlds {
		taken := false
		for _, l := range st.Links {
			if l.WorldID == w.World.ID {
				taken = true
			}
		}
		if !taken {
			free = append(free, w)
		}
	}
	const createNew = "— create a new world —"
	options := []string{createNew}
	for _, w := range free {
		label := w.World.Name
		if w.World.GameTitle != "" {
			label += " · " + w.World.GameTitle
		}
		options = append(options, label)
	}

	form := container.NewVBox()
	errLine := container.NewStack()
	splitLine := container.NewStack()

	var chosen *companion.World
	// leaf is the world's own folder name, when joining a world that
	// records one.
	leaf := func() string {
		if chosen == nil {
			return ""
		}
		return chosen.World.SavePath
	}

	// newWorldFields disappear when an existing world is chosen — there
	// is nothing to name and nothing to seed.
	newWorldFields := container.NewVBox(
		text("New world's name", colMist, szCaption),
		name,
		seed,
	)

	// split holds what the last explainer computed, because creating a
	// world records its leaf.
	var split companion.SavePathSplit

	refreshSplit := func() {
		go u.explainSplit(dir.Text, leaf(), game, splitLine, &split)
	}

	worlds := widget.NewSelect(options, func(label string) {
		chosen = nil
		for i := range free {
			l := free[i].World.Name
			if free[i].World.GameTitle != "" {
				l += " · " + free[i].World.GameTitle
			}
			if l == label {
				chosen = &free[i]
			}
		}
		fyne.Do(func() {
			if chosen == nil {
				newWorldFields.Show()
			} else {
				newWorldFields.Hide()
			}
		})
		refreshSplit()
	})
	worlds.SetSelected(createNew)
	dir.OnChanged = func(string) { refreshSplit() }

	if len(game.SaveDirs) > 0 {
		paths := make([]string, 0, len(game.SaveDirs))
		for _, c := range game.SaveDirs {
			paths = append(paths, c.Path+" — "+c.Why)
		}
		candidates := widget.NewSelect(paths, func(s string) {
			if i := strings.Index(s, " — "); i > 0 {
				dir.SetText(s[:i])
			}
		})
		candidates.SetSelected(paths[0])
		form.Add(text("Save folder found on this machine", colMist, szCaption))
		form.Add(candidates)
	} else {
		form.Add(callout(colGold, wrapped(
			"No save folder was found for this game. Choose the folder below to link it — it is usually under %LOCALAPPDATA%, Documents\\My Games, or Saved Games. Nothing is linked until you do.",
			colParchment)))
	}

	dirLabel := "Save folder (required)"
	form.Add(text(dirLabel, colMist, szCaption))
	form.Add(container.NewBorder(nil, nil, nil, u.folderPickerButton(dir), dir))
	form.Add(text("World on the service", colMist, szCaption))
	form.Add(worlds)
	form.Add(splitLine)
	form.Add(newWorldFields)
	form.Add(errLine)

	fail := func(msg string) {
		fyne.Do(func() {
			errLine.Objects = []fyne.CanvasObject{callout(colEmber, wrapped(msg, colEmber))}
			errLine.Refresh()
		})
	}

	submit := func() {
		fyne.Do(func() {
			errLine.Objects = nil
			errLine.Refresh()
		})
		gameTitle := trim(game.Name)
		if gameTitle == "" {
			gameTitle = trim(name.Text)
		}
		path := trim(dir.Text)
		// Caught here rather than at the service: the answer is a folder
		// the player has to supply, and the round trip only delays the
		// ask.
		if path == "" {
			fail("This needs the game's save folder before it can link. Choose the folder that holds the save files.")
			return
		}
		if chosen == nil && trim(name.Text) == "" {
			fail("A new world needs a name.")
			return
		}

		go func() {
			linkDir := path
			savePath := ""
			if chosen != nil && leaf() != "" {
				// Joining a world that knows its own folder: what the
				// player gave is their save root, and the world's folder
				// is created beneath it. This is the whole point — the
				// folder name is usually an opaque id they have no way
				// to type.
				made, _, err := u.engine.ResolveSavePath(path, leaf(), true)
				if err != nil {
					fail(err.Error())
					return
				}
				linkDir = made
			} else if chosen == nil {
				// Creating: record the folder this world lives in, so
				// the next player gets it made for them.
				savePath = split.Leaf
			}
			meta := metaJSON(game)
			var err error
			var okMsg string
			if chosen != nil {
				err = u.engine.Link(chosen.World.ID, gameTitle, linkDir, meta, game.AppID)
				okMsg = "linked"
			} else {
				err = u.engine.CreateWorld(trim(name.Text), gameTitle, linkDir, meta, game.AppID, savePath, seed.Checked)
				okMsg = "world created and linked"
				if seed.Checked {
					okMsg = "world created and seeded with the current save"
				}
			}
			if err != nil {
				// Reported inside the dialog, which stays open: anything
				// that stops a link is the player's next action, so it
				// has to be where they are looking, and the form has to
				// survive to be corrected.
				fail(err.Error())
				return
			}
			fyne.Do(u.dismiss)
			u.say(okMsg, false)
		}()
	}

	buttons := container.NewHBox(
		primaryButton("Link", submit),
		quietButton("Cancel", func() { u.dismiss() }),
	)
	if !byHand {
		label := "Hide from shelf"
		hidden := !game.Hidden
		if game.Hidden {
			label = "Show on shelf"
		}
		// Offered wherever a shelf entry is open, because the entries
		// worth hiding are exactly the ones you only notice by clicking
		// them and finding they are not a game.
		buttons.Add(quietButton(label, func() {
			key := game.Key
			if key == "" {
				key = gameKey(game.AppID, game.Name)
			}
			u.run(func() error { return u.engine.Hide(key, hidden) }, "")
			u.dismiss()
		}))
	}

	title := "Link a folder"
	if game.Name != "" {
		title = "Link " + game.Name
	}
	// The buttons are pinned outside the scroll: a form long enough to
	// need scrolling must not hide its own Link button below the fold.
	body := container.NewBorder(nil, container.NewPadded(buttons), nil, nil, dialogBody(form))
	u.show(dialog.NewCustomWithoutButtons(title, body, u.win))
	refreshSplit()
}

// explainSplit shows the two halves of a save folder before anything is
// created.
//
// Joining an existing world is the case this exists for. A world's
// folder is often an opaque id an Unreal game generated once: everyone
// playing that world shares it, nobody can retype it, and the game will
// not create it until it has saved there. So the joining player supplies
// only the half they can know, and the companion makes the rest
// underneath. Creating is the mirror — the folder is split, the player
// is shown where the line falls, and the leaf is recorded so everyone
// who joins later gets it made for them.
func (u *ui) explainSplit(dir, leaf string, game companion.Game, into *fyne.Container, out *companion.SavePathSplit) {
	set := func(o fyne.CanvasObject) {
		fyne.Do(func() {
			if o == nil {
				into.Objects = nil
			} else {
				into.Objects = []fyne.CanvasObject{o}
			}
			into.Refresh()
		})
	}
	dir = trim(dir)
	if dir == "" {
		*out = companion.SavePathSplit{}
		set(nil)
		return
	}
	if leaf != "" {
		*out = companion.SavePathSplit{}
		// create:false — nothing is made until the player submits.
		made, exists, err := u.engine.ResolveSavePath(dir, leaf, false)
		if err != nil {
			set(callout(colRune, wrapped(err.Error(), colEmber)))
			return
		}
		verb := "create"
		if exists {
			verb = "use"
		}
		lines := container.NewVBox(
			wrapped("This world lives in a folder named "+leaf+". Point at the folder your game keeps its saves in above, and linking will "+verb+":", colParchment),
			monoText(made, colGoldHi, szCaption),
		)
		if !exists {
			lines.Add(italicText("It does not exist yet — that is expected if you have never played this world.", colMist, szCaption))
		}
		set(callout(colRune, lines))
		return
	}

	split, err := u.engine.SplitSavePath(dir, game.AppID, game.Name)
	if err != nil || split.Leaf == "" {
		*out = split
		set(nil)
		return
	}
	*out = split
	why := ""
	if split.Why != "" {
		why = " — " + split.Why
	}
	set(callout(colRune, container.NewVBox(
		wrapped("This world will be recorded as the folder "+split.Leaf+", inside "+split.Root+why+". Anyone else joining picks their own save folder and gets that same folder name created for them, so they never need to know it.", colParchment),
	)))
}

// --- a linked tile ---

// showLinkedGame is what a *linked* tile opens: what it points at, and
// the way out. Custody itself lives in "Your worlds" at the top of the
// window — one world, one place to check it in and out.
func (u *ui) showLinkedGame(game companion.Game, link companion.WorldLink, world *companion.World) {
	target := widget.NewEntry()
	target.TextStyle = fyne.TextStyle{Monospace: true}
	target.SetText(link.LaunchTarget)
	if link.AppID != "" {
		target.SetPlaceHolder("steam://rungameid/" + link.AppID)
	} else {
		target.SetPlaceHolder(`D:\Games\thegame.exe`)
	}

	worldName := "world #" + itoa(link.WorldID)
	if world != nil {
		worldName = world.World.Name
	}

	explain := container.NewStack()
	updateExplain := func() {
		probe := link
		probe.LaunchTarget = target.Text
		var o fyne.CanvasObject
		if opens := launchTargetOf(probe); opens != "" {
			o = italicText("Checking this world out will open "+opens, colMist, szCaption)
		} else {
			o = wrapped("Nothing here says what starts this game, so checking the world out will fetch the save and leave the game to you. A path or a URI the desktop can open — an .exe, a shortcut, another launcher's link. Not a command line; a shortcut carries its arguments already.", colMist)
		}
		explain.Objects = []fyne.CanvasObject{o}
		explain.Refresh()
	}
	target.OnChanged = func(string) { updateExplain() }
	updateExplain()

	hideLabel := "Hide from shelf"
	if game.Hidden {
		hideLabel = "Show on shelf"
	}
	key := game.Key
	if key == "" {
		key = gameKey(game.AppID, game.Name)
	}

	body := container.NewVBox(
		wrapped("Linked to "+worldName+" — check it out and in from Your worlds at the top of this window.", colParchment),
		monoText(link.Dir, colMist, szCaption),
		widget.NewSeparator(),
		// What starts this game when the world is checked out. Steam
		// games answer for themselves; this is for the ones that cannot
		// — a non-Steam install, a modded launcher, a shortcut.
		text("Launch target (optional)", colMist, 11),
		container.NewBorder(nil, nil, nil, u.filePickerButton(target), target),
		explain,
		actions(quietButton("Save launch target", func() {
			t := target.Text
			msg := "launch target cleared"
			if trim(t) != "" {
				msg = "launch target saved"
			}
			u.run(func() error {
				return u.engine.EditLink(link.WorldID, companion.LinkEdit{LaunchTarget: &t})
			}, msg)
		})),
		widget.NewSeparator(),
		actions(
			dangerButton("Unlink", func() {
				u.confirm("Unlink this game?", "Nothing is deleted.", "Unlink", func() {
					u.run(func() error { return u.engine.Unlink(link.WorldID) }, "unlinked")
					u.dismiss()
				})
			}),
			quietButton(hideLabel, func() {
				u.run(func() error { return u.engine.Hide(key, !game.Hidden) }, "")
				u.dismiss()
			}),
			quietButton("Close", func() { u.dismiss() }),
		),
	)

	title := game.Name
	if title == "" {
		title = worldName
	}
	u.show(dialog.NewCustomWithoutButtons(title, dialogBody(body), u.win))
}

// --- edit a world ---

// showEditWorld covers the two things about a linked world that are not
// settled for good at link time after all: the local folder it points
// at, and the name it carries on the service. Each saves independently,
// since one lives here and the other is a call to the service.
func (u *ui) showEditWorld(link companion.WorldLink, world *companion.World) {
	name := widget.NewEntry()
	if world != nil {
		name.SetText(world.World.Name)
	}
	dir := widget.NewEntry()
	dir.TextStyle = fyne.TextStyle{Monospace: true}
	dir.SetText(link.Dir)

	held := link.SessionID != 0
	folderNote := container.NewStack()
	if held {
		folderNote.Objects = []fyne.CanvasObject{
			italicText("Check this world in before pointing it at a different folder.", colMist, szCaption),
		}
	}

	saveFolder := quietButton("Save folder", func() {
		d := trim(dir.Text)
		u.run(func() error {
			return u.engine.EditLink(link.WorldID, companion.LinkEdit{Dir: &d})
		}, "folder updated")
	})
	if held {
		saveFolder.Disable()
	}

	body := container.NewVBox(
		text("World name", colMist, szCaption),
		name,
		actions(quietButton("Save name", func() {
			n := trim(name.Text)
			u.run(func() error {
				return u.engine.EditLink(link.WorldID, companion.LinkEdit{WorldName: &n})
			}, "world renamed")
		})),
		widget.NewSeparator(),
		text("Local folder", colMist, szCaption),
		container.NewBorder(nil, nil, nil, u.folderPickerButton(dir), dir),
		folderNote,
		actions(saveFolder),
		widget.NewSeparator(),
		actions(quietButton("Close", func() { u.dismiss() })),
	)
	u.show(dialog.NewCustomWithoutButtons("Edit world", dialogBody(body), u.win))
}

// --- rename a world ---

// showRenameWorld is the overflow menu's Rename: one field, one verb.
//
// Renaming used to be buried in the Edit dialog beside the folder
// picker, which meant the commonest small edit was two clicks deeper
// than the rarest. The folder still lives in Edit; the name has its own
// door.
func (u *ui) showRenameWorld(link companion.WorldLink, world *companion.World) {
	name := widget.NewEntry()
	current := ""
	if world != nil {
		current = world.World.Name
	}
	name.SetText(current)

	var d *dialog.CustomDialog
	save := func() {
		n := trim(name.Text)
		if n == "" || n == current {
			d.Hide()
			return
		}
		d.Hide()
		u.run(func() error {
			return u.engine.EditLink(link.WorldID, companion.LinkEdit{WorldName: &n})
		}, "world renamed")
	}
	name.OnSubmitted = func(string) { save() }

	body := container.NewVBox(
		wrapped("The name everyone in your group sees for this world. It is stored on the vault, so renaming it here renames it for all of them.", colMist),
		text("World name", colMist, szCaption),
		name,
	)
	buttons := actions(
		primaryButton("Rename", save),
		quietButton("Cancel", func() { d.Hide() }),
	)
	content := container.NewBorder(nil, container.NewPadded(buttons), nil, nil, dialogBody(body))
	d = dialog.NewCustomWithoutButtons("Rename world", content, u.win)
	d.Resize(u.confirmSize())
	d.Show()
}

// --- diagnostics ---

// showDiagnostics is where the facts that are only interesting when
// something is wrong now live: the scan trail, every path it tried, the
// save-location catalogue's state, and the two build versions.
//
// All of it used to be in the window's chrome — the trail at the bottom
// of the main screen, the versions in the footer, a save path in a well
// on every world row. None of it is daily information, and having it in
// the chrome is what made the window read as a debug console with a
// custody app inside it.
func (u *ui) showDiagnostics() {
	st := u.snapshot()
	u.mu.Lock()
	artAsked, artCount, artError := u.artAsked, len(u.art), u.artError
	hintsOK, hintsKnown, hintsError := u.hintsOK, u.hintsKnown, u.hintsError
	u.mu.Unlock()

	versions := "companion " + st.Version
	switch {
	case st.Sync.ServerVersion != "":
		versions += " · vault " + st.Sync.ServerVersion
	case st.Sync.Configured:
		versions += " · vault version unknown"
	}

	body := container.NewVBox(
		sectionLabel("Builds", colMist, ""),
		monoText(versions, colMist, szMicro),
	)

	body.Add(widget.NewSeparator())
	body.Add(sectionLabel("Vault", colMist, ""))
	body.Add(monoText("url  "+orDash(st.Config.ServerURL), colMist, szMicro))
	body.Add(monoText("as   "+orDash(st.Sync.Username), colMist, szMicro))
	if st.Sync.PolledAt != nil {
		body.Add(monoText("last poll  "+st.Sync.PolledAt.Local().Format("2 Jan 15:04:05"), colMist, szMicro))
	}
	if st.Sync.LastError != "" {
		body.Add(wrapped("last error: "+st.Sync.LastError, colEmber))
	}
	if st.Sync.LastAction != "" {
		body.Add(monoText("last action  "+st.Sync.LastAction, colMist, szMicro))
	}

	body.Add(widget.NewSeparator())
	body.Add(sectionLabel("Game scan", colMist, ""))
	body.Add(u.scanTrail(st, false))
	switch {
	case hintsError != "":
		body.Add(wrapped("Save-location catalogue unavailable: "+hintsError, colEmber))
	case hintsOK:
		body.Add(wrapped("Save locations known for "+itoa(int64(hintsKnown))+" games (Ludusavi manifest, via the vault).", colMist))
	case st.Sync.Configured:
		body.Add(wrapped("The vault has no save-location catalogue loaded — folders are found by search alone.", colMist))
	}
	if artError != "" {
		body.Add(wrapped("Cover art unavailable: "+artError, colEmber))
	} else {
		body.Add(monoText("cover art  "+itoa(int64(artCount))+" resolved of "+itoa(int64(artAsked))+" asked", colMist, szMicro))
	}

	// The save paths, all in one place. This is the fact the world rows
	// used to carry in a well each, and the reason it is here is that
	// you look it up when something has gone to the wrong folder — not
	// every time you check a world out.
	if len(st.Links) > 0 {
		body.Add(widget.NewSeparator())
		body.Add(sectionLabel("Linked folders", colMist, ""))
		for _, l := range st.Links {
			label := "world #" + itoa(l.WorldID)
			if w := worldFor(st, l.WorldID); w != nil {
				label = w.World.Name
			}
			body.Add(text(label, colParchment, szCaption))
			body.Add(pathLine(l.Dir))
		}
		body.Add(actions(quietButton("Copy all save paths", func() {
			var b strings.Builder
			for _, l := range st.Links {
				b.WriteString(l.Dir)
				b.WriteByte('\n')
			}
			u.app.Clipboard().SetContent(b.String())
			u.say("save paths copied", false)
		})))
	}

	sheet := container.NewBorder(nil,
		container.NewPadded(actions(quietButton("Close", func() { u.dismiss() }))),
		nil, nil, dialogBody(body))
	u.show(dialog.NewCustomWithoutButtons("Diagnostics", sheet, u.win))
}

func orDash(s string) string {
	if trim(s) == "" {
		return "—"
	}
	return s
}

// --- settings ---

// showSettings holds everything that is not about one world: where the
// vault is, where Steam is, what a checkout does, the desktop-only
// behaviours, and this build's own version.
func (u *ui) showSettings() {
	st := u.snapshot()

	url := widget.NewEntry()
	url.SetPlaceHolder("https://vault.example.com")
	url.SetText(st.Config.ServerURL)
	token := widget.NewPasswordEntry()
	tokenLabel := "Your sync token"
	if st.Config.TokenSet {
		tokenLabel += " (saved — paste to replace)"
	}
	token.SetPlaceHolder("paste the token from the service's page")

	steam := widget.NewEntry()
	steam.TextStyle = fyne.TextStyle{Monospace: true}
	steam.SetPlaceHolder(`e.g. D:\SteamLibrary or D:\Steam\steamapps\common`)
	if len(st.Config.SteamDirs) > 0 {
		steam.SetText(st.Config.SteamDirs[0])
	}

	launch := widget.NewCheck("Start the game when I check a world out", func(on bool) {
		v := on
		u.run(func() error {
			return u.engine.SetConfig(companion.ConfigUpdate{LaunchOnCheckout: &v})
		}, "")
	})
	launch.SetChecked(st.Config.LaunchOnCheckout)

	notify := widget.NewCheck("Notify me when a claim arrives, a hold is nearly up, or a sync fails", func(on bool) {
		u.setNotifyEnabled(on)
	})
	notify.SetChecked(u.notifyEnabled())

	autostart := widget.NewCheck("Start with this computer, minimized to the tray", func(on bool) {
		if err := setAutostart(on); err != nil {
			u.say(err.Error(), true)
			return
		}
		if on {
			u.say("the companion will start with this computer", false)
		} else {
			u.say("the companion will no longer start with this computer", false)
		}
	})
	if on, err := autostartEnabled(); err == nil {
		autostart.SetChecked(on)
	} else {
		autostart.Disable()
	}

	updateLine := container.NewStack(text(updateWord(st), colMist, 12))

	body := container.NewVBox(
		text("Save-sync service URL", colMist, szCaption),
		url,
		text(tokenLabel, colMist, szCaption),
		token,
		actions(primaryButton("Save & connect", func() {
			serverURL := url.Text
			tok := token.Text
			u.run(func() error {
				// A completed connection is proven with a status poll: a
				// typo'd token fails here, not silently every minute
				// forever. An empty token keeps the saved one.
				err := u.engine.SetConfig(companion.ConfigUpdate{ServerURL: &serverURL, Token: tok})
				fyne.Do(func() { token.SetText("") })
				return err
			}, "connected")
		})),

		widget.NewSeparator(),
		text("Steam folder (blank = auto-detect)", colMist, szCaption),
		container.NewBorder(nil, nil, nil, u.folderPickerButton(steam), steam),
		// Wrapped, not a one-line canvas.Text: this is a sentence, and a
		// sentence in a dialog that can be narrower than the sentence has
		// to reflow rather than run off the edge.
		wrapped("Paste the Steam root, steamapps, or steamapps\\common — extra libraries on other drives are found from it.", colMist),
		actions(quietButton("Save folder & rescan", func() { u.saveSteamDir(steam.Text) })),

		widget.NewSeparator(),
		launch,
		wrapped("The save is put in place first, then the game starts — never the other way round. Switch it off to take custody of a world without opening it. Games linked by hand carry nothing that says what starts them, so those check out without launching either way.", colMist),

		widget.NewSeparator(),
		notify,
		autostart,

		widget.NewSeparator(),
		// The one door into the machine facts. Everything the window
		// chrome used to carry — the scan trail, the tried paths, the
		// save paths, the build versions — is behind it.
		text("Diagnostics", colMist, szCaption),
		wrapped("The game scan's trail, every path it tried, the folders this machine has linked, and which builds are talking to each other.", colMist),
		actions(quietButton("Open diagnostics", func() { u.showDiagnostics() })),

		widget.NewSeparator(),
		text("Companion version ("+st.Version+")", colMist, szCaption),
		updateLine,
		actions(quietButton("Check for update", func() {
			go func() {
				up := u.engine.CheckUpdate(u.ctx())
				switch {
				case up.Error != "":
					u.say(up.Error, true)
				case up.Available:
					u.say("update available: "+up.Version, false)
				default:
					u.say("you're up to date", false)
				}
			}()
		})),

		widget.NewSeparator(),
	)
	// Close is pinned below the scroll rather than at the bottom of it:
	// the settings sheet is the longest thing in the app, and the way out
	// of it must not be something you have to scroll to find.
	sheet := container.NewBorder(nil,
		container.NewPadded(actions(quietButton("Close", func() { u.dismiss() }))),
		nil, nil, dialogBody(body))
	u.show(dialog.NewCustomWithoutButtons("Settings", sheet, u.win))
}

func updateWord(st companion.State) string {
	up := st.Update
	switch {
	case up.Error != "":
		return "last check failed: " + up.Error
	case up.Available:
		return "an update is available: " + up.Version
	case up.CheckedAt != nil:
		return "up to date, as of the last check"
	}
	return "checked automatically every few hours"
}

// --- small shared bits ---

func trim(s string) string { return strings.TrimSpace(s) }

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// metaJSON is what a link records about the game it came from. Kept as
// the same two fields the web dialog sends, because the service stores
// it verbatim.
func metaJSON(g companion.Game) string {
	return `{"appId":` + quote(g.AppID) + `,"installDir":` + quote(g.InstallDir) + `}`
}

func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

type errString string

func (e errString) Error() string { return string(e) }

// ctx is the context the update calls run under. Deliberately not tied
// to any widget's lifetime: a download outlives the dialog that started
// it.
func (u *ui) ctx() context.Context { return context.Background() }
