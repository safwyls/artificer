# Reliquary Companion — the native desktop shell

Status: **in-development parallel build**. The browser-and-tray build
(`cmd/companion`, released as `artificer-companion.exe`) remains the
shipping companion until this one has passed the parity checklist below
on a real Windows machine; then it cuts over and the old build retires.

History: this shell began on 2026-08-22 as a Wails v2 window
(`companion-wails/`, PR #69) that rendered the React frontend in
WebView2. It was rebuilt the same week as a real desktop application —
Fyne v2 widgets drawn in-process with the engine — and the Wails module
was deleted in that change. That Fyne build was itself replaced
(2026-08-23, `companion-cutover.md` is the plan of record) by the current
shape: a headless Go daemon plus an Electron shell wrapping the existing
React renderer, rather than a second in-process Go UI. Reasons and
guardrails for that second cut are in `companion-cutover.md`; the API
contract it was built against is `docs/companion-api-surface.md`. What
both earlier cuts established is still true: the parallel-app structure,
the separate release identity, and the engine extraction that makes any
shell possible. Only the widget toolkit changed, again.

## What it is

The same save-sync client, as a proper desktop application. The engine —
discovery, custody sync, the `/api` surface, in-place updates — lives in
the importable `companion/` package, and two entrypoints wrap it:

- `cmd/companion` — the original shell: local server on `127.0.0.1:8377`,
  default browser as the window, Windows systray as the handle.
- `cmd/companiond` + `companion-desktop/` — this shell: `cmd/companiond`
  is a thin, headless daemon over `companion/` (same shape as a console
  binary over `core`) speaking HTTP plus an SSE push stream on loopback;
  `companion-desktop/` is an Electron main process that spawns it,
  health-gates a window against it, and owns the tray, native dialogs,
  autostart and OS notifications. The renderer is `web/companion`'s
  existing React app, served by the daemon and loaded into the window —
  not a second UI implementation. `companion-desktop/` is a **plain Node
  project**, not a Go module, and is not listed in `go.work`.

"Native" is worth stating precisely: this is a real desktop application —
its own window and event loop, the OS's own folder dialogs, an OS tray,
OS notifications, a `contextIsolation`-locked preload bridge — but the
window content itself is the same web renderer the browser build already
ships, run inside Electron's Chromium rather than the system browser. The
vault design language is unchanged from the browser build:
`design-system/lib/vault.mjs`'s tokens flow into `web/companion` exactly
as they do for `cmd/companion`, so the two shells render identically.

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
- **Window size persistence**, via Electron's own `BrowserWindow` bounds
  handling in the main process — never in the shared config file, which
  the browser build reads.

## The window, reorganised around custody (2026-08-22)

The first native cut (Fyne) carried the browser build's information
architecture across verbatim: "Your worlds" as one small card above an
installed-games grid that took most of the screen. A maintainer-approved
redesign reorganises the window around the question the app exists to
answer — *can I take this world right now?* Five structural changes,
implemented against the Fyne UI at the time and carried forward into
`web/companion`'s React screens by the cutover (`companion-cutover.md`
Phase 4):

1. **Worlds is the whole page.** The installed-games grid became its own
   **Games** tab, with search, an All/Linked/Unlinked filter, linked and
   unlinked sections, two-line tile names and hidden entries as a line of
   prose rather than a fake tile in the grid.
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
(`web/companion/src/lib`'s `custodyOf`, the one rule both the old Fyne
`model.go` and the current TS lib implement), so a row cannot say "Free"
beside a "Check in" button. Covered by that lib's own tests.

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
replace each other: window title "Reliquary Companion",
`appId: com.artificer.reliquarycompanion` (`companion-desktop/electron-builder.yml`),
rolling release tag `reliquary-companion-latest` (workflow
`release-reliquary-companion.yml`). The browser build keeps the frozen
`artificer-companion.exe` name on `companion-latest` (workflow
`release-companion.yml`); the two release tags, workflows and asset
names have never collided across the Wails→Fyne→Electron history because
each rebuild kept them pinned rather than renaming anything.

## Building

```
cd web/companion && npm run build    # the daemon embeds dist/
cd companion-desktop
npm ci
npm run build:daemon                 # stage resources/companiond[.exe] for the host platform
npm run build                        # tsc, main/preload/renderer glue
npx electron-builder --linux AppImage    # or --win nsis / --mac dmg
```

`companion-desktop/README.md` has the full packaging contract: the
`resources/companiond[.exe]` staging dir `extraResources` carries into
`process.resourcesPath`, and the `app.isPackaged` dev-vs-packaged path
branch in `src/daemon.ts`. `release-reliquary-companion.yml` cross-builds
the daemon per target OS/arch in CI (companion-cutover.md Phase 5 item
3) rather than relying on a dev machine to produce all three platforms.
The window/tray/app icon (`companion-desktop/build/icon.png`, 256×256) is
generated from `web/companion/public/favicon.ico` — the same artwork the
browser build's exe icon and the page favicon use.

## Parity checklist (the cutover gate)

Everything here verified on a real Windows machine before
`cmd/companion` retires. **None of it is ticked**: the app has never been
run outside CI. `release-reliquary-companion.yml` does build and package
it on real Windows and macOS runners, which is enough to catch a broken
build, but a CI job clicking nothing is not the same as a person
confirming behavior — the checklist below stays open until someone does.

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
- [ ] Self-update from `reliquary-companion-latest` end-to-end, including
      restart and `.old` cleanup — **not yet implemented**: `companion`'s
      update watcher is wired into `cmd/companion` only; the Electron
      shell has no updater code yet, so this item blocks on that work
      landing before it can be checked
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
