package main

// The Games tab: setup, visited rarely, and no longer the first thing
// anyone sees.
//
// What was wrong with this as a shelf on the Worlds page: it took
// roughly 70% of the window, eleven of thirteen tiles read "not linked"
// under a dim treatment that already said so, the one linked tile was
// lost among them, titles truncated mid-word, there was no search, and
// the "4 entries hidden / show them" control was rendered as a fake game
// tile inside the grid.
//
// So: linked games get their own section above the rest, the per-tile
// note says something useful instead of restating the dimming, names
// wrap to two lines rather than ellipsizing, and hidden entries are a
// line of prose below the grid rather than something masquerading as a
// game.

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/safwyls/artificer/companion"
)

// tileWidth matches the web grid's 130 px minimum column, widened a
// little: the type scale went up (theme.go), and a caption at 13 px
// needs more room than one at 10 px before it starts losing words.
const tileWidth = 148

// tileCaptionWidth is how much of a tile a caption may use. The inset
// leaves room for the tile's own border and padding, so a trimmed
// caption stops short of the edge rather than touching it.
const tileCaptionWidth = tileWidth - 20

// tileHeight is the cover at 3:4 plus two wrapped name lines and a note.
const tileHeight = tileWidth*4/3 + 62

// gameFilter is the segmented control's three positions.
type gameFilter int

const (
	filterAll gameFilter = iota
	filterLinked
	filterUnlinked
)

// gamesPage is the whole tab: the toolbar, then linked and unlinked in
// their own sections.
func (u *ui) gamesPage(st companion.State) fyne.CanvasObject {
	u.mu.Lock()
	artError, artAsked, artCount := u.artError, u.artAsked, len(u.art)
	hintsOK, hintsError := u.hintsOK, u.hintsError
	u.mu.Unlock()

	var linked, unlinked []companion.Game
	hiddenCount := 0
	for _, g := range st.Discovered.Games {
		if g.Hidden {
			hiddenCount++
			if !u.showHidden {
				continue
			}
		}
		if linkFor(g, st.Links) != nil {
			linked = append(linked, g)
		} else {
			unlinked = append(unlinked, g)
		}
	}

	page := container.NewVBox(u.gamesToolbar(st, len(linked)+len(unlinked), len(linked), len(unlinked)))

	// The search and the filter narrow what is drawn; the counts on the
	// filter itself stay whole, because a segmented control whose own
	// labels changed as you filtered would be unreadable.
	showLinked := u.matching(linked)
	showUnlinked := u.matching(unlinked)

	if u.gamesFilter != filterUnlinked && len(showLinked) > 0 {
		page.Add(sectionLabel("Linked · "+itoa(int64(len(showLinked))), colGold, ""))
		page.Add(u.tileGrid(st, showLinked))
	}
	if u.gamesFilter != filterLinked {
		sub := ""
		// "of these" has to mean the tiles actually under the heading.
		// The engine's own count is over every discovered game, which
		// on a filtered or searched page names a set the reader cannot
		// see — so the count is taken over what is drawn.
		if hintsOK {
			known := 0
			for _, g := range showUnlinked {
				if len(g.SaveDirs) > 0 {
					known++
				}
			}
			sub = "Reliquary knows a save location for " + itoa(int64(known)) + " of these."
		}
		if len(showUnlinked) > 0 {
			page.Add(sectionLabel("Not linked · "+itoa(int64(len(showUnlinked))), colMist, sub))
			page.Add(u.tileGrid(st, showUnlinked))
		}
	}

	if len(showLinked)+len(showUnlinked) == 0 {
		page.Add(u.emptyGames(st, len(st.Discovered.Games)))
	}

	// Hidden entries are a line of text, not a tile. A control shaped
	// like a game is a control people click expecting a game.
	if hiddenCount > 0 {
		word := "entries were hidden as duplicates or launchers."
		if hiddenCount == 1 {
			word = "entry was hidden as a duplicate or a launcher."
		}
		action := "Show them"
		if u.showHidden {
			action = "Hide them again"
		}
		page.Add(container.NewHBox(
			text(itoa(int64(hiddenCount))+" "+word, colMist, szCaption),
			linkText(action, func() {
				u.showHidden = !u.showHidden
				u.redraw()
			}),
		))
	}

	// Covers and save locations both degrade to nothing rather than to
	// an error — but when the *service* is the reason, saying so points
	// at the panel that can fix it.
	switch {
	case artError != "":
		page.Add(italicText("Cover art unavailable: "+artError, colEmber, szCaption))
	case artAsked > 0 && artCount == 0:
		page.Add(italicText("The vault has no cover art for these games — check its Cover art panel.", colMist, szCaption))
	}
	if hintsError != "" {
		page.Add(italicText("Save-location catalogue unavailable: "+hintsError, colEmber, szCaption))
	}
	return page
}

