package main

// The vault, as a Fyne theme.
//
// design-system/lib/vault.mjs is the source of truth for these eleven
// colours: it is the palette reliquary and the companion share verbatim,
// because they are two halves of one custody flow rather than two
// products that happen to match. The hexes below are that module's,
// name for name. If one moves there, it moves here.
//
// Fyne draws its own widgets, so this cannot reproduce the web UI
// pixel-for-pixel and does not try: there are no gradients on buttons,
// and a few things the CSS does with a border-left accent are done here
// with a coloured rule. What the theme *can* carry — ground, panel,
// edge, text, the gold accent, the three status colours, the radii and
// the serif — it carries exactly.

import (
	_ "embed"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The vault palette. One definition, used by the theme below and by the
// custom widgets that draw outside Fyne's semantic colours (the custody
// chip, the cover tiles, the callout rules).
var (
	// ink is the page ground.
	colInk = hex(0x100d17)
	// well is a shade under the page: header and inset strips, so a
	// panel on it still reads as raised.
	colWell = hex(0x14101d)
	// panel is cards, dialogs, menus.
	colPanel = hex(0x1a1524)
	// edge is every border and divider.
	colEdge = hex(0x2f2740)
	// parchment is body text.
	colParchment = hex(0xe8e0cf)
	// mist is secondary text, quiet buttons, labels.
	colMist = hex(0x948da3)
	// gold is the accent: headings, the primary button, the focus ring.
	colGold = hex(0xc9a860)
	// goldhi is accent hover, links, the HEAD badge.
	colGoldHi = hex(0xe3c67f)
	// ok is free custody, the live dot, success.
	colOK = hex(0x7fc46a)
	// ember is danger, conflict, an expired hold.
	colEmber = hex(0xd4735e)
	// rune is game tags, info, the CHECKPOINT badge.
	colRune = hex(0x9d7fc4)

	// The chip fills: each an accent laid over the ground at low weight.
	// vault.mjs calls these compounds of the palette rather than members
	// of it, and they are the only literal colours either app writes
	// outside its :root.
	colFillFree    = hex(0x14200f)
	colFillHeld    = hex(0x23180c)
	colFillExpired = hex(0x26130e)
	// colFillCover stands in for the web UI's cover gradient — Fyne has
	// no gradient fill for a rectangle, so the darker stop is used flat.
	colFillCover = hex(0x221b2e)
)

func hex(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}

// dim returns a colour at partial alpha, for the wash behind a callout
// and for the veil over an unlinked cover.
func dim(c color.NRGBA, alpha uint8) color.NRGBA {
	c.A = alpha
	return c
}

// The bundled faces, and the one rule that governs which is used where.
//
// The first cut of this theme set Gelasio — the Georgia-metric serif the
// web UI's font stack names — as the face for *everything*, on the
// reasoning that the web CSS does the same. On a real Windows machine
// that turned out to be the single biggest thing wrong with the window.
// A browser hands its text to the OS rasterizer, which applies Georgia's
// hinting instructions and snaps its stems to the pixel grid; Fyne
// rasterizes glyphs itself through golang.org/x/image, which executes no
// hinting at all. An old-style serif with modulated strokes and fine
// bracketed slabs is precisely the design that depends on that hinting:
// unhinted at 13 px it comes out soft, uneven and tiring to read, which
// is exactly what the screenshots showed.
//
// So the faces are split by job rather than used uniformly:
//
//   - Source Sans 3 carries body text, labels, controls and everything
//     small. It is a humanist sans drawn for screen UI — large x-height,
//     open apertures, near-uniform stem weight — which is the shape that
//     survives an unhinted rasterizer at small sizes. Semibold rather
//     than Bold stands in for bold: on a dark ground a true Bold blooms.
//   - Gelasio is kept for headings and titles only, at sizes (17 px and
//     up) where a serif has enough pixels to be a serif. That is where
//     the vault's voice actually comes from; the rest of the mood is
//     carried by colour and spacing, which cost no legibility at all.
//   - JetBrains Mono still carries paths, versions and badges, which is
//     everywhere the web UI reaches for `font-mono`.
//
// All three are OFL; their licences sit beside them in fonts/.
//
//go:embed fonts/SourceSans3-Regular.ttf
var fontRegular []byte

//go:embed fonts/SourceSans3-Semibold.ttf
var fontBold []byte

//go:embed fonts/SourceSans3-It.ttf
var fontItalic []byte

//go:embed fonts/Gelasio-Regular.ttf
var fontSerif []byte

//go:embed fonts/Gelasio-Bold.ttf
var fontSerifBold []byte

//go:embed fonts/JetBrainsMono-Regular.ttf
var fontMono []byte

var (
	resRegular   = fyne.NewStaticResource("SourceSans3-Regular.ttf", fontRegular)
	resBold      = fyne.NewStaticResource("SourceSans3-Semibold.ttf", fontBold)
	resItalic    = fyne.NewStaticResource("SourceSans3-It.ttf", fontItalic)
	resSerif     = fyne.NewStaticResource("Gelasio-Regular.ttf", fontSerif)
	resSerifBold = fyne.NewStaticResource("Gelasio-Bold.ttf", fontSerifBold)
	resMono      = fyne.NewStaticResource("JetBrainsMono-Regular.ttf", fontMono)
)

// The type scale, named rather than sprinkled as literals.
//
// Everything moved up: Fyne sizes are density-independent points that
// the driver multiplies by the display scale, and the previous scale
// bottomed out at 10, which is unreadable once an unhinted rasterizer
// has had its way with it. Nothing here is below 12, and body sits at
// 15 — a desktop window is read at a greater distance than a browser
// tab, and this window is mostly prose about custody.
const (
	// szMicro is the smallest thing this app is allowed to say: a tile
	// caption, a probe path, a chip.
	szMicro = 12
	// szCaption is secondary lines — field labels, explanations, the
	// sentence under a world's name.
	szCaption = 13
	// szBody is ordinary text, and matches the theme's own text size so
	// a canvas.Text sits level with a widget's label beside it.
	szBody = 15
	// szSubhead is a world's name and a card's heading.
	szSubhead = 17
	// szTitle is the window's own title, in the serif.
	szTitle = 22
)

// vaultTheme is the whole look. It answers every colour itself rather
// than falling through to Fyne's default for the ones it does not name:
// a theme that half-answers produces a window that is mostly the vault
// with Fyne's blue focus ring in it, which reads as a bug.
type vaultTheme struct{}

var _ fyne.Theme = vaultTheme{}

// Color maps Fyne's semantic names onto the palette. The variant is
// ignored on purpose: the vault is a dark design language, and a light
// mode of it does not exist — a window that turned parchment-on-white
// under a system setting would not be this app.
func (vaultTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return colInk
	case theme.ColorNameHeaderBackground, theme.ColorNameInputBackground:
		return colWell
	case theme.ColorNameMenuBackground, theme.ColorNameOverlayBackground:
		return colPanel
	case theme.ColorNameButton:
		// Quiet by default: the vault's ordinary button is a panel with an
		// edge, and only the primary one wears gold.
		return colPanel
	case theme.ColorNameDisabledButton:
		return colWell
	case theme.ColorNameForeground:
		return colParchment
	case theme.ColorNameForegroundOnPrimary, theme.ColorNameForegroundOnError,
		theme.ColorNameForegroundOnSuccess, theme.ColorNameForegroundOnWarning:
		// Text laid on an accent fill. The accents are all light enough
		// that ink is the readable choice.
		return colInk
	case theme.ColorNameDisabled, theme.ColorNamePlaceHolder:
		return colMist
	case theme.ColorNamePrimary, theme.ColorNameFocus:
		return colGold
	case theme.ColorNameHyperlink:
		return colGoldHi
	case theme.ColorNameHover:
		// A wash of gold rather than gold itself: hover must not read as
		// selection.
		return dim(colGold, 0x22)
	case theme.ColorNamePressed:
		return dim(colGold, 0x44)
	case theme.ColorNameSelection:
		return dim(colGold, 0x55)
	case theme.ColorNameSeparator, theme.ColorNameInputBorder,
		theme.ColorNameInnerWindowBorder, theme.ColorNameInnerWindowBorderInactive:
		return colEdge
	case theme.ColorNameScrollBar:
		return dim(colMist, 0x88)
	case theme.ColorNameScrollBarBackground:
		return colWell
	case theme.ColorNameShadow:
		return color.NRGBA{A: 0x77}
	case theme.ColorNameError:
		return colEmber
	case theme.ColorNameSuccess:
		return colOK
	case theme.ColorNameWarning:
		return colGoldHi
	}
	return theme.DefaultTheme().Color(name, theme.VariantDark)
}

