# Reliquary Companion — native desktop rebuild plan

Written 2026-08-22 as a handoff: an agent executing this plan should be
able to build the app from this document plus the pointers in it. The
design mock lives at
<https://claude.ai/code/artifact/c531db04-fbac-4d50-8dea-872f0c013891>
(main window, first run, link dialog, settings, tray + notifications,
with annotations on the native-specific decisions).

## What this is

Rebuild the companion UI as a **proper desktop application with
natively rendered elements** — real widgets drawn by a desktop toolkit,
in-process with the Go engine — rather than a web frontend inside a
webview. This supersedes the Wails shell (`companion-wails/`, PR #69):
that shell proved out the parallel-app structure, the separate release
identity and the engine extraction, but it still renders the React UI
in WebView2. The native app keeps everything PR #69 established except
the webview itself.

The product identity carries over unchanged: **reliquary-companion** —
window title "Reliquary Companion", binary `reliquary-companion.exe`,
rolling release tag `reliquary-companion-latest`, and the update-knob
pins (`companion.UpdateTag`, `companion.UpdateAssets`) that keep it and
the browser build from ever replacing each other.

## Ground rules (read before writing code)

- **The engine is done; do not fork it.** All custody/discovery/update
  logic lives in the importable `companion/` package (extracted
  2026-08-22). The UI calls it in-process. Any logic the UI needs that
  the package does not expose is a small, exported addition to that
  package — never a copy, and never new logic in the UI layer.
- **Frozen API stays frozen** (`docs/companion-ui-rebuild.md`): the
  local page on `127.0.0.1:8377`, the config file location, and every
  `/api` route keep working. The native app runs `http.Serve(ln,
  app.Routes())` exactly as both existing shells do — the browser page
  remains the escape hatch and the old-build handoff target.
- **Single instance across all builds**: refuse to start when
  `companion.AlreadyRunning("127.0.0.1:8377")` — two sync loops over
  one config file fight over custody. Same rule the other shells
  follow.
- **Game-blind**: the companion never branches on which game a world
  belongs to (`scripts/checkbounds.sh` enforces it for the web
  frontends; hold the native UI to the same rule by review). A game's
  name, cover and save quirks arrive as data from the engine.
- The web frontend (`web/companion/`) is **not modified** by this work;
  it keeps shipping in both existing builds until the cutover.

## Decision record

### Toolkit: Fyne v2, in-process

| Option | Verdict |
|---|---|
| **Fyne v2** (Go, custom-rendered widgets, OpenGL) | **Chosen.** In-process with the `companion` package — no HTTP, no serialization, direct method calls and callbacks. Mature widget set (lists, grids, dialogs, forms), themeable enough to carry the vault palette, image widgets for cover art, cross-platform, and the tray story is literally the same library the browser build already uses (`fyne.io/systray`). |
| Gio (Go, immediate-mode) | Full design fidelity, but everything is hand-drawn: lists, focus, text fields, dialogs all bespoke. Too much UI infrastructure for a ~15-view app. Fallback if Fyne's theming proves too coarse. |
| walk / Win32 native widgets | Literally-native controls, but Windows-only, semi-maintained, and stock Win32 chrome discards the vault design language entirely. |
| C#/WinUI3 or Avalonia over the HTTP API | Real native (WinUI) or polished custom (Avalonia), but a second language in the repo, out-of-process against localhost, and duplicated model types. Not worth it for this app's size. |

The honest tradeoff to state up front: Fyne renders its own widgets —
"native" here means *a real desktop application* (native window, native
event loop, OS dialogs, tray, notifications, no browser engine), not
Win32 stock controls. Fyne's theming cannot reproduce the web UI
pixel-for-pixel (no gradients on buttons, one radius per widget class);
the mock shows the target, and "as close as the theme API allows" is
the bar. Where Fyne genuinely cannot express something load-bearing
(the cover-art tile treatment, the custody chip), a small custom widget
(`fyne.io/fyne/v2/widget.BaseWidget` + canvas primitives) is the
answer, not a webview.

### Module layout: replace `companion-wails/` with `companion-desktop/`

Fyne needs CGO on every platform (OpenGL), so like the Wails shell it
cannot live in the root module without dragging CGO into `go build
./...`. New module `companion-desktop/` (same `require` + `replace ../`
shape as `companion-wails/`, added to `go.work`). Once it reaches
parity, delete `companion-wails/` — its job was the interim webview
window, and this plan replaces it. Do the delete in the same PR that
lands the desktop app in CI, so there is never a moment with three
shells.

### Process model: close-to-tray

The window is a view over a resident sync process. Closing the window
hides it and keeps syncing (tray icon stays); Quit lives in the tray
menu and in a confirm path when a transfer is running ("Transferring a
save — don't quit yet" is already the tray's most important line).
This replaces the browser build's model where the process is tray-only
and the UI is a tab.

## Phase 1 — engine facade: events instead of polling

The web UI polls `GET /api/state` every 5 s; a native in-process UI
subscribes. Add to the `companion/` package (this is the one root-module
change in the plan):

1. **Typed state snapshot.** The `/api/state` handler already builds a
   JSON view (`companion/server.go`). Extract its assembly into an
   exported method — `func (a *App) Snapshot() State` — returning an
   exported struct (`State`: config presence, sync worlds, discovery,
   update state, versions, last action). The HTTP handler renders the
   same struct; one source of truth.
2. **Change notification.** `func (a *App) Subscribe() (<-chan
   struct{}, func())` — a coalescing signal channel (buffered, size 1)
   nudged whenever any mutation lands (sync tick results, rescan,
   config save, update state, checkpoint times). Fire it from the
   handful of places that already take `a.mu`; the UI reacts by calling
   `Snapshot()`. No payloads on the channel — snapshot-on-nudge keeps
   ordering trivial.
3. **Presence heartbeat.** The web page's GET is what sets `pageSeen`
   (the custody poll speeds up while someone is looking). Export `func
   (a *App) MarkSeen()` doing what the state handler does today; the
   desktop app calls it on a ticker while the window is visible/focused
   and stops when hidden to tray. Without this the poll never enters
   its fast mode and the window shows stale custody — this is the
   subtlest parity requirement in the plan.
4. **Typed actions.** The action routes are thin wrappers already;
   export the ones the UI needs as methods returning Go errors:
   checkout (with the takeover/play flags and the two-halves result —
   checked-out-but-launch-failed must stay distinguishable), checkin,
   checkpoint, renew, claim, link/create/unlink/edit, hide, set-config,
   browse-less savepath split/resolve (still needed to explain what a
   link will do), artwork fetch, savehints fetch, update check/apply.
   Where a handler holds real logic today, invert it: handler calls the
   exported method.
5. **Artwork bytes.** Covers are currently URLs the browser fetches.
   The engine already caches lookups (`app.art`); add a fetch-and-cache
   of the image bytes (bounded, misses cached) so the UI gets
   `[]byte`/`fyne.Resource` and tiles never flicker or refetch — the
   web UI's hard-won rule "covers must not remount on poll" becomes
   "cache `fyne.Resource` per game key, forever".

The full engine test suite must stay green untouched apart from
additions; the facade gets its own tests (snapshot completeness,
subscribe coalescing, MarkSeen pacing).

## Phase 2 — app skeleton

- `companion-desktop/main.go`: mirror `companion-wails/main.go`'s
  startup exactly (version stamp, update pins, LoadConfig,
  SetupLogging, ClearOldBinary, single-instance refusal, engine loops,
  8377 server), then `app.New()` → themed window 1120×780.
- **Theme**: implement `fyne.Theme` mapping the vault tokens
  (`design-system/lib/vault.mjs` is the source of truth):
  ink `#100d17` (background), well `#14101d` (input/header fills),
  panel `#1a1524` (card/menu/dialog), edge `#2f2740` (borders,
  separators), parchment `#e8e0cf` (foreground), mist `#948da3`
  (placeholder/disabled/secondary), gold `#c9a860` (primary, focus,
  selection), goldhi `#e3c67f` (hover), ok `#7fc46a`, ember `#d4735e`
  (error), rune `#9d7fc4`. Radii: 8 px panels / 4 px inputs — Fyne has
  one corner radius; set 4 and draw panel cards as custom rounded
  rects. Fonts: bundle **Gelasio** (OFL, Georgia-metric serif — the web
  UI's own stack names it) as the regular face and a mono face (e.g.
  JetBrains Mono or Go Mono) for paths/versions/badges; embed via
  `fyne bundle`.
- **Tray**: reuse the browser build's tray verbatim (same
  `fyne.io/systray` menu: open window, sync now, status line, quit),
  with "Open companion page" becoming "Open Reliquary Companion"
  (show/raise the window). Same favicon-derived icon.
