// Package docs embeds the project documentation so the API can serve it to
// the pal advisor's docs-search tool. The advisor's tools execute in the
// browser, but these files live in the repo, not the web bundle — so the
// binary carries them and one authenticated endpoint hands them over
// (main assigns FS to the server's DocsFS).
package docs

import "embed"

//go:embed *.md
var FS embed.FS
