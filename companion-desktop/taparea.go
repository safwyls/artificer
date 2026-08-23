package main

// tapArea is an invisible click target laid over a composed tile.
//
// The shelf's tiles are a cover, a name and a caption stacked inside a
// bordered frame, and all of it has to be one click. Fyne's button would
// paint its own fill and its own hover over that composition, so the
// tile is drawn as it should look and this sits on top to catch the tap.
//
// It also grows the tooltip Fyne 2.8 does not have, and how that
// tooltip is *rendered* is the whole of a bug worth recording.
//
// The symptom, on a real Windows machine: hovering a tile made the
// cover art, the tooltip and the mouse cursor all flicker continuously
// while the pointer moved. The cause was that the tip was a
// widget.PopUp. A Fyne popup is not just a floating panel — it installs
// an overlay across the whole canvas, because that overlay is how
// clicking anywhere outside a popup dismisses it. So the moment the tip
// appeared, the overlay swallowed the pointer, the tile underneath
// received MouseOut and tore the tip down, the overlay went with it,
// the tile immediately got MouseIn again, and the tip came back. That
// loop ran at mouse-event rate: the tip strobed, the cursor changed on
// every hand-off between tile and overlay, and each teardown repainted
// the region under it — which is the cover art.
//
// Placing the popup away from the pointer does not help, and neither
// does a delay: the overlay is canvas-wide, so *no* position is outside
// it. Only not being a popup helps.
//
// So the tip is drawn into a plain, non-interactive layer stacked over
// the window's content (ui.tipLayer). Fyne's hit-testing looks for
// objects implementing Tappable or Hoverable; the tip is made of a
// canvas.Rectangle and a canvas.Text, which implement neither, so the
// pointer passes straight through it to the tile below and the tile
// never loses its hover. On top of that: the tip is shown at most once
// per hover, it waits out a short delay so sweeping across a grid
// raises none at all, MouseMoved does nothing whatsoever (a widget that
// writes state on every mouse event is a widget that repaints on every
// mouse event), and Cursor answers a constant so it cannot change while
// the pointer is inside.

import (
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// tipHost is whatever can draw a tooltip for this area — the window, in
// practice. Kept as an interface so the tap area does not reach into
// the ui's fields.
type tipHost interface {
	showTipAt(content fyne.CanvasObject, at fyne.Position)
	clearTip()
}

// tipDelay is how long the pointer must rest before a tip appears.
// Long enough that crossing a grid of tiles raises none of them, short
// enough that stopping on one still feels answered.
const tipDelay = 450 * time.Millisecond

// tipGap is the space between the widget's bottom edge and the tip, so
// the two never share a pixel and the pointer cannot fall into the tip
// while it is still inside the widget.
const tipGap = 6

type tapArea struct {
	widget.BaseWidget
	onTap func()
	// hover paints the vault's hover wash over the tile, so a tile still
	// answers the pointer the way the web shelf's does.
	hover *canvas.Rectangle

	// tip is the full text of whatever the tile had to trim. It exists
	// because a 148 px tile cannot hold "RuneScape: Dragonwilds" and the
	// caption has to ellipsize — the name must still be readable
	// somewhere.
	tip  string
	host tipHost

	// The tip's state is guarded because the delay timer fires on its
	// own goroutine while the pointer may already have left.
	mu      sync.Mutex
	inside  bool
	timer   *time.Timer
	showing bool
}

var (
	_ fyne.Tappable      = (*tapArea)(nil)
	_ desktop.Hoverable  = (*tapArea)(nil)
	_ desktop.Cursorable = (*tapArea)(nil)
	_ fyne.Widget        = (*tapArea)(nil)
)

func newTapArea(onTap func()) *tapArea {
	t := &tapArea{onTap: onTap}
	t.hover = canvas.NewRectangle(dim(colGold, 0x00))
	t.hover.CornerRadius = 6
	t.ExtendBaseWidget(t)
	return t
}

func (t *tapArea) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.hover)
}

// Cursor is a constant. Answering the same cursor for the whole of a
// hover is half of why the pointer no longer flickers over a tile.
func (t *tapArea) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (t *tapArea) Tapped(*fyne.PointEvent) {
	// A tip belongs to hovering, not to what happens next: whatever the
	// tap opens must not appear behind a leftover popup.
	t.hideTip()
	if t.onTap != nil {
		t.onTap()
	}
}

// setTip gives this area a hover tip. The host is needed because the
// tip is drawn into a layer over the whole window rather than inside
// this widget, which has only its own few pixels to draw in.
func (t *tapArea) setTip(host tipHost, tip string) {
	t.host, t.tip = host, tip
}

func (t *tapArea) MouseIn(*desktop.MouseEvent) {
	t.hover.FillColor = dim(colGold, 0x1c)
	t.hover.Refresh()

	if t.tip == "" || t.host == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.inside {
		// Already hovering as far as this widget is concerned. Nothing
		// to restart, and nothing to repaint.
		return
	}
	t.inside = true
	t.timer = time.AfterFunc(tipDelay, func() { fyne.Do(t.showTip) })
}

// MouseMoved does nothing, deliberately. The tip is placed against the
// widget rather than the pointer, so there is no per-event state to
// keep — and a widget that writes state on every mouse event is a
// widget that repaints on every mouse event.
func (t *tapArea) MouseMoved(*desktop.MouseEvent) {}

func (t *tapArea) MouseOut() {
	t.hover.FillColor = dim(colGold, 0x00)
	t.hover.Refresh()
	t.hideTip()
}

// showTip draws the tip below the widget, once. Runs on Fyne's thread.
func (t *tapArea) showTip() {
	t.mu.Lock()
	if !t.inside || t.showing || t.tip == "" || t.host == nil {
		t.mu.Unlock()
		return
	}
	t.showing = true
	t.mu.Unlock()

	// Every part of this is a canvas primitive rather than a widget.
	// That is the point: primitives implement neither Tappable nor
	// Hoverable, so Fyne's hit-testing looks straight past them to the
	// tile underneath and the hover that raised the tip survives it.
	bg := canvas.NewRectangle(colWell)
	bg.StrokeColor = colEdge
	bg.StrokeWidth = 1
	bg.CornerRadius = 4
	content := container.NewStack(bg, container.NewPadded(text(t.tip, colParchment, szCaption)))

	// Anchored to the widget, not the pointer: a tip that chases the
	// cursor is a tip that is never where you were about to read.
	at := fyne.CurrentApp().Driver().AbsolutePositionForObject(t)
	t.host.showTipAt(content, fyne.NewPos(at.X, at.Y+t.Size().Height+tipGap))
}

func (t *tapArea) hideTip() {
	t.mu.Lock()
	t.inside = false
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	was := t.showing
	t.showing = false
	host := t.host
	t.mu.Unlock()

	if was && host != nil {
		fyne.Do(host.clearTip)
	}
}

// setEntryText writes a value into an entry from any goroutine.
func setEntryText(entry *widget.Entry, value string) {
	fyne.Do(func() { entry.SetText(value) })
}
