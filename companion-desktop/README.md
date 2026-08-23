# Reliquary Companion (Electron shell)

The native-window replacement for the deleted Fyne `companion-desktop`
module (see `../companion-cutover.md` and `../docs/reliquary-companion.md`).
This directory is a plain Node project — **not a Go module**, and not
listed in the repo's `go.work` — wrapping two things:

- `cmd/companiond`, the headless Go daemon that owns all custody/sync
  domain logic and speaks HTTP on loopback.
- `web/companion`'s built renderer, served by the daemon at `/`.

This shell (`companion-desktop/`) does only what
`../companion-cutover.md`'s Phase 3 assigns it: spawn/health-gate/kill the
daemon, own the window and tray, native dialogs, autostart, and the three
notification policies. No custody/sync/save logic lives here — see the
guardrails in the cutover doc.

## Layout

- `src/daemon.ts` — spawn, handshake (parse the address line off stdout),
  health-poll, and kill (stdin-close, then SIGTERM/SIGKILL escalation).
  Deliberately has no `electron` import so it can be tested headlessly.
- `src/sse.ts` — consumes the daemon's `GET /api/events` SSE stream from
  main using `fetch` (not `EventSource`, which can't carry the bearer
  token).
- `src/notifications.ts` — the three notification policies (claim
  arrived, hold nearly up, sync failed while hidden), as pure logic over
  state snapshots.
- `src/autostart.ts` — login-item toggle: `app.setLoginItemSettings` on
  Windows/macOS, an XDG `.desktop` file on Linux.
- `src/main.ts` — Electron main process entry: wires all of the above,
  the `BrowserWindow`, the tray, single-instance-lock raise, and IPC
  handlers for folder picking / open-path / autostart.
- `src/preload.ts` — the only bridge into the renderer. `contextIsolation`
  is on, `nodeIntegration` is off; it exposes `window.companion` with
  `baseUrl`, `token`, and IPC-backed methods (`pickFolder`, `openPath`,
  `setAutostart`, `getAutostart`). Everything privileged goes through
  `ipcRenderer.invoke`.
- `src/ipc-contract.ts` — the shared channel-name/type contract between
  preload and main.
- `test/daemon.test.js` — a headless Node test (`node --test`) that spawns
  a real `go run ./cmd/companiond`, asserts the handshake, the `/healthz`
  auth gate, and that the process does not survive after `killDaemon`
  closes its stdin (escalating to SIGTERM/SIGKILL if needed).

## Dev workflow

```
npm install
npm start          # builds TypeScript, then launches electron .
```

`npm start` spawns `go run ./cmd/companiond` from the repo root by
default (dev mode). Override with:

- `COMPANION_BIN=/path/to/companiond` (or `COMPANIOND_BIN`) — run a
  prebuilt daemon binary instead of `go run`. This is the seam Phase 5's
  packaging (`app.isPackaged` / `process.resourcesPath` branching) will
  extend.
- `COMPANION_DEV_URL=http://localhost:5173` — point the window at a Vite
  dev server for `web/companion` instead of the daemon's embedded
  `dist/`. Run `cd ../web/companion && npm run dev` separately for this.

The daemon's own auth token is generated fresh per launch inside main and
handed to the renderer only over a synchronous IPC call from preload
(never argv, env, or a query string reaching the page) — see
`src/preload.ts`.

## Testing

```
npm test
```

