# Companion API surface (Phase 0 inventory)

Written for the Fyne → Electron cutover
(`companion-cutover.md`). This is the contract Phase 1 (daemon
extraction) and Phase 4 (renderer) build against: every operation the
Fyne desktop UI (`companion-desktop/`) can trigger, every piece of state
it displays, and whether that operation already exists on
`companion/server.go`'s HTTP surface or is new.

## 1. File inventory

`companion-desktop/` is its own Go module (`go.work` lists it alongside
`.` and `./anvil`), built over the importable `companion/` package. Every
domain rule it needs (custody, sync, discovery, config, launch, update)
already lives in `companion/` and is reached through `companion.App`'s
facade methods (`facade.go`) or its HTTP routes (`server.go`) — the
desktop shell was already thin over the same engine the browser build
uses. Nothing found here needs to be *ported* to `companion/`; the
question for each file is only whether it becomes Electron-main
responsibility, is discarded with the Fyne tree, or needs a genuinely
new engine capability.

| File | Classification | Notes |
|---|---|---|
| `main.go` | Fyne UI / glue | App bootstrap, window creation, `--minimized` flag, single-instance listen on `127.0.0.1:8377`. The listen-and-hand-off pattern (`handOverToRunningInstance`) is Electron-main's job (single-instance lock in Phase 3). |
| `ui.go` | Fyne UI | Window chrome, redraw loop, cover-image caching for in-process widgets, window size persistence, show/hide/quit. All Fyne-specific; discarded. `watchEngine`'s poll-facade-and-redraw pattern maps to the renderer's `Subscribe`-via-WebSocket equivalent (see §3). |
| `screens.go` | Fyne UI | Tab bar, header, footer, first-run screen, connect screen layout. Discarded; `web/companion` already has (or needs) equivalent React screens per the redesign handoff. |
| `worlds.go` | Fyne UI | World rows, group panels, row actions, checkin/checkout wrappers. Every action calls an existing `companion.App` method (`Launch`, `Checkin`, `Checkout`, `Claim`, `Unlink`, `OpenURI`) — no new domain logic. Discarded. |
| `games.go` | Fyne UI | Games tab: grid, toolbar/filter/search, tile rendering, show-game dialog trigger, scan trail. Discarded; filter/search logic is presentation-only and belongs in the renderer. |
| `dialogs.go` | Fyne UI | Every modal: link-game, linked-game detail, edit-world, rename-world, diagnostics, settings. All read/write through existing facade calls (`SetConfig`, `Link`, `CreateWorld`, `EditLink`, `CheckUpdate`) except the autostart and notify-toggle checkboxes inside `showSettings` — see glue rows below. Discarded as UI; the settings *operations* it wires up are enumerated in §2. |
| `button.go`, `taparea.go`, `theme.go`, `kit.go` | Fyne UI | Custom widget kit and vault theme for Fyne. Purely presentational, no ported logic; discarded outright (the design system for the vault palette already lives in `design-system/lib/vault.mjs` for the web renderer). |
| `fonts/` | Fyne UI (asset) | Embedded font files for the Fyne theme. Discarded; the web renderer uses its own font loading. |
| `model.go` | **Duplicate domain logic — flag** | A hand-written Go port of `web/companion/src/lib` (`types.ts`, `format.ts`): `custodyOf`, `custodyLine`, `fmtDuration`, `fmtBytes`, `fmtWhen`, `headMeta`, `worldsOf`, `launchTargetOf`, `gameKey`. The file's own header says this rule for rule and explicitly disclaims deciding anything new — but it is a second implementation of custody labelling that must be kept in sync with the TS original by hand. **Not new logic**, and not something to carry forward: with the renderer being `web/companion`'s existing TS lib, this file is pure duplication and is deleted with the rest of the Fyne tree, not ported. Called out because at a glance it looks like it might contain domain logic that needs rescuing — it does not; it is presentation-layer formatting that already exists correctly on the TS side. |
| `model_test.go` | Fyne UI (test) | Tests `model.go`'s ported rules. Discarded with it. |
| `notify.go` | **Glue → Electron shell, with logic to preserve** | OS notification dispatch (`notify`) is textbook Electron-main OS integration. But the three *policies* — `claimArrived` (diff two snapshots for a claim that landed unattended), `holdNearlyUp` (debounced warning inside a 15-minute window, re-fires hourly via `holdWarned` map), `syncFailedUnseen` (only fires when window is hidden, edge-triggered on `lastError` change) — are decision logic that does not exist in `companion/` today. This is the one place actual "when should we act" logic is trapped outside the engine. See §2/§4 for the new operation this implies. |
| `tray.go` | Electron shell | System tray menu, status line refresh loop, `ExitForRestart` wiring for self-update. All native tray responsibility, moves to Electron main (Phase 3). `StatusLine()` itself is already an engine method (`app.go`) and needs no change — the daemon keeps it, Electron main polls it or the renderer surfaces the same summary. |
| `raise.go` | Electron shell (as a concept) / glue | Single-instance "raise the window" handshake via an additive `POST /api/raise` route on the same loopback listener. The route pattern is right and should stay on the daemon (`/api/raise` is genuinely useful for a daemon that outlives its shell), but *who calls it and what "raise" means* (focus an Electron `BrowserWindow`) is Electron-main's job. Classify the route as glue to keep on the Go side; classify the raising behavior as Electron shell. |
| `picker.go` | Electron shell | Native folder/file pickers via `github.com/sqweek/dialog`. Directly replaced by Electron's `dialog.showOpenDialog` in the main process, exposed to the renderer over IPC/`contextBridge`. The existing `GET /api/browse` route (in `companion/browse.go`, used by the browser build) stays as the fallback for anyone running the plain browser page. |
| `autostart_windows.go`, `autostart_other.go` | Electron shell | HKCU Run-key autostart toggle (Windows-only; other platforms return `errAutostartUnsupported`). This is exactly the kind of OS integration `electron-builder`/Electron's own autostart APIs (or a small IPC-exposed native module) replace. No engine logic to preserve — it is pure registry/OS plumbing. |
| `resource_windows.go`, `rsrc_windows_amd64.syso` | Fyne UI (build asset) | Windows resource embedding (icon) for the Fyne binary. Discarded; Electron/electron-builder has its own icon/resource pipeline. |
| `go.mod`, `go.sum` | Fyne UI (module) | The companion-desktop module's dependencies (Fyne, sqweek/dialog, systray, golang.org/x/sys/windows). Discarded/replaced by whatever `cmd/companiond`'s go.mod needs (Phase 1) — none of Fyne's dependencies carry forward. |