// Font hands back the bundled faces for everything Fyne draws itself —
// which is body text, labels and every stock widget, so this answers
// with the sans. The serif is not reachable from here: Fyne's theme has
// no "this is a heading" style bit, so headings ask for it explicitly
// through serifText below, which sets canvas.Text.FontSource.
//
// Anything monospace gets JetBrains Mono whatever else the style says,
// because the web UI's mono spans are never also bold or italic.
func (vaultTheme) Font(style fyne.TextStyle) fyne.Resource {
	switch {
	case style.Monospace:
		return resMono
	case style.Symbol:
		// Fyne's own symbol face: the palette has no opinion about it,
		// and Gelasio has no symbols to offer.
		return theme.DefaultTheme().Font(style)
	case style.Bold:
		return resBold
	case style.Italic:
		return resItalic
	}
	return resRegular
}

func (vaultTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

// Size carries the vault's two radii — 8 px for panels, 4 px for inputs
// — which Fyne 2.8 can express per widget class rather than as one
// global corner, and the type scale above.
//
// On scaling: every number here is density-independent. Fyne detects the
// display scale itself — on Windows that is the per-monitor DPI, so a
// 150% display gets 150% of these sizes without anything in this app
// being involved, and nothing here or in main.go sets fyne.Settings'
// scale or the FYNE_SCALE environment variable. That is deliberate: an
// app that pins its own scale is an app that ignores the player's
// display settings. FYNE_SCALE remains available to the player as an
// escape hatch — `FYNE_SCALE=1.25` before launching nudges the whole UI
// up or down when Windows' own scaling is not to their taste.
func (vaultTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return szBody
	case theme.SizeNameCaptionText:
		return szCaption
	case theme.SizeNameSubHeadingText:
		return szSubhead
	case theme.SizeNameHeadingText:
		return 20
	case theme.SizeNameInputRadius, theme.SizeNameButtonRadius,
		theme.SizeNameSelectionRadius, theme.SizeNameScrollBarRadius:
		return 4
	case theme.SizeNameCardRadius, theme.SizeNameDialogRadius,
		theme.SizeNamePopupRadius, theme.SizeNameMenuRadius:
		return 8
	case theme.SizeNamePadding:
		return 5
	case theme.SizeNameInnerPadding:
		return 9
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameInputBorder:
		return 1
	}
	return theme.DefaultTheme().Size(name)
}
