package main

// The small pieces the vault needs and Fyne's stock widgets do not
// offer: coloured text at a chosen size, a panel card with an edge, an
// accent callout, the custody chip, and a cover-art tile.
//
// Everything here is presentation. Nothing in this file knows what a
// world is, which game it belongs to, or when a hold expires — that is
// the engine's, and the screens below read it from a Snapshot.

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// text is a line in a chosen colour and size. canvas.Text rather than
// widget.Label because the vault paints meaning with colour — mist for
// secondary, ember for an error, rune for a game tag — and a Label
// takes the theme's foreground and nothing else.
func text(s string, c color.Color, size float32) *canvas.Text {
	t := canvas.NewText(s, c)
	t.TextSize = size
	return t
}

func boldText(s string, c color.Color, size float32) *canvas.Text {
	t := text(s, c, size)
	t.TextStyle = fyne.TextStyle{Bold: true}
	return t
}

func monoText(s string, c color.Color, size float32) *canvas.Text {
	t := text(s, c, size)
	t.TextStyle = fyne.TextStyle{Monospace: true}
	return t
}

func italicText(s string, c color.Color, size float32) *canvas.Text {
	t := text(s, c, size)
	t.TextStyle = fyne.TextStyle{Italic: true}
	return t
}

// wrapped is prose that has to reflow: canvas.Text is one line forever,
// so anything longer than a label uses a Label with wrapping on.
func wrapped(s string, c color.Color) *widget.RichText {
	rt := widget.NewRichTextWithText(s)
	rt.Wrapping = fyne.TextWrapWord
	for _, seg := range rt.Segments {
		if ts, ok := seg.(*widget.TextSegment); ok {
			ts.Style.ColorName = colorNameFor(c)
		}
	}
	return rt
}

// wrappedCenter is prose that reflows *and* centres.
//
// The distinction matters because of how Fyne sizes things: putting a
// RichText inside container.NewCenter hands it its MinSize, and a
// RichText's MinSize is its text on one unwrapped line — so a centred
// paragraph does not wrap at all, it runs off the side of the window.
// Centring the text inside a full-width RichText is the way to have
// both.
func wrappedCenter(s string, c color.Color) *widget.RichText {
	rt := wrapped(s, c)
	for _, seg := range rt.Segments {
		if ts, ok := seg.(*widget.TextSegment); ok {
			ts.Style.Alignment = fyne.TextAlignCenter
		}
	}
	return rt
}

// colorNameFor maps one of the palette's colours back to the theme name
// that yields it, because RichText styles by name rather than by value.
func colorNameFor(c color.Color) fyne.ThemeColorName {
	switch c {
	case color.Color(colMist):
		return "disabled"
	case color.Color(colEmber):
		return "error"
	case color.Color(colGold), color.Color(colGoldHi):
		return "primary"
	case color.Color(colOK):
		return "success"
	}
	return "foreground"
}

// panelCard is the vault's card: panel fill, edge border, 8 px corners.
// Fyne's widget.Card carries a title area and its own padding rules, so
// the card is drawn here instead — a rectangle with a stroke, with the
// content stacked on it.
func panelCard(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colPanel)
	bg.StrokeColor = colEdge
	bg.StrokeWidth = 1
	bg.CornerRadius = 8
	return container.NewStack(bg, container.NewPadded(content))
}

// inset is the recessed strip: well fill on the page's ink, for the
// header and for boxed-in trails.
func inset(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colWell)
	bg.StrokeColor = colEdge
	bg.StrokeWidth = 1
	bg.CornerRadius = 4
	return container.NewStack(bg, container.NewPadded(content))
}

// callout is the web UI's accent-rule box: a 3 px coloured rule down the
// left, a wash of that colour behind, and the message beside it. The
// three that matter are gold (a hint), ember (a refusal) and rune (the
// split explainer).
func callout(accent color.NRGBA, content fyne.CanvasObject) fyne.CanvasObject {
	wash := canvas.NewRectangle(dim(accent, 0x1a))
	wash.CornerRadius = 4
	rule := canvas.NewRectangle(accent)
	rule.SetMinSize(fyne.NewSize(3, 1))
	return container.NewStack(wash, container.NewBorder(nil, nil, rule, nil, container.NewPadded(content)))
}

