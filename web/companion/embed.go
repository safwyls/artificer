// Package web embeds the built Artificer Companion frontend
// (web/companion/dist, produced by `npm run build`) into the companion
// binary. The single-binary, no-installer shape is the point: a player
// downloads one exe, and the page it serves on loopback is inside it.
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
