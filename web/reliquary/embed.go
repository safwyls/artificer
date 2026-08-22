// Package web embeds the built Reliquary frontend (web/reliquary/dist,
// produced by `npm run build`) into the reliquary binary — the same
// pattern the three consoles use.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Dist returns the embedded frontend rooted at dist/, so callers see
// index.html etc. directly rather than under a dist/ prefix.
func Dist() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
