# Companion Desktop: Fyne → Electron Cutover

Plan of record for moving `companion-desktop` from an in-process Go/Fyne app to a
Go daemon (`companiond`) plus an Electron shell wrapping a React renderer in
`web/companion`.

## Guardrails

Read these before starting. They are the ways this goes wrong.

- **Do not port Go domain logic to TypeScript.** Save parsing, file watching,
  sync, and any local filesystem work stay in Go. The renderer is a view over an
  API and holds no domain logic.
- **Do not substitute Wails or Tauri.** Both put WebKitGTK in the render path on
  Linux, which is the target dev platform. This decision is settled.
- **Do not invent new API conventions.** Match whatever `core/` already does for
  routes, auth, error shape, and store access.
- **Do not preserve Fyne patterns in the new UI.** The Fyne layer is discarded,
  not translated. `web/companion` follows the structure of the existing console
  apps under `web/`.
- Every phase ends with a green verification command. Do not begin the next phase
  with a red one.

## Phase 0 — Inventory

1. Walk `companion-desktop` and classify every file into: **domain** (keep),
   **Fyne UI** (discard), **glue** (rewrite as HTTP handlers).
2. Produce `docs/companion-api-surface.md`: every operation the Fyne UI can
   currently trigger, plus every piece of state it displays. This is the API
   contract. Include for each: name, inputs, outputs, whether it is a
   request/response or a stream.
3. Note which operations are long-running or push-based (file watch events, sync
   progress, log tails). These become WebSocket, not REST.
4. Confirm whether `companion-desktop` is its own module in `go.work` or part of
   the root module, and where it currently sits relative to the dependency rules
   in `scripts/checkbounds.sh`.

**Verify:** the API surface doc exists and every Fyne event handler maps to
exactly one entry in it.

## Phase 1 — Extract the daemon

1. Tag the current state (`git tag pre-electron-cutover`) so the Fyne tree is
   recoverable, then delete the Fyne UI code. Leaving it in place pollutes agent
   context and invites accidental pattern reuse.
2. Restructure into `cmd/companiond` (thin main) over internal packages holding
   the domain logic, mirroring how the console binaries are thin wiring over
   `core`.
3. Implement the API surface from Phase 0 as HTTP handlers plus a WebSocket
   endpoint for the push-based operations.
4. Write Go tests covering each handler. These are the only safety net between
   here and Phase 4 — the UI is gone and cannot exercise the logic.
5. Update `scripts/checkbounds.sh` for wherever the daemon lands in the
   dependency graph. Decide explicitly whether it may import `core` or must stay
   below it.

**Verify:** `go build ./... && go vet ./... && go test ./...` and
`./scripts/checkbounds.sh` both pass.

## Phase 2 — Handshake and lifecycle

1. Bind to port 0 (ephemeral). Print the resolved address on the first line of
   stdout, then nothing else structured on stdout.
2. Read a required auth token from an env var at startup. Reject any request
   missing it. A bare localhost listener is reachable by any process on the box,
   including a page in the user's browser.
3. Exit when stdin closes. This is the parent-death signal; without it, every
   crashed dev session leaves an orphan holding a port and any open file locks.
4. Add a `/healthz` route the shell can poll before showing the window.
5. Add a `--port` and `--token` flag path so the daemon can be run standalone for
   testing without Electron.

**Verify:** launch the daemon by hand, hit `/healthz` with and without the token,
confirm 200 and 401 respectively. Close stdin, confirm the process exits.

## Phase 3 — Electron shell

1. Scaffold the Electron main process. Responsibilities, and nothing more:
   - Generate a token, spawn `companiond` with it, parse the port from stdout.
   - Poll `/healthz`, then create the window.
   - Kill the child on `will-quit` and on `window-all-closed`.
   - Single-instance lock, tray icon, autostart toggle, native file dialogs,
     notifications.
2. Preload script with `contextIsolation: true` and `nodeIntegration: false`.
   Expose the daemon base URL and token to the renderer over `contextBridge` —
   never via a global or a query string.
3. Route all privileged operations (file dialogs, tray state, autostart) through
   IPC to main. The renderer gets no Node access.

**Verify:** shell launches, window appears only after health check passes,
quitting leaves no orphaned `companiond` in the process table. Check that last
one explicitly — it is the most common defect in this architecture.

## Phase 4 — Renderer

1. Scaffold `web/companion` mirroring the structure of an existing console app
   under `web/`. Reuse the shared theme layer; do not fork it.
2. Point Vite dev server at the daemon the same way the console apps talk to
   `core`.
3. Port screens one at a time against the API surface doc. Do not batch. Each
   screen lands with its own commit and passes `npm run build && npm test`.
4. Wire the WebSocket-backed views last, once the request/response paths are
   stable.

**Verify:** `cd web/companion && npm ci && npm run build && npm test`, plus a
manual pass confirming every entry in the API surface doc is reachable from the
UI.

## Phase 5 — Packaging

1. `electron-builder` config with `extraResources` carrying per-platform
   `companiond` builds.
2. Path branching on `app.isPackaged` between `process.resourcesPath` and the dev
   tree. This is where the time goes; budget for it rather than assuming it is a
   config line.
3. Build the Go binary per target platform in CI, not on the dev machine.
4. **Test the tray on Nobara early.** Linux tray behavior varies by desktop
   environment and may require AppIndicator support present on the system. Do not
   defer this to the end.

**Verify:** a packaged build on Linux launches, spawns its bundled daemon, shows a
working tray icon, and exits cleanly.

## Phase 6 — CI and boundaries

1. Add `web/companion` to the npm build matrix alongside the console apps.
2. Add the `companiond` cross-compile to the Go build matrix.
3. Confirm `scripts/checkbounds.sh` still enforces the intended graph with the new
   component in it.
4. Update the root `README.md` layout table and status section.

**Verify:** full CI green on a clean checkout.

## Rollback

`git tag pre-electron-cutover` from Phase 1 is the escape hatch. There is no
partial-rollback story after Phase 1 — the Fyne UI is gone by design. If the
approach is going to be abandoned, abandon it before deleting that code.
