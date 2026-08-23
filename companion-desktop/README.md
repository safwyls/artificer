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

## What this phase does not do

- Packaging (`electron-builder` config, `extraResources`,
  `app.isPackaged` branching) — Phase 5.
- Porting any screen from the old Fyne UI — the renderer is
  `web/companion`, built and iterated on separately (Phase 4).
