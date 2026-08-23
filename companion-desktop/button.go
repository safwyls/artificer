package main

// The vault's button, because Fyne's has no edge.
//
// This is the widget the first cut of the desktop app got most visibly
// wrong, and the reason is worth writing down: a Fyne button is a
// rounded *fill* and a label, and nothing else. It has no border, ever.
// Feed that renderer the vault's palette and two bad things happen at
// once:
//
//   - An ordinary button fills itself with ColorNameButton, which in
//     this theme is the panel colour. Put it on a dialog — which is also
//     panel — and the fill disappears, leaving centred text floating
//     with no chrome. That is exactly what "Save name", "Save folder",
//     "Close" and "Check for update" looked like on Windows.
//   - A HighImportance button fills itself *solid* with the primary
//     colour. The vault's primary is gold, so "Save & connect" came out
//     as a slab of gold — and because a Fyne container stretches its
//     child to the available width, a gold slab the width of the window.
//
// The design's vocabulary is the other way round: a primary action is a
// compact gold-*bordered* button with a faint gold wash inside it and
// gold text; a quiet action is the same shape in the edge colour; a
// destructive one is the same shape in ember. Fyne cannot express any of
// that through the theme, so the button is drawn here.
//
// It is also compact by construction. MinSize is the label plus padding,
// and `actions()` below packs a row of them in an HBox, which lays its
// children out at their minimum width — so a button is the size of what
// it says, and the full-window gold bar cannot happen by accident. The
// one place a stretched button is right (the first-run card's single
// call to action, which is narrow already) asks for it explicitly.

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// buttonTone is the three weights a button comes in. They differ only in
// colour: shape, padding and behaviour are identical, so a row of mixed
// buttons still reads as one row.
type buttonTone int

const (
	// tonePrimary is the one action a screen or dialog is *for*. Gold
	// border, gold text, a wash of gold inside.
	tonePrimary buttonTone = iota
	// toneQuiet is everything else. Edge border, parchment text, panel
	// fill — visible chrome, no claim on the eye.
	toneQuiet
	// toneDanger takes something away. Ember, and never alone in a
	// dialog.
	toneDanger
)

// buttonPadX and buttonPadY are the button's own breathing room. Wider
// than tall on purpose: a compact button still has to look like a
// target, and horizontal padding is what makes a short label ("Edit",
// "Play") read as a button rather than as a word in a box.
const (
	buttonPadX = 14
	buttonPadY = 7
	// iconGap separates an icon from its label, and iconSize is the
	// square an icon is drawn in.
	iconGap  = 6
	iconSize = 15
)

type vaultButton struct {
	widget.BaseWidget

	label string
	icon  fyne.Resource
	tone  buttonTone
	onTap func()

	disabled bool
	hovered  bool

	bg   *canvas.Rectangle
	text *canvas.Text
	img  *canvas.Image
}

var (
	_ fyne.Tappable     = (*vaultButton)(nil)
	_ fyne.Widget       = (*vaultButton)(nil)
	_ fyne.Disableable  = (*vaultButton)(nil)
	_ desktop.Hoverable = (*vaultButton)(nil)
)

func newVaultButton(label string, tone buttonTone, onTap func()) *vaultButton {
	b := &vaultButton{label: label, tone: tone, onTap: onTap}
	b.ExtendBaseWidget(b)
	return b
}

// fill, stroke and ink are the three colours a tone resolves to, given
// whether the button is disabled or hovered.
func (b *vaultButton) colors() (fill, stroke, ink color.Color) {
	if b.disabled {
		return colWell, colEdge, colMist
	}
	switch b.tone {
	case tonePrimary:
		fill, stroke, ink = dim(colGold, 0x22), colGold, colGoldHi
		if b.hovered {
			fill = dim(colGold, 0x3a)
		}
	case toneDanger:
		fill, stroke, ink = dim(colEmber, 0x1c), colEmber, colEmber
		if b.hovered {
			fill = dim(colEmber, 0x33)
		}
	default:
		fill, stroke, ink = colPanel, colEdge, colParchment
		if b.hovered {
			fill, stroke = dim(colGold, 0x18), colGold
		}
	}
	return fill, stroke, ink
}

func (b *vaultButton) CreateRenderer() fyne.WidgetRenderer {
	b.bg = canvas.NewRectangle(colPanel)
	b.bg.StrokeWidth = 1
	b.bg.CornerRadius = 4
	b.text = canvas.NewText(b.label, colParchment)
	b.text.TextSize = szCaption
	b.text.TextStyle = fyne.TextStyle{Bold: true}
	b.text.Alignment = fyne.TextAlignCenter
	if b.icon != nil {
		b.img = canvas.NewImageFromResource(b.icon)
		b.img.FillMode = canvas.ImageFillContain
	}
	r := &vaultButtonRenderer{b: b}
	r.applyColors()
	return r
}

func (b *vaultButton) Tapped(*fyne.PointEvent) {
	if b.disabled || b.onTap == nil {
		return
	}
	b.onTap()
}

func (b *vaultButton) MouseIn(*desktop.MouseEvent) {
	if b.disabled {
		return
	}
	b.hovered = true
	b.Refresh()
}

func (b *vaultButton) MouseMoved(*desktop.MouseEvent) {}

func (b *vaultButton) MouseOut() {
	b.hovered = false
	b.Refresh()
}

