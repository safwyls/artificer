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

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
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
	t := text(label, fg, 11)
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
func coverTile(res fyne.Resource, label string, width float32, dimmed bool) fyne.CanvasObject {
	size := fyne.NewSize(width, width*4/3)
	var art fyne.CanvasObject
	if res != nil {
		img := canvas.NewImageFromResource(res)
		img.FillMode = canvas.ImageFillStretch
		img.SetMinSize(size)
		art = img
	} else {
		bg := canvas.NewRectangle(colFillCover)
		bg.SetMinSize(size)
		name := text(label, colMist, 11)
		name.Alignment = fyne.TextAlignCenter
		art = container.NewStack(bg, container.NewCenter(name))
	}
	if !dimmed {
		return art
	}
	veil := canvas.NewRectangle(dim(colInk, 0x99))
	veil.SetMinSize(size)
	return container.NewStack(art, veil)
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
func sectionHeader(title, hint string, actions ...fyne.CanvasObject) fyne.CanvasObject {
	left := container.NewHBox(boldText(title, colGold, 13))
	if hint != "" {
		left.Add(italicText(hint, colMist, 11))
	}
	if len(actions) == 0 {
		return left
	}
	return container.NewBorder(nil, nil, left, container.NewHBox(actions...))
}

// primaryButton is the one gold button in a group. Fyne's HighImportance
// paints it with the theme's primary colour, which is the vault's gold.
func primaryButton(label string, tapped func()) *widget.Button {
	b := widget.NewButton(label, tapped)
	b.Importance = widget.HighImportance
	return b
}

// dangerButton is for the verbs that take something away — a takeover,
// an unlink. Ember, and never the only button in a dialog.
func dangerButton(label string, tapped func()) *widget.Button {
	b := widget.NewButton(label, tapped)
	b.Importance = widget.DangerImportance
	return b
}
