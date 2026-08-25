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
- **Self-update**, in a different shape from the browser build's. That
  one replaces its own exe and restarts, which is right for a single file
  a player keeps wherever they like. This app is installed, so its
  release asset is an *installer* and the thing being replaced is the
  whole application — companiond is one file inside it, and on Windows a
  running executable cannot be overwritten at all. So the daemon
  downloads and verifies (`UpdateInstalls` mode in `companion/update.go`)
  and stops; the shell runs the installer and quits, because quitting the
  app being replaced is the one step a process inside it cannot take.
  macOS says it installs by hand: a dmg is mounted and dragged, not run,
  and these builds are unsigned.
- **Autostart** — an HKCU `…\CurrentVersion\Run` entry passing
  `--minimized`, so login brings it up in the tray. Windows only; other
  platforms answer with a reason rather than a broken checkbox.
- **Its own window chrome.** The window is frameless
  (`titleBarStyle: "hidden"`) and the page draws the titlebar —
  `web/companion`'s `TitleBar.tsx`. The caption buttons stay the
  platform's, recoloured through `titleBarOverlay` to sit in that strip:
  hand-drawn ones would cost Windows 11 its snap-layouts menu on hover,
  which is a worse loss than a mismatched button shape.
  `TITLEBAR_HEIGHT` in `main.ts` is only a *request*; the strip sizes
  itself from `env(titlebar-area-height)` and bounds its content with
  `env(titlebar-area-width)`, which are the Window Controls Overlay's own
  report of where the OS actually drew those buttons. A number agreed by
  hand across the two builds cannot survive a change of display scaling.
  The strip is then **one pixel taller than that reported height**: the
  overlay paints its own background across every pixel it claims, so a
  1px bottom rule counted inside that height sits under the caption
  buttons and disappears. The content box is the reported height and the
  rule goes below it — see `.app-titlebar` in `web/companion/src/index.css`.
  The **menu bar is gone** on Windows and Linux (`Menu.setApplicationMenu(null)`);
  every command it held is on the page or in the tray. macOS keeps a
  role-only menu, because its menu is not in the window and removing it
  takes Cmd+Q/C/V with it.
  The window's title is pinned to `APP_NAME` (`page-title-updated` is
  cancelled), so the taskbar and alt-tab say *Reliquary Companion* rather
  than the document title the browser build ships.
- **Scrollbars in the app's palette**, which is a Chromium trap worth
  knowing: it ignores *every* `::-webkit-scrollbar` rule on any element
  that also sets the standard `scrollbar-color`/`scrollbar-width`. The
  two are mutually exclusive, so declaring both threw all the styling
  away and left the platform's plain bar. The standard properties are
  behind `@supports not selector(::-webkit-scrollbar)`, which is exactly
  Firefox.
- **The titlebar names the machine**, from `os.Hostname()` on the
  engine's `State` (`companion/facade.go`, asked once — `Snapshot` runs
  on every poll and every change nudge). "This machine" was a truism on
  the screen in front of you; a name stops being one as soon as a second
  PC syncs under the same account, which is the case custody exists to
  disambiguate. An empty hostname is a real answer on a locked-down host,
  not an error, and falls back to the old wording.
- **Activity and Conflicts read the vault.** Both are one call —
  `GET /api/history` on the daemon, which reads
  `GET /api/public/sync/{token}/worlds/{id}` per linked world and merges
  the version lists. A conflict is not a separate record: it is a version
  the vault flagged because the check-in came from a session that had
  ended, or from one whose base was no longer the head
  (`core/savesync.Checkin`). So Conflicts is Activity filtered, and the
  two cannot disagree about what happened. **The world-detail route on
  the companion tier is new** (2026-08-23) — against an older reliquary
  both tabs report, correctly, that the worlds could not be read.
  Resolving a conflict moves a world's head, which is admin-only and
  deliberately *not* on the token tier: the view names where the ability
  lives rather than offering a button that would be refused.
- **Diagnostics is a dialog**, opened from the status bar
  (`DiagnosticsDialog.tsx`). Nothing in it is a setting, so it does not
  live in Settings: the link used to switch you to a tab you had not
  asked for and scroll you down it. One place, the same rule that took
  the settings cog off the header.
