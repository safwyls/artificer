package main

// tapArea is an invisible click target laid over a composed tile.
//
// The shelf's tiles are a cover, a name and a caption stacked inside a
// bordered frame, and all of it has to be one click. Fyne's button would
// paint its own fill and its own hover over that composition, so the
// tile is drawn as it should look and this sits on top to catch the tap.

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type tapArea struct {
	widget.BaseWidget
	onTap func()
	// hover paints the vault's hover wash over the tile, so a tile still
	// answers the pointer the way the web shelf's does.
	hover *canvas.Rectangle

	// tip is the full text of whatever the tile had to trim, shown while
	// the pointer rests on it. Fyne 2.8 has no tooltip, so this is one:
	// a small panel-coloured popup, placed under the pointer, torn down
	// on the way out. It exists because a 148 px tile cannot hold
	// "RuneScape: Dragonwilds" and the caption has to ellipsize — the
	// name must still be readable somewhere.
	tip    string
	tipWin fyne.Window
	tipPop *widget.PopUp
	tipAt  fyne.Position
}

var (
	_ fyne.Tappable     = (*tapArea)(nil)
	_ desktop.Hoverable = (*tapArea)(nil)
	_ fyne.Widget       = (*tapArea)(nil)
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

func (t *tapArea) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

// setTip gives this area a hover tip. The window is needed because a
// Fyne popup belongs to a canvas rather than to the widget it explains.
func (t *tapArea) setTip(win fyne.Window, tip string) {
	t.tipWin, t.tip = win, tip
}

func (t *tapArea) MouseIn(e *desktop.MouseEvent) {
	t.hover.FillColor = dim(colGold, 0x1c)
	t.hover.Refresh()
	t.tipAt = e.AbsolutePosition
	t.showTip()
}

func (t *tapArea) MouseMoved(e *desktop.MouseEvent) { t.tipAt = e.AbsolutePosition }

func (t *tapArea) MouseOut() {
	t.hover.FillColor = dim(colGold, 0x00)
	t.hover.Refresh()
	t.hideTip()
}

func (t *tapArea) showTip() {
	if t.tip == "" || t.tipWin == nil || t.tipPop != nil {
		return
	}
	bg := canvas.NewRectangle(colWell)
	bg.StrokeColor = colEdge
	bg.StrokeWidth = 1
	bg.CornerRadius = 4
	label := text(t.tip, colParchment, szCaption)
	content := container.NewStack(bg, container.NewPadded(label))
	pop := widget.NewPopUp(content, t.tipWin.Canvas())
	// Offset below the pointer so the tip does not sit under the cursor
	// and immediately take the hover it was triggered by.
	pop.ShowAtPosition(t.tipAt.AddXY(10, 20))
	t.tipPop = pop
}

func (t *tapArea) hideTip() {
	if t.tipPop == nil {
		return
	}
	t.tipPop.Hide()
	t.tipPop = nil
}

// setEntryText writes a value into an entry from any goroutine.
func setEntryText(entry *widget.Entry, value string) {
	fyne.Do(func() { entry.SetText(value) })
}