- **Window lifecycle**: `SetCloseIntercept` → hide to tray; restore
  from tray or from a second-launch handoff. Persist window
  size/position beside the config file (`companion-desktop.json`, not
  in the shared companion config — the browser build must not learn new
  keys).
- **Restart-for-update**: set `companion.ExitForRestart` to tear the
  tray down (as `cmd/companion`'s Windows build does) — no page reload
  dance; `restartSelf` already launches the replacement.

## Phase 3 — screens

Build in this order; each maps a web component (file names from
`web/companion/src/components/`) to its native counterpart. The mock
artboards show all of these.

1. **Shell**: header (status dot, "Connected as _user_", mono
   freshness line, Sync now, settings gear — `HeaderBar.tsx`), status
   bar (`companion <ver> · service <ver>` + last action), content
   scroll. Freshness re-renders on a 1 s ticker independent of engine
   events, exactly like the web header does.
2. **First run** (`FirstRun.tsx`): centered connect card (URL + token)
   over the login radial treatment, Steam-folder override with a
   native folder picker. Shown when `!state.sync.configured`.
3. **Worlds list** (`WorldRow.tsx`, `CustodyChip.tsx`): custom row
   widget — cover thumb, name, rune game tag, custody chip (six
   states: free/mine/fetching/held/expired/gone, colors per mock),
   custody sentence, mono path, per-state action buttons. The verb
   matrix in `WorldRow.tsx:19-169` is the spec; port it exactly,
   including takeover-behind-confirm and claim-next.
4. **Shelf** (`Shelf.tsx`, `GameTile.tsx`, `CoverArt.tsx`):
   `container.NewGridWrap` of custom tiles — 3:4 cover, gold border
   when linked, dimmed/desaturated when not (Fyne: render the cached
   cover through a translucent dark overlay; no CSS grayscale), hidden-
   count tile, empty/scan-error/save-hints caption lines, Rescan and
   Link-by-hand actions. Scan trail becomes a collapsible
   (`widget.NewAccordion`) that auto-expands when a fresh scan found
   nothing.
5. **Dialogs**: link game (candidate dropdown, folder entry + native
   picker, world dropdown incl. create-new, split explainer text,
   seed-checkbox — `LinkGameDialog.tsx` is the spec; **the in-app
   `FolderBrowser` and `/api/browse` UI are dropped in favor of the OS
   folder picker**, the one place the native app deliberately deletes
   web UI), linked-game detail (launch target), edit world, settings
   (service, Steam folder, launch-on-checkout, autostart, notification
   toggle, version block + check/apply update), confirm dialog for
   unlink/takeover.
6. **Error surfaces**, all four kinds preserved: fatal startup error
   (dialog + exit), header ember band for `sync.lastError`, inline
   ember/rune callouts in forms, and per-action feedback — web toasts
   become a transient in-window status strip anchored above the status
   bar, plus OS notifications for the events worth hearing about with
   the window closed (next phase).

## Phase 4 — desktop-proper features

The reason this rebuild exists; the web UI cannot do these.

- **Native folder picker** everywhere a path is chosen (Fyne's
  `dialog.NewFolderOpen` is custom-drawn; prefer the OS dialog via
  `sqweek/dialog` or a small win32 `IFileOpenDialog` binding on
  Windows, falling back to Fyne's on other platforms).
- **OS notifications** (`fyne.App.SendNotification`), off a
  notification toggle in settings, for exactly three events: a queued
  claim came through (world checked out to this machine), a hold
  nearing expiry, and a sync error while the window is hidden. No
  notification spam — everything else stays in-window.
- **Close-to-tray + autostart**: "Start with Windows, minimized to the
  tray" checkbox (HKCU `...\CurrentVersion\Run` entry writing
  `reliquary-companion.exe --minimized`; flag starts hidden).
- **Busy protection**: while `sync.busy`, the quit paths (tray Quit,
  window close if quit-on-close ever configured) confirm first — a
  killed transfer is the one unforgivable failure.
- **Second-launch handoff**: launching the exe while an instance runs
  raises the existing window. Cheapest correct mechanism given the
  frozen surface: a tiny `POST /api/raise` (loopback-only like
  everything else) the new process calls before bowing out; the old
  builds 404 it harmlessly. Additive route = allowed; removing routes
  is not.

## Phase 5 — build, release, CI

- **CGO cross-build**: Fyne Windows builds need mingw-w64. CI
  (`ci.yml`, monorepo job) gains `sudo apt-get install -y
  gcc-mingw-w64-x86-64` and builds `companion-desktop` with
  `GOOS=windows CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc -ldflags="-H
  windowsgui"`. Native Linux dev builds need `libgl1-mesa-dev
  xorg-dev` (document, don't build in CI).
- **Release**: repoint `release-reliquary-companion.yml` at
  `companion-desktop/` with the mingw toolchain; tag, asset name, and
  manifest files (`companion-version.txt`, `companion-sha256.txt`)
  stay exactly as they are — installed dev builds keep updating.
  Confirm `verifyExecutable`'s MZ check still passes (it will; PE is
  PE).
- **Icon**: generate an `.syso` from `web/companion/public/favicon.ico`
  like `cmd/companion`'s (`go generate` + the rsrc freshness check in
  CI), plus `window.SetIcon` for the runtime window icon.