- **The chrome is two strips, not three.** The titlebar carries the app's
  name, which machine and account this is, and the whole sync report; the
  tab row carries the tabs and "Sync now"; the status bar carries what
  this machine holds and the way into Diagnostics. The separate header
  bar that used to sit under the titlebar is gone — it repeated the name
  and spent about 90px of window on one button.
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
- [ ] Self-update from `reliquary-companion-latest` end-to-end: the
      banner appears against a real release, the installer downloads and
      verifies, running it closes the app and replaces it, and the new
      build comes up with its config and links intact. **Implemented but
      never run end-to-end** — the logic is unit-tested against a
      stand-in GitHub, and no part of the real path (a published
      installer, an actual NSIS run over a live install) has been
      exercised. It is an update mechanism, so it is the last thing that
      should be taken on trust: check it before the download cuts over
- [ ] `127.0.0.1:8377` page still fully works in a browser alongside the
      window
- [ ] The window renders: fonts, theme, icon, and the layout at 1120×780
      and at a small window
- [ ] The redesign on a real machine: the three custody groups, the
      Games tab's search and filters, the overflow menu, first run and
      offline
- [ ] The two reported hover/render defects stay fixed on Windows — a
      shelf tile's cover, tooltip and cursor hold steady while the
      pointer moves across it, and a world row's thumbnail keeps its
      aspect at every window width — both frames are cut to IGDB's
      264×374 `t_cover_big` now, so a poster is shown whole rather than
      cropped to a letterbox
- [ ] Cover art survives a broken credential on the service: with IGDB
      refusing, the shelf says why rather than going quietly blank, and
      the covers come back **without restarting the companion** once the
      credential works again. This was a real defect — a failed lookup
      was cached as "IGDB has never heard of this game", on the service
      for hours and on the companion until it was restarted, so a shelf
      that lost its covers never got them back
- [ ] "Start the companion when I sign in" survives closing and
      reopening Settings, and the entry actually starts it minimized at
      login. Reading the setting back has to ask about the *same* login
      item that was written — path and args both — or Windows answers
      about a different one
- [ ] Activity and Conflicts against a reliquary new enough to have the
      world-detail route: entries appear, a world checked in from another
      machine shows up, a genuine conflict shows the badge and the tab
      count, and a world the vault refuses is named as unread rather than
      silently dropped
- [ ] The window's own chrome: drag by the titlebar, double-click to
      maximize, Windows 11 snap layouts still appear on hover over the
      caption buttons, and no menu bar anywhere. Check it at a display
      scaling other than 100%, which is the case `env(titlebar-area-*)`
      exists to survive
- [ ] The vault mark reads as a diamond everywhere it is worn — the
      titlebar, the empty state, and the icon at every size the OS asks
      for: taskbar, alt-tab, tray, installer, and the shortcut. The mark
      was a diamond on a wider kite until 2026-08-23; at 16-48px that
      pair reads as a small person (diamond a head, kite a body), which
      is invisible in the 1024px master. Check icons at the sizes an OS
      asks for, not at the size they are drawn. Regenerate with
      `npm run icon`
- [ ] The taskbar shows this app's own icon and groups its windows under
      one button, and a toast carries the app's name — all three come
      from the AppUserModelID (`APP_ID` in `main.ts`), not from
      `icon.png`, and without it an unpackaged run wears Electron's
      identity instead
- [ ] The native capabilities actually reach the page. They did not
      until 2026-08-23: `preload.ts` is a *sandboxed* preload, loaded as
      a single file whose `require` resolves only Electron built-ins, so
      its `require("./ipc-contract")` threw and killed the whole script.
      `window.companion` was undefined in every build, and nothing looked
      wrong — the shell injects the daemon's bearer at the network layer,
      so the page loaded and synced while the folder picker, "open the
      save folder" and the autostart toggle silently fell back to the
      browser build's "this build cannot answer". The channel names are
      duplicated into `preload.ts` now, with
      `test/ipc-contract.test.js` holding the copies to each other and
      failing on any relative `require`

## Also still open

- The reliquary image bundles only the browser build's exe for its
  token-gated download; the cutover decides whether it switches or
  carries both.
- Cutover itself: retire `cmd/companion`'s release, point players'
  download links at the new asset (the frozen-name rule says that is a
  migration, not an edit).
