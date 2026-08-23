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
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type tapArea struct {
	widget.BaseWidget
	onTap func()
	// hover paints the vault's hover wash over the tile, so a tile still
	// answers the pointer the way the web shelf's does.
	hover *canvas.Rectangle
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

func (t *tapArea) MouseIn(*desktop.MouseEvent) {
	t.hover.FillColor = dim(colGold, 0x1c)
	t.hover.Refresh()
}

func (t *tapArea) MouseMoved(*desktop.MouseEvent) {}

func (t *tapArea) MouseOut() {
	t.hover.FillColor = dim(colGold, 0x00)
	t.hover.Refresh()
}

// setEntryText writes a value into an entry from any goroutine.
func setEntryText(entry *widget.Entry, value string) {
	fyne.Do(func() { entry.SetText(value) })
}