func (b *vaultButton) Enable()        { b.disabled = false; b.Refresh() }
func (b *vaultButton) Disable()       { b.disabled = true; b.hovered = false; b.Refresh() }
func (b *vaultButton) Disabled() bool { return b.disabled }
func (b *vaultButton) Cursor() desktop.Cursor {
	if b.disabled {
		return desktop.DefaultCursor
	}
	return desktop.PointerCursor
}

type vaultButtonRenderer struct {
	b *vaultButton
}

func (r *vaultButtonRenderer) applyColors() {
	fill, stroke, ink := r.b.colors()
	r.b.bg.FillColor = fill
	r.b.bg.StrokeColor = stroke
	r.b.text.Color = ink
}

func (r *vaultButtonRenderer) Objects() []fyne.CanvasObject {
	out := []fyne.CanvasObject{r.b.bg}
	if r.b.img != nil {
		out = append(out, r.b.img)
	}
	if r.b.text.Text != "" {
		out = append(out, r.b.text)
	}
	return out
}

func (r *vaultButtonRenderer) MinSize() fyne.Size {
	w, h := float32(0), float32(0)
	if r.b.text.Text != "" {
		s := fyne.MeasureText(r.b.text.Text, r.b.text.TextSize, r.b.text.TextStyle)
		w, h = s.Width, s.Height
	}
	if r.b.img != nil {
		w += iconSize
		if r.b.text.Text != "" {
			w += iconGap
		}
		if h < iconSize {
			h = iconSize
		}
	}
	return fyne.NewSize(w+buttonPadX*2, h+buttonPadY*2)
}

func (r *vaultButtonRenderer) Layout(size fyne.Size) {
	r.b.bg.Resize(size)
	r.b.bg.Move(fyne.NewPos(0, 0))

	textW := float32(0)
	if r.b.text.Text != "" {
		textW = fyne.MeasureText(r.b.text.Text, r.b.text.TextSize, r.b.text.TextStyle).Width
	}
	contentW := textW
	if r.b.img != nil {
		contentW += iconSize
		if textW > 0 {
			contentW += iconGap
		}
	}
	x := (size.Width - contentW) / 2
	if x < buttonPadX {
		x = buttonPadX
	}
	if r.b.img != nil {
		r.b.img.Resize(fyne.NewSquareSize(iconSize))
		r.b.img.Move(fyne.NewPos(x, (size.Height-iconSize)/2))
		x += iconSize + iconGap
	}
	if r.b.text.Text != "" {
		th := r.b.text.MinSize().Height
		r.b.text.Resize(fyne.NewSize(textW, th))
		r.b.text.Move(fyne.NewPos(x, (size.Height-th)/2))
	}
}

func (r *vaultButtonRenderer) Refresh() {
	r.applyColors()
	r.b.text.Text = r.b.label
	r.b.text.Refresh()
	r.b.bg.Refresh()
	if r.b.img != nil {
		r.b.img.Refresh()
	}
	canvas.Refresh(r.b)
}

func (r *vaultButtonRenderer) Destroy() {}

// --- the constructors the screens actually call ---

// primaryButton is the one gold button in a group: a gold edge and gold
// text, sized to its label.
func primaryButton(label string, tapped func()) *vaultButton {
	return newVaultButton(label, tonePrimary, tapped)
}

// quietButton is every other verb. It has a visible edge, which is the
// whole difference from what Fyne would have drawn.
func quietButton(label string, tapped func()) *vaultButton {
	return newVaultButton(label, toneQuiet, tapped)
}

// dangerButton is for the verbs that take something away — a takeover,
// an unlink.
func dangerButton(label string, tapped func()) *vaultButton {
	return newVaultButton(label, toneDanger, tapped)
}

// iconPrimary is the primary carrying a glyph — the play triangle on
// "Play" and "Check out & play". The icon is the fastest way to tell
// the two checkout verbs apart at a glance, which is exactly what the
// old four-button row could not do.
func iconPrimary(label string, icon fyne.Resource, tapped func()) *vaultButton {
	b := newVaultButton(label, tonePrimary, tapped)
	b.icon = icon
	return b
}

// iconButton is a quiet button carrying an icon, with or without a
// label. The settings gear in the header is the label-less case.
func iconButton(label string, icon fyne.Resource, tapped func()) *vaultButton {
	b := newVaultButton(label, toneQuiet, tapped)
	b.icon = icon
	return b
}

// wideButton is a primary that fills the width it is given. The first-run
// card's single call to action is the only caller: that card is a narrow
// column, and a button spanning it reads as the card's own footer rather
// than as a bar across the window.
func wideButton(label string, tapped func()) *vaultButton {
	// Nothing special is needed to make it stretch: a Fyne container
	// that is not an HBox already resizes its child to the width it has.
	// "Wide" here is a statement of intent at the call site — this one is
	// meant to fill its card, and it is not going through actions().
	return newVaultButton(label, tonePrimary, tapped)
}

// actions packs a row of buttons at their natural widths, left-aligned,
// and is how every group of verbs in this app is laid out. An HBox lays
// children out at MinSize, so nothing in it can stretch — which is the
// property that makes "a button the width of the window" impossible
// rather than merely unintended.
func actions(objs ...fyne.CanvasObject) fyne.CanvasObject {
	return container.NewHBox(objs...)
}