// chip is the pill the custody state is said in: a rounded fill, a
// border of the accent, and the word itself in the accent's bright form.
func chip(label string, fg, border, fill color.NRGBA) fyne.CanvasObject {
	bg := canvas.NewRectangle(fill)
	bg.StrokeColor = border
	bg.StrokeWidth = 1
	// Fyne has no true pill; the largest radius the theme uses reads as
	// one at this height.
	bg.CornerRadius = 9
	t := text(label, fg, szMicro)
	return container.NewStack(bg, container.NewPadded(container.NewHBox(t)))
}

// coverTile draws a game's cover at the web UI's 3:4 ratio. A cover that
// is missing (or that failed to fetch) falls back to the game's name on
// the cover fill, which is what the web CoverArt does rather than show a
// torn-page icon.
//
// The dimming an unlinked shelf tile wants is a translucent ink veil
// over the image: Fyne has no CSS grayscale, and a veil is the honest
// approximation — it reads as "not yet yours" at a glance, which is the
// whole job of the greyed tile.
// The aspect bug this fixes is worth naming, because the fix is not the
// obvious one. The first cut used canvas.ImageFillStretch with a
// SetMinSize of the 3:4 frame. SetMinSize is a *floor*, not a size: put
// that image in a row that has spare width — which every world row has,
// because the name column beside it is elastic — and the layout hands
// the image more width than the minimum, Stretch dutifully fills all of
// it, and the cover comes out squashed horizontally by however much
// room the row happened to have. At a different window width it
// squashes by a different amount, which is why it looked like a
// rendering fault rather than a layout one.
//
// So the frame is pinned rather than floored: a GridWrap cell is
// resized to exactly its CellSize whatever the parent offers, and
// ImageFillContain fits the art inside it at the art's own aspect. The
// cover fill sits behind, so the letterbox bars a non-3:4 cover leaves
// are the design's cover ground rather than a hole in the row.
func coverTile(res fyne.Resource, label string, width float32, dimmed bool) fyne.CanvasObject {
	size := fyne.NewSize(width, width*4/3)

	// The fallback ground is always drawn: it is the backdrop for a
	// cover that does not fill the frame, and the whole tile when there
	// is no cover at all.
	bg := canvas.NewRectangle(colFillCover)
	bg.StrokeColor = colEdge
	bg.StrokeWidth = 1
	bg.CornerRadius = 5
	layers := []fyne.CanvasObject{bg}

	if res != nil {
		img := canvas.NewImageFromResource(res)
		img.FillMode = canvas.ImageFillContain
		layers = append(layers, img)
	} else {
		// Trimmed to the tile: a fallback that overflows its own cover
		// and collides with the next tile is worse than a fallback that
		// says half the name.
		name := text(ellipsize(label, width-10, szMicro, fyne.TextStyle{}), colMist, szMicro)
		name.Alignment = fyne.TextAlignCenter
		layers = append(layers, container.NewCenter(name))
	}
	if dimmed {
		layers = append(layers, canvas.NewRectangle(dim(colInk, 0x99)))
	}
	// GridWrap pins the cell to exactly this size in both axes, so no
	// amount of spare width in the parent can stretch what is inside.
	return container.NewGridWrap(size, container.NewStack(layers...))
}

// --- the redesign's structural pieces ---

