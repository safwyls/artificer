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

// The bundled faces. Gelasio is the Georgia-metric serif the web UI's
// own font stack names (`Georgia, Gelasio, "Times New Roman", serif`) —
// on a player's Windows machine that stack resolves to Georgia, and
// Gelasio is the metric-compatible open face that matches it. JetBrains
// Mono carries paths, versions and badges, which is everywhere the web
// UI reaches for `font-mono`.
//
// Both are OFL; their licences sit beside them in fonts/.
//
//go:embed fonts/Gelasio-Regular.ttf
var fontRegular []byte

//go:embed fonts/Gelasio-Bold.ttf
var fontBold []byte

//go:embed fonts/Gelasio-Italic.ttf
var fontItalic []byte

//go:embed fonts/JetBrainsMono-Regular.ttf
var fontMono []byte

var (
	resRegular = fyne.NewStaticResource("Gelasio-Regular.ttf", fontRegular)
	resBold    = fyne.NewStaticResource("Gelasio-Bold.ttf", fontBold)
	resItalic  = fyne.NewStaticResource("Gelasio-Italic.ttf", fontItalic)
	resMono    = fyne.NewStaticResource("JetBrainsMono-Regular.ttf", fontMono)
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

// Font hands back the bundled faces. Anything monospace gets JetBrains
// Mono whatever else the style says, because the web UI's mono spans are
// never also bold or italic.
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
// global corner. Text sizes follow the web UI's 13/11 px body-and-
// caption pairing, one step up because a desktop window is read at a
// slightly greater distance than a browser tab.
func (vaultTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return 14
	case theme.SizeNameCaptionText:
		return 12
	case theme.SizeNameSubHeadingText:
		return 16
	case theme.SizeNameHeadingText:
		return 19
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