// matching applies the search box. The filter itself is applied by the
// caller, which knows which section it is drawing.
func (u *ui) matching(games []companion.Game) []companion.Game {
	q := strings.ToLower(trim(u.gamesSearch))
	if q == "" {
		return games
	}
	out := make([]companion.Game, 0, len(games))
	for _, g := range games {
		if strings.Contains(strings.ToLower(g.Name), q) {
			out = append(out, g)
		}
	}
	return out
}

// gamesToolbar is search, the three filters, and the two setup verbs.
func (u *ui) gamesToolbar(st companion.State, all, linked, unlinked int) fyne.CanvasObject {
	search := widget.NewEntry()
	search.SetPlaceHolder("Search installed games")
	search.SetText(u.gamesSearch)
	search.OnChanged = func(s string) {
		// Stored on the ui, so the engine's next poll cannot empty the
		// box out from under whoever is typing in it.
		u.gamesSearch = s
		u.redraw()
	}
	// Bounded: a search field the width of the window reads as the page
	// being a search page, which this is not.
	box := container.NewGridWrap(fyne.NewSize(340, search.MinSize().Height), search)

	segments := container.NewHBox()
	for _, s := range []struct {
		f     gameFilter
		label string
	}{
		{filterAll, "All " + itoa(int64(all))},
		{filterLinked, "Linked " + itoa(int64(linked))},
		{filterUnlinked, "Unlinked " + itoa(int64(unlinked))},
	} {
		active := u.gamesFilter == s.f
		c := colMist
		if active {
			c = colGoldHi
		}
		cell := container.NewPadded(container.NewHBox(text(s.label, c, szCaption)))
		layers := []fyne.CanvasObject{}
		if active {
			bg := canvas.NewRectangle(colPanel)
			bg.CornerRadius = 3
			layers = append(layers, bg)
		}
		f := s.f
		layers = append(layers, cell, newTapArea(func() {
			u.gamesFilter = f
			u.redraw()
		}))
		segments.Add(container.NewStack(layers...))
	}
	// The whole segmented control sits in one bordered box, which is
	// what makes three words read as one control.
	seg := canvas.NewRectangle(colInk)
	seg.StrokeColor = colEdge
	seg.StrokeWidth = 1
	seg.CornerRadius = 4

	right := container.NewHBox(
		quietButton("Rescan", func() {
			go func() {
				u.engine.Rescan()
				found := len(u.engine.Snapshot().Discovered.Games)
				u.say("rescanned — "+plural(found, "game", "games")+" found", false)
			}()
		}),
		quietButton("Link a folder by hand…", func() { u.showLinkGame(companion.Game{}, true) }),
	)
	left := container.NewHBox(box, container.NewStack(seg, segments))
	return container.NewBorder(nil, nil, left, right)
}