Builds TypeScript and runs `test/daemon.test.js` against a real
`go run ./cmd/companiond` child process. Requires a working Go toolchain
on PATH; no display/X server needed. This only exercises the
main-process spawn/handshake/lifecycle module — it cannot verify
window-level behavior (tray rendering, dialogs, notifications) without a
real display, which is Phase 3's known gap until a real-machine smoke
(see `../docs/reliquary-companion.md`'s parity checklist).

## Packaging (Phase 5)

`electron-builder.yml` holds the config. Identity per
`../docs/reliquary-companion.md`: `appId: com.artificer.reliquarycompanion`,
`productName: Reliquary Companion` — deliberately separate from the
browser-and-tray build's `artificer-companion.exe` / `companion-latest`
identity, so the two updaters can never collide. Targets: Linux
(AppImage), Windows (nsis), macOS (dmg).

### The staging dir contract

`extraResources` carries whatever sits in `companion-desktop/resources/`
into the packaged app at `process.resourcesPath`:

- `resources/companiond` (Linux/macOS) or `resources/companiond.exe`
  (Windows) — the Go daemon binary for the platform being packaged.

Nothing else about packaging cares how that file got there. Locally,
`npm run build:daemon` (`scripts/build-daemon.js`) builds it for the
*current host platform* via `go build -o resources/companiond
./cmd/companiond` — a dev convenience, not the release path. Phase 6's CI
matrix cross-compiles per target OS/arch and drops the binary in the same
staging spot before invoking `electron-builder` per platform;
`resources/` and `release/` are both gitignored.

### Building a package

```
npm run package     # build:daemon (host platform) + electron-builder, all configured targets
```

or drive the pieces separately:

```
npm run build:daemon                              # stage resources/companiond[.exe]
npx electron-builder --linux AppImage             # or --win nsis / --mac dmg
```

### Path resolution: dev vs packaged

`src/daemon.ts`'s `resolveDaemonCommandForApp` is the `app.isPackaged`
branch: given `{ isPackaged, platform, resourcesPath }` (the only three
`app`/`process` fields `main.ts` needs to pass — see `startDaemon()`
there), packaged runs resolve to `resourcesPath/companiond[.exe]`; dev
runs fall through to the existing `resolveDaemonCommand` behavior
(`COMPANION_BIN`/`COMPANIOND_BIN` override, else `go run
./cmd/companiond`). It's a pure function over plain data — no `electron`
import — so `test/daemon.test.js` covers the packaged-path resolution
(including the Windows `.exe` suffix and the explicit-override-wins case)
without needing a packaged build or a display.

### Icon

`build/icon.png` (256x256, committed) is generated from
`web/companion/public/favicon.ico`, which is only 32x32 — too small for
electron-builder's Linux/AppImage icon requirement (>=256x256). Regenerate
it if the source favicon changes:

```
magick "../web/companion/public/favicon.ico[1]" -resize 256x256 build/icon.png
```

### What has been verified vs. deferred

Verified on this machine (Linux, headless, no FUSE available so the
unpacked `release/linux-unpacked/` tree was run directly instead of the
`.AppImage` under `xvfb-run`):

- `electron-builder --linux AppImage` completes and produces a
  `.AppImage` plus `linux-unpacked/`, with `resources/companiond` present
  in the packaged output.
- Launching the unpacked binary spawns the **bundled** `companiond`
  (`resources/companiond`, not `go run`) — confirmed via `ps -ef` showing
  it as a child of the Electron main process, and via the daemon's own
  stderr line in the app's log.
- Sending `SIGTERM` to the main process cleanly tears the whole tree down
  — no orphaned `companiond`, zygote, GPU, or renderer process survives —
  and the daemon's own log shows the stdin-close shutdown path firing
  (`"stdin closed — shutting down"`).

Deferred to the real-machine smoke (`../docs/reliquary-companion.md`'s
parity checklist covers the equivalent for the Fyne build; this Electron
shell's own pass is still open):

- Tray icon rendering/behavior on Nobara (no display server or desktop
  environment here; Xvfb has no tray host).
- Actual AppImage execution end-to-end (FUSE is unavailable in this
  environment — only the unpacked tree, which is what the AppImage
  extracts to, was exercised).
- Windows (nsis) and macOS (dmg) builds — not attempted here; Phase 6's
  CI does the cross-compiles and platform-specific packaging.
- Native dialogs, notifications, autostart, and anything else needing a
  real window manager / desktop session.

## What this phase does not do

- Cross-platform (Windows/macOS) builds and the CI matrix that produces
  them — Phase 6.
- Porting any screen from the old Fyne UI — the renderer is
  `web/companion`, built and iterated on separately (Phase 4).
