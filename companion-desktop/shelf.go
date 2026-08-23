package main

// The installed-game shelf, and the trail that explains an empty one.
//
// web/companion/src/components/Shelf.tsx, GameTile.tsx and ScanTrail.tsx
// are the spec. Linked games are in colour with a gold border; unlinked
// ones are dimmed — telling the two apart at a glance is the whole point
// of the shelf. Hidden entries collapse into one dashed tile that says
// how many and offers to show them: a Steam library is full of things
// that are not games, and a shelf of redistributables is a shelf nobody
// reads.

import (
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
const tileCaptionWidth = tileWidth - 18

// tileHeight is the cover at 3:4 plus the two caption lines under it.
const tileHeight = tileWidth*4/3 + 48

func (u *ui) shelf(st companion.State) fyne.CanvasObject {
	rows := container.NewVBox(sectionHeader(
		"Installed games",
		"linked games in colour — click a dimmed tile to link it",
		quietButton("Rescan", func() {
			go func() {
				u.engine.Rescan()
				found := len(u.engine.Snapshot().Discovered.Games)
				u.say("rescanned — "+plural(found, "game", "games")+" found", false)
			}()
		}),
		quietButton("Link a folder by hand…", func() { u.showLinkGame(companion.Game{}, true) }),
	))

	games := st.Discovered.Games
	hiddenCount := 0
	for _, g := range games {
		if g.Hidden {
			hiddenCount++
		}
	}
	shown := make([]companion.Game, 0, len(games))
	for _, g := range games {
		if u.showHidden || !g.Hidden {
			shown = append(shown, g)
		}
	}

	tiles := container.NewGridWrap(fyne.NewSize(tileWidth, tileHeight))
	for _, g := range shown {
		tiles.Add(u.gameTile(st, g))
	}
	if hiddenCount > 0 {
		tiles.Add(u.hiddenTile(hiddenCount))
	}
	rows.Add(tiles)

	switch {
	case len(shown) == 0 && hiddenCount == 0:
		rows.Add(italicText("No games found. The scan trail below says where it looked — if your Steam folder is missing or was rejected, set it in the settings. Any save folder can also be linked by hand.", colMist, szCaption))
	case len(shown) == 0:
		rows.Add(italicText("Every game found here is hidden.", colMist, szCaption))
	}

	// Covers and save locations both degrade to nothing rather than to
	// an error — but when the *service* is the reason, saying so points
	// at the panel that can fix it.
	u.mu.Lock()
	artError, artAsked, artCount := u.artError, u.artAsked, len(u.art)
	hintsOK, hintsKnown, hintsError := u.hintsOK, u.hintsKnown, u.hintsError
	u.mu.Unlock()
	switch {
	case artError != "":
		rows.Add(italicText("Cover art unavailable: "+artError, colEmber, szCaption))
	case artAsked > 0 && artCount == 0:
		rows.Add(italicText("The sync service has no cover art for these games — check its Cover art panel.", colMist, szCaption))
	}
	switch {
	case hintsError != "":
		rows.Add(italicText("Save-location catalogue unavailable: "+hintsError, colEmber, szCaption))
	case hintsOK:
		rows.Add(italicText("Save locations known for "+itoa(int64(hintsKnown))+" of these games (Ludusavi manifest, via the sync service).", colMist, 12))
	case st.Sync.Configured:
		rows.Add(italicText("The sync service has no save-location catalogue loaded — folders are found by search alone.", colMist, szCaption))
	}

	rows.Add(u.scanTrail(st, false))
	return rows
}

// gameTile is one shelf entry: cover, name, and what it is linked to.
func (u *ui) gameTile(st companion.State, g companion.Game) fyne.CanvasObject {
	link := linkFor(g, st.Links)
	linked := link != nil

	label := g.Name
	if found := u.artFor(g.AppID, g.Name); found.Name != "" {
		label = found.Name
	}

	caption := "not linked"
	captionColor := colMist
	if g.Hidden {
		caption = "hidden"
	}
	if linked {
		caption = "linked"
		captionColor = colGold
		if w := worldFor(st, link.WorldID); w != nil {
			caption = w.World.Name
		}
	}

	art := coverTile(u.cover(g.AppID, g.Name, u.redraw), label, tileWidth-2, !linked)

	// Both lines are trimmed to the tile. A game's name is whatever its
	// publisher chose — "RuneScape: Dragonwilds" and "DragonSword:
	// Awakening" both ran off their tiles and collided with the tile
	// beside them — and canvas.Text does not truncate on its own. The
	// full name is not lost: it is the tile's hover tip.
	nameStyle := fyne.TextStyle{Bold: true}
	name := text(ellipsize(label, tileCaptionWidth, szCaption, nameStyle), colParchment, szCaption)
	name.TextStyle = nameStyle
	cap := text(ellipsize(caption, tileCaptionWidth, szMicro, fyne.TextStyle{}), captionColor, szMicro)

	frame := canvas.NewRectangle(colInk)
	frame.StrokeColor = colEdge
	frame.StrokeWidth = 1
	frame.CornerRadius = 6
	if linked {
		frame.StrokeColor = colGold
	}

	body := container.NewBorder(art, nil, nil, nil, container.NewVBox(name, cap))
	// A whole-tile tap target rather than a button around it: Fyne's
	// button would paint its own fill over the cover.
	tap := newTapArea(func() { u.showGame(st, g) })
	// The full name, for the tiles whose captions had to be trimmed.
	// Fyne 2.8 has no tooltip of its own, so tapArea grows one.
	tip := label
	if caption != "not linked" && caption != "hidden" && caption != "linked" {
		tip += " — " + caption
	}
	tap.setTip(u.win, tip)
	return container.NewStack(frame, body, tap)
}

// hiddenTile is the one dashed entry the put-away shelf items collapse
// into. Fyne has no dashed stroke, so it is drawn as a quiet well tile —
// unmistakably not a game.
func (u *ui) hiddenTile(n int) fyne.CanvasObject {
	frame := canvas.NewRectangle(colWell)
	frame.StrokeColor = colEdge
	frame.StrokeWidth = 1
	frame.CornerRadius = 6

	word := "entries hidden"
	if n == 1 {
		word = "entry hidden"
	}
	action := "show them"
	if u.showHidden {
		action = "hide them again"
	}
	lines := container.NewVBox(
		text(itoa(int64(n))+" "+word, colMist, 12),
		text(action, colGoldHi, szCaption),
	)
	return container.NewStack(frame, container.NewCenter(lines), newTapArea(func() {
		u.showHidden = !u.showHidden
		u.redraw()
	}))
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
// miss missed. This is the whole answer to "why no games", so it is a
// collapsible rather than a wall of mono — and a fresh scan that found
// nothing opens itself, because at that moment it is the only thing on
// screen worth reading.
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
		lines.Add(container.NewHBox(
			text(mark, markColor, 12),
			boldText(p.Source, colMist, szCaption),
			monoText(p.Path, colMist, szMicro),
		))
		if p.Resolved != "" && p.Resolved != p.Path {
			lines.Add(monoText("    → "+p.Resolved, colMist, szMicro))
		}
		if p.Note != "" {
			c := colMist
			if p.Resolved == "" {
				c = colEmber
			}
			lines.Add(text("    "+p.Note, c, 11))
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