**Counts:** 24 files total (excluding `fonts/` contents and the `.syso`
binary as sub-items). Fyne UI (discard): 15. Glue → Electron shell: 6
(`tray.go`, `raise.go`'s raising half, `picker.go`, both autostart
files, `notify.go`'s dispatch half). Domain logic already covered by
`companion/` (no port needed): all of it — no file in
`companion-desktop/` implements custody, sync, discovery, or save-path
logic that `companion/` doesn't already own. One file (`notify.go`)
contains genuine *new* decision logic (debounced notification
triggers) not yet represented in the engine.

## 2. What `companion/server.go` already exposes

Routes as of this inventory (`companion/server.go`, all under
`Routes()`):

| Route | Verb | Facade method | Notes |
|---|---|---|---|
| `GET /` | — | serves embedded `web/companion` `dist/` | |
| `GET /api/state` | request/response | `Snapshot()` (via `MarkSeen` + poll) | The one assembly both shells render from |
| `PUT /api/config` | request/response | `SetConfig` | Server URL, token, Steam dirs, launch-on-checkout |
| `POST /api/discover` | request/response | `Rescan` | |
| `GET /api/artwork` | request/response | `Artwork`, `ArtStatus` | |
| `POST /api/sync/refresh` | request/response | `SyncNow` | |
| `GET /api/savehints` | request/response | `SaveHints` | |
| `GET /api/browse` | request/response | (browse.go) | Folder browser for the web page (native picker replaces it in Electron) |
| `GET /api/savepath/split` | request/response | `SplitSavePath` | |
| `POST /api/savepath/resolve` | request/response | `ResolveSavePath` | |
| `POST /api/hide` | request/response | `Hide` | |
| `POST /api/links` | request/response | `Link` | |
| `POST /api/links/create` | request/response | `CreateWorld` | |
| `PUT /api/links/{worldID}` | request/response | `EditLink` | |
| `POST /api/links/{worldID}/launch` | request/response | `Launch` | |
| `DELETE /api/links/{worldID}` | request/response | `Unlink` | |
| `POST /api/links/{worldID}/checkout` | request/response | `Checkout` | |
| `POST /api/links/{worldID}/checkin` | request/response | `Checkin` | |
| `POST /api/links/{worldID}/checkpoint` | request/response | `Checkpoint` | |
| `POST /api/links/{worldID}/renew` | request/response | `Renew` | |
| `POST /api/links/{worldID}/claim` | request/response | `Claim` | |
| `POST /api/update/check` | request/response | `CheckUpdate` | |
| `POST /api/update/apply` | request/response | `ApplyUpdate` + `RestartAfterUpdate` | Response is written before the process restarts |
| `GET /healthz` | request/response | — | **Added in Phase 1.** Liveness only (`{ok, version}`), and the one route outside the bearer check: a shell polls it before it has proven anything |
| `GET /api/events` | **stream (SSE)** | `Subscribe` + `MarkSeen` | **Added in Phase 1.** The HTTP face of the in-process nudge channel — see below |
| `POST /api/raise` | request/response | `ServerOptions.Raise` | **Added in Phase 1**, promoted from `companion-desktop/raise.go`. 501 with a reason where the shell owns no window (both current builds) |

`companion-desktop`'s only addition on top of this (`raise.go`) was
`POST /api/raise`; Phase 1 promoted it into `companion/server.go`.

**Phase 1 status (2026-08-23).** The three routes above are implemented,
plus `ServerOptions{Token, Raise}` and `RoutesWithOptions`: with a token
set, every request but `GET /healthz` needs
`Authorization: Bearer <token>` (constant-time compare). `Routes()` — the
browser build — passes no options and keeps exactly the surface it had.
Covered by `companion/server_test.go`.

The push endpoint is **server-sent events, not a WebSocket** — a
deliberate deviation from `companion-cutover.md`. It matches what core
already does for custody (`core/api/savesync_live.go`, same 25s
keepalive), and the traffic is one-directional with no payload: the
stream carries `event: ready` once and `event: changed` per nudge, and
the client re-reads `GET /api/state`, which makes a dropped or coalesced
event harmless. A live stream also counts as someone watching
(`MarkSeen`), so a renderer that stopped polling does not silently get
minute-old custody.

None of these were WebSocket. `GET /api/state` is polled (the page
does this every few seconds; `MarkSeen` speeds the background poll up
while someone is watching). The Fyne shell instead used the in-process
`Subscribe()`/`changed()` nudge channel from `facade.go` to redraw
immediately on any mutation — that channel has no HTTP/WebSocket
equivalent yet, which is the main gap Phase 1 needs to close for a
comparably responsive Electron renderer.

## 3. Full operation table (Fyne UI trigger → state → surface)

Each row: what the UI does, its inputs/outputs, request/response vs
stream, whether it exists on the HTTP server already, and which layer
owns it going forward.

| # | Operation | Trigger (companion-desktop) | Inputs | Outputs | R/R or Stream | On server today? | Owning layer |
|---|---|---|---|---|---|---|---|
| 1 | Load full state | `ui.snapshot()`, page poll equivalent | — | `companion.State` (config, links, discovered, sync, version, update) | R/R (Fyne: in-process `Snapshot()`; web: poll) | Yes — `GET /api/state` | Go daemon |
| 2 | Live state change nudge | `ui.watchEngine()` via `Subscribe()` | — | empty nudge → re-fetch snapshot | **Stream** | **Yes, since Phase 1** — `GET /api/events` (SSE, not WS) | Go daemon |
| 3 | Save connection settings | Settings dialog "Save & connect" | serverUrl, token | ok/error | R/R | Yes — `PUT /api/config` | Go daemon |
| 4 | Save Steam folder & rescan | Settings "Save folder & rescan" | steamDirs | ok/error, triggers rescan | R/R | Yes — `PUT /api/config` (SteamDirs triggers `Rescan`) | Go daemon |
| 5 | Toggle launch-on-checkout | Settings checkbox | bool | ok/error | R/R | Yes — `PUT /api/config` | Go daemon |
| 6 | Rescan installed games | Games tab "Rescan", first-run step | — | found count | R/R | Yes — `POST /api/discover` | Go daemon |
| 7 | Resolve cover art | Shelf/tile render | — | map of art + asked/error | R/R (Fyne: `Cover()` bytes cache; web: URL) | Yes — `GET /api/artwork`; `Cover()` bytes-fetch is Fyne-only convenience, not needed in Electron (renderer can `<img src>` the URL like the web build) | Go daemon |
| 8 | Sync now (poll vault) | "Sync now" tray item, offline banner "Retry now" | — | worlds, ok/error | R/R | Yes — `POST /api/sync/refresh` | Go daemon |
| 9 | Save-location hints | Games tab load | — | available, known count, error | R/R | Yes — `GET /api/savehints` | Go daemon |
| 10 | Browse for a folder (web fallback) | n/a in Fyne (native picker used instead) | dir | tree of candidates | R/R | Yes — `GET /api/browse` | Go daemon (kept for plain-browser fallback; Electron renderer uses IPC picker instead, op #21) |
| 11 | Split a save path | Link-game dialog, before confirming | dir, appId, name | `SavePathSplit` | R/R | Yes — `GET /api/savepath/split` | Go daemon |
| 12 | Resolve/join a save path | Link-game dialog (join flow) | root, leaf, create | dir, exists | R/R | Yes — `POST /api/savepath/resolve` | Go daemon |
| 13 | Hide/unhide a shelf entry | Games tab tile menu, "show hidden" link | key, hidden bool | ok/error | R/R | Yes — `POST /api/hide` | Go daemon |
| 14 | Link an existing world to a folder | Link-game dialog (join) | worldId, gameTitle, dir, meta, appId | ok/error | R/R | Yes — `POST /api/links` | Go daemon |
| 15 | Create a new world and link it | Link-game dialog (create), first-run step 3 | name, gameTitle, dir, meta, appId, savePath, seed | ok/error | R/R | Yes — `POST /api/links/create` | Go daemon |
| 16 | Edit a link (launch target / folder / name) | Edit-world dialog | worldId, launchTarget?, dir?, worldName? | ok/error | R/R | Yes — `PUT /api/links/{worldID}` | Go daemon |
| 17 | Rename a world | Rename-world dialog | worldId, name | ok/error | R/R | Yes — `PUT /api/links/{worldID}` (WorldName field) | Go daemon |
| 18 | Launch a held world's game | World row "Play" | worldId | ok/error | R/R | Yes — `POST /api/links/{worldID}/launch` | Go daemon |
| 19 | Unlink a world | Overflow menu "Unlink" (behind confirm) | worldId | ok/error | R/R | Yes — `DELETE /api/links/{worldID}` | Go daemon |
| 20 | Check out (+ optional play) | World row primary action | worldId, takeover, play | launched, launchError? | R/R | Yes — `POST /api/links/{worldID}/checkout` | Go daemon |
| 21 | Check in | World row "Check in" | worldId | ok/error | R/R | Yes — `POST /api/links/{worldID}/checkin` | Go daemon |
| 22 | Checkpoint (mid-session push) | Not directly user-triggered in Fyne UI; run by tray "Sync now" loop and auto-checkpoint | worldId | ok/error | R/R | Yes — `POST /api/links/{worldID}/checkpoint` | Go daemon |
| 23 | Renew a hold | Not directly wired to a visible Fyne button today (engine capability; "renew" appears in custody line copy) | worldId | ok/error | R/R | Yes — `POST /api/links/{worldID}/renew` | Go daemon |
| 24 | Claim / queue for a held world | "Ask to be next" | worldId | ok/error | R/R | Yes — `POST /api/links/{worldID}/claim` | Go daemon |
| 25 | Check for update | Settings "Check for update" | — | `UpdateState` | R/R | Yes — `POST /api/update/check` | Go daemon |
| 26 | Apply update & restart | (Reachable via update flow; not a distinct Fyne button beyond check-for-update surfacing "update available") | — | ok, restarting | R/R | Yes — `POST /api/update/apply` | Go daemon |
| 27 | Open a URI / save folder | Overflow "Open save folder", first-run links, offline handoff | uri | ok/error | R/R | **No dedicated route** — `companion.OpenURI` is called in-process only | **New**: needs an IPC call in Electron main (shell.openPath), not a Go route — this is filesystem/OS integration, correctly Electron-shell, not a daemon concern |
| 28 | Copy save path to clipboard | Overflow "Copy save path" | text | — | local only | N/A | Electron shell (renderer `navigator.clipboard` or IPC) |
| 29 | Raise/focus the window | Second launch, tray "Open" | — | ok/raised | R/R | **Yes, since Phase 1** — `POST /api/raise` in `companion/server.go`; 501 with a reason when the shell passes no `Raise` | Go daemon keeps the route (single-instance handshake belongs on the long-lived process); Electron main's `BrowserWindow.focus()` is the actual raise action |
| 30 | Show/hide to tray, quit | Window close intercept, tray "Quit" | — | — | local only | N/A | Electron shell |
| 31 | Native folder/file picker | "…" buttons beside path entries | start dir | chosen path | R/R (native dialog) | **No** — `GET /api/browse` is the web equivalent; Fyne bypassed it with `sqweek/dialog` | **New IPC op**: Electron main `dialog.showOpenDialog`, exposed via `contextBridge` |
| 32 | System tray menu + status line | `tray.go` | — | `StatusLine()` text | Stream-ish (5s refresh loop) | `StatusLine()` exists as a facade method but **no HTTP route serves it** — Electron main can derive the same summary from `GET /api/state`/the WS stream instead of needing a dedicated route | Electron shell (rendering); Go daemon (data via #1/#2) |
| 33 | Autostart toggle | Settings checkbox | bool | ok/error | R/R | **No** — pure OS registry op, never touched the engine | Electron shell (native autostart API / electron-builder config) |
| 34 | OS notification: claim arrived | `notify.go` `claimArrived`, on state diff | prev/now snapshots | notification | **New — event stream needed** | **No** — logic lives only in the Fyne shell, diffing snapshots itself | **New capability**: this decision logic (diff-based, no engine equivalent) should move to Electron main consuming the WS stream (#2), reading the same diff it does today, OR become a first-class engine-emitted event if other shells will want it too. Flagged as the one place real "policy" logic is trapped in companion-desktop. |
| 35 | OS notification: hold nearly up | `notify.go` `holdNearlyUp` (debounced, 15min window, hourly re-fire) | now snapshot + local debounce state (`holdWarned` map) | notification | **New — event stream + local debounce state** | **No** | Electron shell, same as #34 — debounce state can live in the main process (per the Guardrail: no domain logic in the renderer, and this is arguably not domain logic but a UI-notification policy) |
| 36 | OS notification: sync failed while hidden | `notify.go` `syncFailedUnseen`, edge-triggered on `LastError` change + window-hidden check | now snapshot + window visibility | notification | **New** | **No** | Electron shell — needs to know window visibility, which only Electron main has |
| 37 | Toggle notifications on/off | Settings checkbox | bool | — | local only, stored in a companion-desktop-only pref file (`companion-desktop-notify.off`, sibling to config) | N/A | Electron shell — this pref file is companion-desktop-specific and does not need to survive; Electron can use its own local storage |
| 38 | Persist/restore window size | `ui.go` `restoreWindowSize`/`saveWindowSize` | — | — | local only | N/A | Electron shell (`BrowserWindow` bounds persistence) |
| 39 | Show diagnostics (scan trail, tried paths, save paths, build hashes) | Settings "Open diagnostics" | — | reads from existing snapshot fields (`Discovered.Probes`, links, version) | R/R (no new data — assembled client-side from #1) | Data yes (via `/api/state`), dedicated diagnostics view no | Renderer, from existing state |

**Summary of genuinely NEW operations** (not currently on
`companion/server.go` and not solvable by pointing at an existing
route):

- **#2 — live push of state changes.** ~~New~~ **done in Phase 1**:
  `GET /api/events`, server-sent events rather than the WebSocket the
  cutover doc named — core's custody stream already sets that pattern
  and the traffic is one-directional with no payload. The renderer
  re-fetches `/api/state` on `event: changed`.
- **#27 — open a URI/folder.** Currently in-process only
  (`companion.OpenURI`). Becomes an Electron `shell.openPath`/`shell.openExternal`
  IPC call, not a Go route.
- **#29 — raise/focus.** ~~New~~ **done in Phase 1**: promoted into
  `companion/server.go` as `POST /api/raise`, wired to
  `ServerOptions.Raise`. Neither shipping shell supplies one yet, so it
  answers 501 naming where the ability lives; the Electron shell raises
  its own `BrowserWindow` over IPC and can pass a `Raise` later if the
  single-instance handshake wants to go through HTTP.
- **#31 — native file/folder picker.** Electron main IPC
  (`dialog.showOpenDialog`), replacing `sqweek/dialog`. `GET /api/browse`
  stays for the plain-browser fallback only.
- **#33 — autostart toggle.** Pure OS integration, Electron main /
  electron-builder, no Go daemon involvement at all.
- **#34, #35, #36 — the three notification policies in `notify.go`.**
  These are the one place actual decision logic (not just OS plumbing)
  is currently outside `companion/`. They need either (a) a WebSocket
  event stream Electron main diffs itself (mirroring what `notify.go`
  already does against snapshots), or (b) promotion into the engine as
  emitted events, if a future non-Electron consumer would also want them.
  Recommendation for Phase 1: keep as Electron-main policy reading the
  state stream, since the underlying data (links, holder, expiresAt,
  LastError) is all already in `Snapshot()` — no engine change strictly
  required, just don't drop the debounce/edge-trigger logic when porting.

## 4. go.work / module situation and `checkbounds.sh`

**Resolved in Phase 1 (2026-08-23).** The `companion-desktop` module is
deleted and gone from `go.work`, which now lists `.` and `./anvil` only.
The daemon is `cmd/companiond` inside the root module — a thin main over
`companion/`, the way the console binaries are thin over `core`. Option
(c) below is what `scripts/checkbounds.sh` now enforces, extended to say
what the daemon *may* do as well: `companion/`, `cmd/companion` and
`cmd/companiond` may import `core` (savesync's engine and the shared
DTOs) but never a game module and never anvil. The game-blind rule for
`web/companion`'s TypeScript is unchanged.

- `go.work` (repo root) lists three modules: `.` (root, includes
  `core/`, `games/*`, `cmd/*`, `companion/`), `./anvil`, and
  `./companion-desktop`. `companion-desktop` has its own `go.mod`
  (module path implied by its Fyne/sqweek/systray/golang.org/x/sys
  dependencies, none of which the root module needs) — it is a sibling
  module, not a package inside the root module, same shape as `anvil`.
- **`scripts/checkbounds.sh` does not mention `companion-desktop` at
  all.** It has rules for `core/` (no game/console/anvil imports, no
  gametest in production code, docker create/remove/pull confined to
  anvil), for `anvil` (no console/core references), for game modules
  (no cross-game imports), for agent packages (no console-side game
  import), and a "game-blind" rule for `web/reliquary` and
  `web/companion`'s TypeScript source. Nothing constrains
  `companion-desktop` today — it could import anything without tripping
  the guard. This is a real gap the cutover should close explicitly in
  Phase 1 step 5, whether `cmd/companiond` ends up (a) added to the
  existing loop that checks `games/*`-style isolation, (b) given its own
  game-blind check like `web/companion`'s TS source (the daemon reports
  filesystem-scanned game names as data, same rule as the frontends),
  or (c) simply confirmed to import `companion/` only and nothing from
  `core`/`games`/`anvil` — consistent with "reliquary and the companion
  are game-blind."

## 5. Verification: every Fyne event handler mapped

Searched `companion-desktop/*.go` for widget callbacks — `OnTapped`,
`widget.NewButton*`, menu item actions (`fyne.NewMenuItem`), and every
`func (u *ui) ...` method that either directly is a callback or is
called from one — across `dialogs.go`, `screens.go`, `worlds.go`,
`games.go`, `tray.go`, `notify.go`, `picker.go`, and the autostart files.

Every callback found resolves to one of:

- an existing `companion.App` facade method already routed in
  `companion/server.go` (operations #1–#26 above), or
- one of the new operations #27–#39 identified in §3 and carried
  through to the "genuinely NEW operations" list, or
- pure local UI state with no server-side meaning (window size, dialog
  open/close, tab switching, clipboard write, form field text) — not an
  "operation" in the API-surface sense, just renderer/Electron-main
  local state.

**Handlers I could not map to an operation:** none. Every button,
checkbox, and menu item traced back to a single entry above. The two
handlers that took the most digging were `showSettings`'s autostart
checkbox (traces to #33, pure OS registry, no engine call at all) and
`notify.go`'s three `announce()` sub-functions (trace to #34/#35/#36,
decision logic with no current engine or HTTP counterpart) — both are
called out explicitly in §3 rather than folded silently into "existing
route," since they are the two cases where "already exists on the
server" is false.

## 6. Coverage against the redesign's state needs

`design_handoff_companion_redesign/README.md`'s State section requires:
`view`, `worlds[]` (`custody: {state, holder, expiresAt}`), `games[]`
(`linked, worldId, saveLocationKnown, hidden`), `sync` (`status,
lastSyncAt, queue[]`), `filter`.

- `worlds[]` + `custody` — covered by `GET /api/state`'s `Links` +
  `Sync.Worlds` (the `custodyOf()` derivation in `model.go` — discarded
  as a file, but the *rule* it implements already lives correctly in
  `web/companion/src/lib` and needs no new engine data; `World.Holder`,
  `World.ClaimedBy`, `Holder.ExpiresAt` are already in the DTOs aliased
  in `facade.go`).
- `games[]` with `hidden` — covered by `Discovered.Games[].Hidden`
  (resolved server-side in `Snapshot()`) and `linked`/`worldId` via
  matching against `Links` (same `linkFor` logic, already client-side
  today in both the Fyne and web builds).
- `sync.status/lastSyncAt` — covered by `Sync.Configured`/`LastError`/
  polled-at fields already in `syncState`.
- **`sync.queue[]` (offline queued-work list) is NOT covered.** The
  offline banner in the current Fyne UI explicitly does *not* claim to
  list queued work — `worlds.go`'s `offlineBanner` comment says "The
  engine has no queue of unsent work to list... does not invent a
  manifest of it." The redesign's Offline screen wants a `queue[]` of
  `{time, what, size}` entries. This is a genuinely new piece of engine
  state, not just a new route — `companion/` does not track a
  send-queue today. Flagging for whoever picks up the redesign
  implementation: either the engine needs to start recording queued
  local writes, or the queue view is scoped out of the first cut.
  **Phase 1 half-answer:** `sync.queue` is now a real field on the
  snapshot (`[]QueuedWork` of `{what, worldId, worldName, time, size}`)
  and is *always empty* — so the renderer can say "nothing queued"
  honestly instead of inventing a manifest. Filling it is still the
  engine change described above.
- `filter` (games tab search/segment) — pure renderer-local state, no
  server operation needed (same as today's Fyne `matching()` /
  `gamesToolbar` client-side filtering).