// sectionLabel is the mono uppercase rule the page divides itself with:
// "CHECKED OUT TO YOU", "FREE TO TAKE · 3". It replaces the gold serif
// heading the old page used, because these labels are *counts and
// states* — machine facts — and the type split in this app puts machine
// facts in the mono face.
//
// The design specifies 0.12em tracking, and Fyne has no letter-spacing.
// An earlier cut faked it by putting a space between every character,
// which at mono widths came out as "C H E C K E D  O U T  T O  Y O U" —
// a label nobody could read as a word. Uppercase mono at a small size
// already reads as a section rule, so the tracking is simply dropped:
// the honest approximation is no approximation.
func sectionLabel(title string, c color.Color, sub string) fyne.CanvasObject {
	t := monoText(strings.ToUpper(title), c, szMicro)
	if sub == "" {
		return container.NewPadded(container.NewHBox(t))
	}
	// Padded between the two, or the sub-label runs straight into the
	// label — an HBox packs its children edge to edge.
	return container.NewPadded(container.NewHBox(t,
		container.NewPadded(text(sub, colMist, szCaption))))
}

// groupPanel is a custody group's bordered panel: one rounded box, its
// rows separated by single edge lines rather than each row being its
// own card.
//
// This is the structural change that makes the page read as three
// groups instead of six floating cards — a shared border is what says
// "these worlds are the same kind of thing", which is the whole reason
// the grouping replaces the per-row status prose.
func groupPanel(border color.Color, rows ...fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colPanel)
	bg.StrokeColor = border
	bg.StrokeWidth = 1
	bg.CornerRadius = 8

	stacked := container.NewVBox()
	for i, r := range rows {
		if i > 0 {
			stacked.Add(rule())
		}
		stacked.Add(r)
	}
	return container.NewStack(bg, stacked)
}

// quietWell is the dashed footnote box — the games-library pointer under
// the world groups, and the offline "the rest are hidden" note.
// Fyne cannot stroke a dashed rectangle, so it is drawn as a well with
// a plain edge: quieter than a panel, unmistakably not a row.
func quietWell(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(colWell)
	bg.StrokeColor = colEdge
	bg.StrokeWidth = 1
	bg.CornerRadius = 8
	return container.NewStack(bg, container.NewPadded(container.NewPadded(content)))
}

// badge is the small bordered mono tag: the hold countdown, and
// anything else that is a measurement wearing a border.
func badge(label string, c color.NRGBA) fyne.CanvasObject {
	bg := canvas.NewRectangle(color.Transparent)
	bg.StrokeColor = dim(c, 0x80)
	bg.StrokeWidth = 1
	bg.CornerRadius = 3
	return container.NewStack(bg, container.NewPadded(container.NewHBox(monoText(label, c, szMicro))))
}

// linkText is a word that acts like a link: gold, and tappable. The
// design uses these for the few in-prose navigations — "Open the games
// library", "Show them".
func linkText(label string, onTap func()) fyne.CanvasObject {
	t := text(label, colGoldHi, szCaption)
	tap := newTapArea(onTap)
	return container.NewStack(container.NewHBox(t), tap)
}

// stepMarker is the first-run checklist's 22 px circle: a tick for a
// step already true, a number for the one still to do.
func stepMarker(label string, c color.NRGBA) fyne.CanvasObject {
	ring := canvas.NewCircle(color.Transparent)
	ring.StrokeColor = c
	ring.StrokeWidth = 1
	return container.NewGridWrap(fyne.NewSize(22, 22),
		container.NewStack(ring, container.NewCenter(monoText(label, c, szMicro))))
}

// wrapLines breaks a label into at most n lines that each fit a width,
// ellipsizing only the last one.
//
// The games grid asks for this: a tile's name must wrap to two lines
// rather than truncate, because "RuneScape: Dragonwilds" cut to
// "RuneScape: Dra…" tells nobody which game it is. canvas.Text neither
// wraps nor truncates, and widget.Label takes the theme's foreground
// rather than the palette colour a tile needs, so the breaking is done
// here against the same font metrics the text is drawn with.
func wrapLines(s string, max float32, n int, size float32, style fyne.TextStyle) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	cur := ""
	for _, w := range words {
		try := w
		if cur != "" {
			try = cur + " " + w
		}
		if fyne.MeasureText(try, size, style).Width <= max || cur == "" {
			cur = try
			continue
		}
		lines = append(lines, cur)
		cur = w
		if len(lines) == n-1 {
			break
		}
	}
	lines = append(lines, cur)
	// Whatever did not fit in the allowed lines is folded back into the
	// last one and trimmed there, so nothing is silently dropped.
	if used := len(lines); used == n {
		rest := strings.Join(words[wordsUsed(lines[:used-1]):], " ")
		lines[used-1] = ellipsize(rest, max, size, style)
	}
	return lines
}