// tileGrid wraps as many tiles per row as the window is wide enough
// for. The design draws six columns at 1440; this window is resizable,
// so the column count follows the width rather than being pinned to a
// number that would clip at 1120.
func (u *ui) tileGrid(st companion.State, games []companion.Game) fyne.CanvasObject {
	grid := container.NewGridWrap(fyne.NewSize(tileWidth, tileHeight))
	for _, g := range games {
		grid.Add(u.gameTile(st, g))
	}
	return grid
}

// gameTile is one entry: cover, a name that wraps, and what it is
// linked to — or an invitation to link it.
func (u *ui) gameTile(st companion.State, g companion.Game) fyne.CanvasObject {
	link := linkFor(g, st.Links)
	linked := link != nil

	label := g.Name
	if found := u.artFor(g.AppID, g.Name); found.Name != "" {
		label = found.Name
	}

	frame := canvas.NewRectangle(colWell)
	frame.StrokeColor = colEdge
	frame.StrokeWidth = 1
	frame.CornerRadius = 6

	// The note is the tile's one line of information, and it earns its
	// place: for a linked tile it names the world, for an unlinked one
	// it says whether the folder will be suggested or asked for. What
	// it never does is say "not linked" — the section heading and the
	// dimming both already said that.
	note, noteColor := "pick the folder yourself", colMist
	if len(g.SaveDirs) > 0 {
		note = "save folder known"
	}
	if linked {
		frame.FillColor = colPanel
		frame.StrokeColor = dim(colGold, 0x80)
		note, noteColor = "1 world", colGoldHi
		if w := worldFor(st, link.WorldID); w != nil {
			note = "1 world · " + w.World.Name
		}
	}
	if g.Hidden {
		note, noteColor = "hidden", colMist
	}

	art := coverTile(u.cover(g.AppID, g.Name, u.redraw), label, tileWidth-14, !linked)

	// Two lines rather than one ellipsized line: "RuneScape: Dragonwilds"
	// cut to "RuneScape: Dra…" names no game anybody recognises.
	nameStyle := fyne.TextStyle{Bold: true}
	names := container.NewVBox()
	for _, l := range wrapLines(label, tileCaptionWidth, 2, szMicro, nameStyle) {
		t := text(l, colParchment, szMicro)
		t.TextStyle = nameStyle
		names.Add(t)
	}

	// The Link affordance sits on the tile, so clickability is stated
	// where the click happens rather than in a section subtitle.
	foot := container.NewBorder(nil, nil, nil, nil,
		text(ellipsize(note, tileCaptionWidth-28, szMicro, fyne.TextStyle{}), noteColor, szMicro))
	if !linked && !g.Hidden {
		foot = container.NewBorder(nil, nil, nil,
			text("Link", colGoldHi, szMicro),
			text(ellipsize(note, tileCaptionWidth-28, szMicro, fyne.TextStyle{}), noteColor, szMicro))
	}

	body := container.NewPadded(container.NewBorder(
		container.NewCenter(art), foot, nil, nil, names))

	// A whole-tile tap target rather than a button around it: Fyne's
	// button would paint its own fill over the cover.
	tap := newTapArea(func() { u.showGame(st, g) })
	// The full name, for the tiles whose two lines still had to trim.
	tap.setTip(u, label)
	return container.NewStack(frame, body, tap)
}

// emptyGames is what the tab says when nothing is drawn, and it names
// its own cause rather than leaving a blank grid.
func (u *ui) emptyGames(st companion.State, found int) fyne.CanvasObject {
	switch {
	case trim(u.gamesSearch) != "":
		return quietWell(container.NewHBox(
			text("Nothing here matches “"+trim(u.gamesSearch)+"”.", colMist, szCaption),
			linkText("Clear the search", func() {
				u.gamesSearch = ""
				u.redraw()
			}),
		))
	case found == 0:
		return quietWell(container.NewVBox(
			wrapped("No games found on this machine. Diagnostics lists every path the scan tried — if your Steam folder is missing or was rejected, set it in Settings. Any save folder can also be linked by hand.", colMist),
			linkText("Open diagnostics", func() { u.showDiagnostics() }),
		))
	default:
		return quietWell(wrapped("Every game found here is hidden.", colMist))
	}
}