- **Retire `companion-wails/`** in the PR that lands CI for
  `companion-desktop/`: delete the module, drop its CI step and
  `go.work` entry, update `docs/reliquary-companion.md` to describe the
  native shell instead.

## Parity checklist (the cutover gate)

Everything here verified on a real Windows machine before
`cmd/companion` retires; this extends the gate already recorded in
`docs/reliquary-companion.md`:

- [ ] First-run connect against a real reliquary; the intercepted-200
      hint (Cloudflare Access page) still reaches the user verbatim
- [ ] Discovery, shelf, covers (and covers survive an hour of
      sync ticks without refetching), hidden games, scan trail
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
- [ ] `127.0.0.1:8377` page still fully works in a browser alongside
      the window
- [ ] Engine suite + facade tests green; `checkbounds`, `checkdocs`
      green

## Non-goals

- No redesign: the vault language and the existing screen inventory
  are the spec; the mock is the web UI translated to a desktop shell,
  not a new UI. New affordances are limited to Phase 4's list.
- No macOS target (no players there; the engine's asset map has no
  darwin entry). Linux stays a dev-only build.
- No touch to `web/companion`, `cmd/companion`, or the reliquary
  service.

## Pointers

- Mock: <https://claude.ai/code/artifact/c531db04-fbac-4d50-8dea-872f0c013891>
- Engine: `companion/` (esp. `server.go` for the full route/action
  inventory, `app.go` for state shape, `update.go` for the update
  identity knobs, `run.go` for the shell contract)
- Existing shells: `cmd/companion` (tray patterns to reuse),
  `companion-wails/` (startup sequence to mirror, then retire)
- Web UI as behavioral spec: `web/companion/src/components/` — the
  survey-worthy details are called out inline above by file name
- Design tokens: `design-system/lib/vault.mjs`,
  `design-system/companion/system.mjs` (+ committed previews)
- Custody semantics: `docs/save-sync-architecture.md`; frozen surface:
  `docs/companion-ui-rebuild.md`; current shell state:
  `docs/reliquary-companion.md`
