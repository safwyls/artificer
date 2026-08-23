# Reliquary Companion — the native desktop shell

Status: **in-development parallel build**. The browser-and-tray build
(`cmd/companion`, released as `artificer-companion.exe`) remains the
shipping companion until this one has passed the parity checklist below
on a real Windows machine; then it cuts over and the old build retires.

History: this shell began on 2026-08-22 as a Wails v2 window
(`companion-wails/`, PR #69) that rendered the React frontend in
WebView2. It was rebuilt the same week as a real desktop application —
Fyne v2 widgets drawn in-process with the engine — and the Wails module
was deleted in that change. What the Wails cut established is all still
here: the parallel-app structure, the separate release identity, and the
engine extraction that made either shell possible. Only the webview is
gone. The native rebuild plan it was built to lands separately in PR #70
(add the pointer here once that has merged).

## What it is

The same save-sync client, as a proper desktop application. The engine —
discovery, custody sync, the `/api` surface, in-place updates — lives in
the importable `companion/` package, and two entrypoints wrap it:

- `cmd/companion` — the original shell: local server on `127.0.0.1:8377`,
  default browser as the window, Windows systray as the handle.
- `companion-desktop/` — this shell: a native window whose widgets are
  drawn by Fyne and fed directly by the engine, with no browser engine
  anywhere. **Its own Go module**, because Fyne needs CGO and OpenGL on
  every platform and the root module's `go build ./...` must stay
  toolchain-free.

"Native" is worth stating precisely: this is a real desktop application
— its own window and event loop, the OS's own folder dialogs, an OS tray,
OS notifications — but Fyne renders its own widgets rather than using
stock Win32 controls. The vault design language is carried by a theme
(`companion-desktop/theme.go`, tokens from `design-system/lib/vault.mjs`)
and a few small custom widgets, as closely as a theme API allows.

Both builds share one config file, one custody state and one frozen local
address, so **only one may sync at a time**: each refuses to start when
`127.0.0.1:8377` is already a live companion. The frozen page stays served
on that address by this build too — it is the "open it in a real browser"
escape hatch and the handover target for old builds.

### The in-process facade

The page polls `GET /api/state`; a UI in the same process does not need
to. `companion/facade.go` adds `Snapshot()` (the state the page renders,
as one exported struct — the HTTP handler marshals the same value),
`Subscribe()` (a coalescing change nudge), `MarkSeen()` (the presence
heartbeat that puts the custody poll into its fast mode — without it the
window shows custody up to a minute stale), typed action methods, and a
cover-art byte cache. No custody logic lives in the UI; anything it needs
is an addition to the engine.

### What the desktop shell does that the page cannot

- **Native folder picker** wherever a path is chosen. This is the one
  place the desktop app deliberately deletes web UI: the in-app
  `FolderBrowser` is gone from this window. `/api/browse` itself stays —
  frozen surface, and the browser page still needs it.
- **OS notifications**, for exactly three events and no others: a queued
  claim came through, a hold is nearing expiry, and a sync failed while
  the window was hidden. Toggleable in settings.
- **Close-to-tray.** The window is a view over a resident sync process;
  closing it hides it and syncing continues. Quit lives in the tray menu
  and confirms first while a transfer is running.
- **Autostart** — an HKCU `…\CurrentVersion\Run` entry passing
  `--minimized`, so login brings it up in the tray. Windows only; other
  platforms answer with a reason rather than a broken checkbox.
- **Second-launch raise** via one additive `POST /api/raise`. Old builds
  404 it harmlessly and the caller opens the page instead.
- **Window size persistence**, in `companion-desktop.json` beside the
  config — never in the shared config file, which the browser build
  reads. Size only: Fyne exposes no cross-platform window position.

## The window, reorganised around custody (2026-08-22)

The first native cut carried the browser build's information
architecture across verbatim: "Your worlds" as one small card above an
installed-games grid that took most of the screen. A maintainer-approved
redesign reorganises the window around the question the app exists to
answer — *can I take this world right now?* Five structural changes:

1. **Worlds is the whole page.** The installed-games grid became its own
   **Games** tab (`games.go`), with search, an All/Linked/Unlinked
   filter, linked and unlinked sections, two-line tile names and hidden
   entries as a line of prose rather than a fake tile in the grid.
2. **Worlds are grouped by what you can do with them** — checked out to
   you → free to take → held by someone else, plus a fourth group for
   worlds that have left the vault. The grouping replaces the per-row
   status prose.
3. **One primary action per row**, one quiet second, and everything rare
   or destructive behind a 3-dot overflow (open folder, copy path,
   rename, edit link, unlink).
4. **Sync state is reported exactly once** — the header's dot and one
   relative time. The scan trail, the tried paths, the linked folders
   and the build versions moved to **Settings › Diagnostics**, reachable
   from the footer too.
5. **Hold pressure is shown only when it is pressure** — a countdown
   badge appears on somebody else's hold inside its last three hours and
   not before.

The invariant the design asks to be preserved: **the chip and the
primary action are both derived from one `custodyOf(world)` result**
(`model.go`'s `custodyInfo`), so a row cannot say "Free" beside a
"Check in" button. `model_test.go` covers it.

Two derived states join the tabs rather than being tabs: **first run**
(connected, nothing linked — a three-step checklist) and **offline**
(configured, last poll failed — a banner, the held world still playable,
and the other worlds hidden behind "Show them read-only" because custody
cannot be confirmed).

What the design asked for that the engine cannot answer yet, and so is
**not** shipped rather than shipped empty: an Activity tab (the engine
has one overwritten `LastAction` string, not a feed), a Conflicts tab
(no conflict exists in the engine at all), "Ask for it back" (the
engine can queue you behind a holder — offered as **Ask to be next** —
but has no verb to *request* a return), "Download a copy" of a world
someone else holds, world history (only a single `Head`), and the
offline "queued to send" list (nothing tracks unsent work; the banner
ships without it). Each is an engine addition before it is a UI one.

## Identity

The desktop build ships under its own name so the two updaters can never
replace each other: window title "Reliquary Companion", binary
`reliquary-companion.exe`, rolling release tag
`reliquary-companion-latest` (workflow
`release-reliquary-companion.yml`). The entrypoint pins
`companion.UpdateTag` and `companion.UpdateAssets` before starting the
update watcher; the browser build keeps the frozen
`artificer-companion.exe` names on `companion-latest`. Those names, the
tag and the two manifest files (`companion-version.txt`,
`companion-sha256.txt`) survived the Wails→Fyne rebuild unchanged,
because installed dev builds pin them.

## Building

```
cd web/companion && npm run build   # the exe embeds dist/
cd companion-desktop
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc \
  go build -ldflags="-H windowsgui" -o reliquary-companion.exe .
```

The cross-build needs `gcc-mingw-w64-x86-64` (what CI installs). A native
Linux build additionally needs `libgl1-mesa-dev xorg-dev`; it exists for
development only and is not released or built in CI. The exe icon comes
from the committed `rsrc_windows_amd64.syso`, generated from
`web/companion/public/favicon.ico` — the same artwork as the tray, the
window icon and the page's favicon, and byte-identical to
`cmd/companion`'s. CI regenerates both and fails on a difference.

## Parity checklist (the cutover gate)

Everything here verified on a real Windows machine before
`cmd/companion` retires. **None of it is ticked**: the app has never been
run — it is cross-compiled from Linux, and this repo's CI has no Windows
runner.

- [ ] First-run connect against a real reliquary; the intercepted-200
      hint (Cloudflare Access page) still reaches the user verbatim
- [ ] Discovery, shelf, covers (and covers survive an hour of sync ticks
      without refetching), hidden games, scan trail
- [ ] Link with candidate, link by hand via native picker, create+seed
      world, split explainer correctness for a shared-folder game
- [ ] Full custody round trip: checkout(+launch), checkpoint (auto and
      manual), renew, checkin; takeover; claim-next and its
      notification; the two-halves checkout result
- [ ] Busy state blocks quit; error band, inline errors, action
      feedback, fatal path
- [ ] Tray: open/raise, sync now, status line truncation, quit
- [ ] Close-to-tray, autostart minimized, second-launch raise,
      window-state persistence
- [ ] Self-update from `reliquary-companion-latest` end-to-end,
      including restart and `.old` cleanup
- [ ] `127.0.0.1:8377` page still fully works in a browser alongside the
      window
- [ ] The window renders: fonts, theme, icon, and the layout at 1120×780
      and at a small window
- [ ] The redesign on a real machine: the three custody groups, the
      Games tab's search and filters, the overflow menu, first run and
      offline
- [ ] The two reported hover/render defects stay fixed on Windows — a
      shelf tile's cover, tooltip and cursor hold steady while the
      pointer moves across it, and a world row's 54×72 thumbnail keeps
      its aspect at every window width

## Also still open

- The reliquary image bundles only the browser build's exe for its
  token-gated download; the cutover decides whether it switches or
  carries both.
- Cutover itself: retire `cmd/companion`'s release, point players'
  download links at the new asset (the frozen-name rule says that is a
  migration, not an edit).
