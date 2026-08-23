# Reliquary Companion — the Wails desktop shell

Status: **in-development parallel build** (started 2026-08-22). The
browser-and-tray build (`cmd/companion`, released as
`artificer-companion.exe`) remains the shipping companion until this one
has passed a real-machine smoke; then it cuts over and the old build
retires.

## What it is

The same save-sync client, in its own native window instead of a browser
tab. The engine — discovery, custody sync, the `/api` surface, in-place
updates — was extracted from `cmd/companion` into the importable
`companion/` package (2026-08-22, this feature's enabling refactor), and
two entrypoints now wrap it:

- `cmd/companion` — the original shell: local server on `127.0.0.1:8377`,
  default browser as the window, Windows systray as the handle.
- `companion-wails/` — this shell: a Wails v2 window (WebView2 on
  Windows) whose asset server serves the embedded `web/companion`
  frontend directly and falls through to the same `Routes()` handler for
  `/api`. **Its own Go module**, because Wails' Linux target needs
  webkit2gtk headers via CGO and the root module's `go build ./...` must
  stay dependency-free; the Windows target is CGO-free and cross-builds
  from Linux (that is what CI verifies).

Both builds share one frontend, one config file, one custody state and
one frozen local address, so **only one may sync at a time**: each
refuses to start when `127.0.0.1:8377` is already a live companion.

## Identity

The desktop build ships under its own name so the two updaters can never
replace each other: window title "Reliquary Companion", binary
`reliquary-companion.exe`, rolling release tag
`reliquary-companion-latest` (workflow
`release-reliquary-companion.yml`). The entrypoint pins
`companion.UpdateTag` and `companion.UpdateAssets` before starting the
update watcher; the browser build keeps the frozen
`artificer-companion.exe` names on `companion-latest`.

## Building

```
cd web/companion && npm run build   # the exe embeds dist/
cd companion-wails
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -tags desktop,production -ldflags="-H windowsgui" \
  -o reliquary-companion.exe .
```

A native Linux build needs `libgtk-3-dev libwebkit2gtk-4.0-dev` and
plain CGO; it exists for development only and is not released. `wails
dev` works from `companion-wails/` with the Wails CLI installed
(`wails.json` points the frontend at `../web/companion`).

## Not done yet (the cutover gate)

- No real-Windows smoke: window opens, discovery, link, a
  checkout/check-in round trip, self-update from its own tag — the same
  list `docs/companion-ui-rebuild.md` phase 3 still owes the browser
  build.
- No exe icon: `cmd/companion` carries `rsrc_windows_amd64.syso`; this
  build needs its own (same favicon) plus the Wails window icon.
- The reliquary image bundles only the browser build's exe for its
  token-gated download; the cutover decides whether it switches or
  carries both.
- Cutover itself: retire `cmd/companion`'s release, point players'
  download links at the new asset (frozen-name rule says that is a
  migration, not an edit).