func wordsUsed(lines []string) int {
	n := 0
	for _, l := range lines {
		n += len(strings.Fields(l))
	}
	return n
}

// dot is the header's connection light: green connected, ember when the
// last poll failed, mist when nothing is configured.
func dot(c color.NRGBA) fyne.CanvasObject {
	d := canvas.NewCircle(c)
	d.Resize(fyne.NewSize(7, 7))
	return container.NewGridWrap(fyne.NewSize(7, 7), d)
}

// rule is a full-width divider in the vault's edge colour.
func rule() fyne.CanvasObject {
	r := canvas.NewRectangle(colEdge)
	r.SetMinSize(fyne.NewSize(1, 1))
	return r
}

// sectionHeader is the gold small-caps heading the page divides itself
// with, with its actions pushed to the right.
// The parameter is `acts` rather than `actions` because actions() is
// now the function that lays a row of buttons out — see button.go.
func sectionHeader(title, hint string, acts ...fyne.CanvasObject) fyne.CanvasObject {
	left := container.NewHBox(serifBold(title, colGold, szSubhead))
	if hint != "" {
		left.Add(italicText(hint, colMist, szCaption))
	}
	if len(acts) == 0 {
		return left
	}
	return container.NewBorder(nil, nil, left, container.NewHBox(acts...))
}

// --- headings ---

// serifText is the vault's voice: Gelasio, asked for explicitly through
// canvas.Text's FontSource because Fyne's theme has no heading style bit
// to hang it on.
//
// Only headings and titles use it. See the note on the faces in
// theme.go: unhinted at body sizes the serif is what made the first cut
// of this window hard to read, and at heading sizes it is what makes the
// window look like the vault rather than like a toolkit demo.
func serifText(s string, c color.Color, size float32) *canvas.Text {
	t := text(s, c, size)
	t.FontSource = resSerif
	return t
}

func serifBold(s string, c color.Color, size float32) *canvas.Text {
	t := text(s, c, size)
	t.TextStyle = fyne.TextStyle{Bold: true}
	t.FontSource = resSerifBold
	return t
}

// pathLine is a filesystem path on one line, ellipsized to whatever
// width it is given at layout time.
//
// A canvas.Text cannot do this: it needs the width up front, and a
// world's folder is as long as the player's folder is long. Left
// unbounded it ran off the right edge of its card — the row's own
// panel border was drawn past the end of the text. widget.Label's
// truncation happens during layout, which is the only place the real
// width is known.
func pathLine(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.TextStyle = fyne.TextStyle{Monospace: true}
	l.Truncation = fyne.TextTruncateEllipsis
	l.SizeName = theme.SizeNameCaptionText
	l.Importance = widget.LowImportance
	return l
}

// ellipsize trims a label to fit and marks that it did.
//
// A shelf tile is 130 px wide and a game's name is whatever its
// publisher felt like: "RuneScape: Dragonwilds" ran straight off its
// tile and into the next one's. Fyne's canvas.Text does not truncate —
// it draws the whole string and lets it overflow — so the trimming
// happens here, against a width measured with the same font metrics the
// text will be drawn with.
//
// The full name is not lost: the tile carries it as a hover tip.
func ellipsize(s string, max, size float32, style fyne.TextStyle) string {
	if max <= 0 || fyne.MeasureText(s, size, style).Width <= max {
		return s
	}
	runes := []rune(s)
	for n := len(runes) - 1; n > 0; n-- {
		candidate := strings.TrimRight(string(runes[:n]), " ") + "…"
		if fyne.MeasureText(candidate, size, style).Width <= max {
			return candidate
		}
	}
	return "…"
}