// showGame opens what a tile points at: a linked entry opens what it is
// linked to, an unlinked one opens the link form.
func (u *ui) showGame(st companion.State, g companion.Game) {
	if link := linkFor(g, st.Links); link != nil {
		u.showLinkedGame(g, *link, worldFor(st, link.WorldID))
		return
	}
	u.showLinkGame(g, false)
}

// scanTrail is every path the scan tried, what it resolved to, and why a
// miss missed.
//
// This is the whole answer to "why no games", and it used to sit at the
// bottom of the main screen where it was chrome nobody reads. It now
// lives in Diagnostics — and on the connect screen, which is the one
// place it is the most useful thing available.
func (u *ui) scanTrail(st companion.State, startOpen bool) fyne.CanvasObject {
	probes := st.Discovered.Probes
	if len(probes) == 0 {
		return container.NewVBox()
	}
	// The player's choice about this belongs to the player and must
	// survive a redraw. The web version learned this the hard way — its
	// markup was rewritten on every five-second poll, which snapped the
	// trail shut two seconds after anyone expanded it. Here the widget
	// is rebuilt rather than re-rendered, so the open flag is read back
	// off the old one before it is thrown away.
	if u.trailItem != nil {
		chosen := u.trailItem.Open
		u.trailChosen = &chosen
	}
	// A *fresh* scan that found nothing opens itself. One that found
	// something leaves the player's choice alone.
	sig := probeSignature(probes)
	if u.trailSig != "" && u.trailSig != sig {
		anyHit := false
		for _, p := range probes {
			if p.Resolved != "" {
				anyHit = true
			}
		}
		if !anyHit {
			open := true
			u.trailChosen = &open
		}
	}
	u.trailSig = sig

	hits := 0
	for _, p := range probes {
		if p.Resolved != "" {
			hits++
		}
	}

	lines := container.NewVBox()
	for _, p := range probes {
		mark, markColor := "·", colMist
		if p.Resolved != "" {
			mark, markColor = "✓", colOK
		}
		// The paths are truncated to whatever width the trail is given
		// rather than drawn at full length: a canvas.Text does not
		// truncate, and a deep library path is longer than the card it
		// sits in — which pushed the connect screen's right-hand column
		// off the side of the window.
		lines.Add(container.NewBorder(nil, nil, container.NewHBox(
			text(mark, markColor, szMicro),
			boldText(p.Source, colMist, szCaption),
		), nil, pathLine(p.Path)))
		if p.Resolved != "" && p.Resolved != p.Path {
			lines.Add(container.NewBorder(nil, nil,
				text("   →", colMist, szMicro), nil, pathLine(p.Resolved)))
		}
		if p.Note != "" {
			c := colMist
			if p.Resolved == "" {
				c = colEmber
			}
			lines.Add(text("    "+p.Note, c, szMicro))
		}
	}

	summary := "scan trail — " + plural(hits, "library", "libraries") + " found, " +
		plural(len(probes), "path", "paths") + " tried"
	item := widget.NewAccordionItem(summary, lines)
	acc := widget.NewAccordion(item)
	u.trailItem = item
	// The player's choice wins where they have made one; failing that a
	// trail with no hits opens itself, because then it is the only thing
	// on screen worth reading.
	open := hits == 0 || startOpen
	if u.trailChosen != nil {
		open = *u.trailChosen
	}
	if open {
		acc.Open(0)
	}
	return acc
}

// probeSignature identifies one scan's trail, so a *new* scan can be
// told from a redraw of the same one.
func probeSignature(probes []companion.Probe) string {
	out := make([]byte, 0, 128)
	for _, p := range probes {
		out = append(out, p.Source...)
		out = append(out, ':')
		out = append(out, p.Path...)
		out = append(out, ':')
		out = append(out, p.Resolved...)
		out = append(out, '|')
	}
	return string(out)
}
