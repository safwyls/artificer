# Sidecar agent (`palagent`) design

Status: phase 1 in progress (2026-07). This documents the agreed design for
managing Palworld servers directly from the dashboard via a per-server agent
container, replacing the pile of bind mounts palcon needs today.

## Why

Everything palcon does beyond RCON/REST rests on one assumption: palcon
shares a filesystem and a docker host with the game server. The save viewer,
settings editor, backups and SteamCMD cache repair each need another bind
mount wired into palcon's container, and none of them work when the game
server lives on a different machine. The recurring failure that motivated
this design: a Palworld game update corrupts the game container's SteamCMD
manifests/package cache, and the container's embedded updater then fails on
every start. Palcon can only patch around that from the outside.

The agent inverts the privilege direction. A small trusted container sits
*next to* each game server, holding the mounts and the SteamCMD tooling, and
palcon becomes a pure control plane speaking HTTP to it.

## Shape

- **One agent per game server.** "Palcon + a fleet of palagents", never one
  agent supervising many servers. Each agent owns exactly one game server:
  its own volume, its own auth token, its own compose stack. Blast radius,
  updates and restarts stay per-server by construction.
- **Fixed verbs, not an exec agent.** Like the docker-socket-proxy rationale
  in `internal/dockerctl`: the agent exposes only dashboard-shaped
  operations. A compromised palcon (or leaked token) can bounce/repair one
  game server and touch its files — nothing else.
- **Same repo, second binary.** `cmd/palagent`, sharing internal packages
  (e.g. `internal/steamops`) with palcon so file operations behave
  identically whichever side executes them. Published as its own image
  (`Dockerfile.palagent`), versioned with a compatibility handshake.

## Two modes

**Companion (phases 1–2, shipped).** The existing game image keeps
running the server; the agent mounts the same `/palworld` volume and absorbs
the file-side features:

- SteamCMD repair: clear `steamapps/*` + `steam/packages/*`, and run
  `steamcmd +app_update 2394010 validate` itself against the shared volume
  — fixing the update-corruption class properly instead of restart-and-pray.
- File verbs (phase 2): the world save directory as a tar bundle
  (ETag/304, so unchanged polls transfer nothing) and
  `PalWorldSettings.ini` GET/PUT (atomic write). Palcon mirrors these into
  `DATA_DIR/agentfiles/<id>/` via `internal/agentfiles`, and the save
  parser, settings editor and backup archiver consume that local cache —
  they never learn agents exist. Game-log tail is deliberately deferred to
  supervisor mode: the companion agent shares a volume, not a PID
  namespace, and container stdout already flows through the docker proxy.

Container power stays with the docker socket proxy in this mode. The agent
cannot see the game process (separate container), so palcon — which can —
refuses SteamCMD updates while the container is running.

**Supervisor (the end state, phase 3).** The same agent image *is* the
server container: it installs the game via SteamCMD and runs `PalServer.sh`
as a child process. Start/stop/restart, update, crash detection and clean
shutdown become agent verbs; the docker proxy becomes unnecessary for these
servers; the watchdog gets real exit codes. A server graduates from
companion to supervisor by redeploying its stack — palcon only cares what
`/health` reports.

## Lifecycle coupling (the question that shaped this)

- **Palcon restarts never touch game servers**, in either mode. The agent
  is not a child of palcon and holds no live connection; the relationship is
  request/response. Long-running verbs are **jobs**: `POST` starts the work
  and returns immediately, palcon polls status. A palcon restart mid-update
  orphans nothing — the same `context.WithoutCancel` philosophy as the
  power-stop sequence, one level up.
- **Companion mode has zero lifecycle coupling anywhere.**
- **Supervisor mode couples the game to the *agent image* only**: updating
  the agent restarts the game (it's the parent process; containers can't
  re-exec across image updates). Mitigation: keep the agent tiny and boring
  so it updates rarely; palcon orchestrates agent updates like scheduled
  restarts (warn, save, stop, recreate). Palcon can update weekly while
  agents update a few times a year.

## Deployment rule: separate stacks

Palcon's stack (dashboard + docker-proxy) and each game server's stack
(game + agent, or supervisor-agent alone) are **separate compose files**
joined by a shared external network. `docker compose down` on the palcon
stack must be structurally unable to take a game server with it. The
compose-snippet generator (phase 4) emits standalone per-server stack files,
never service blocks to paste into palcon's own stack.

Companion-mode example (game server stack):

```yaml
# palworld-main/docker-compose.yml — one stack per game server
services:
  palworld:
    image: thijsvanloef/palworld-server-docker:latest
    volumes: ["./palworld:/palworld"]
    # ... existing game config unchanged ...
  palagent:
    image: ghcr.io/safwyls/palagent:latest
    volumes: ["./palworld:/palworld"]
    environment:
      - PALAGENT_TOKEN=${PALAGENT_TOKEN}   # generated in the palcon UI
    networks: [palcon-net]
networks:
  palcon-net:
    external: true
```

## Agent API (v1)

All `/v1/*` routes require `Authorization: Bearer <token>` (constant-time
compare; agent refuses to start with a token under 16 chars). Bare
`GET /healthz` (204, no body) exists for container healthchecks only.

| Verb | Route | Notes |
|---|---|---|
| GET | `/v1/health` | agent version, API version, mode, install dir status, disk free, current/last job |
| POST | `/v1/steam/clear-cache` | empties `steamapps/*` and `steam/packages/*`; returns `{removed}` |
| POST | `/v1/steam/update` | starts a SteamCMD `app_update` job (`{"validate": bool}`); 202 + `{job}`; 409 if a job is already running |
| GET | `/v1/jobs/{id}` | job status: state, timestamps, exit ok, capped log tail |

Phase 2 adds the file verbs (save serving, config, backups, logs); phase 3
adds `/v1/power/*` for supervisor mode. Never a generic exec or arbitrary
path parameter.

## Auth

Per-agent bearer token, generated by the admin (palcon UI suggests one),
stored encrypted in the `servers` row exactly like the RCON/REST passwords,
pasted into the agent's environment. Plain HTTP on the shared compose
network to start; for cross-host deployments, TLS with a pinned self-signed
cert fingerprint stored alongside the token (later phase). The
reverse-connection variant (agent dials out over WebSocket for NAT'd hosts)
is deferred; the verb surface doesn't change if it's added.

## Palcon integration

- `internal/agentctl` — client mirroring `dockerctl`'s structure.
- `servers` gains `agent_url` + `agent_token_enc`.
- Feature resolution per server: agent if configured, else the local
  path / docker proxy it uses today. Bind mounts remain a fully supported
  degraded mode; nothing existing breaks.
- Palcon-side safety that the agent can't provide in companion mode lives
  in palcon: SteamCMD update is refused while the container reports running.

## Provisioning

Palcon never gains docker create/mount rights (see the proxy comment in
`docker-compose.yml` for why). Instead palcon generates the per-server
stack file for the human to apply; in supervisor mode the agent installs
the game on first boot, so "new server" is: paste stack, `docker compose
up -d`, server appears in the dashboard fully manageable.

## Phases

1. **Agent skeleton** — SHIPPED 2026-07 (API v1): token auth, `/health`,
   steam verbs (clear-cache, update/validate as a job), `internal/agentctl`,
   palcon UI. Field-validated on a real TrueNAS deployment.
2. **File verbs** — SHIPPED 2026-07 (API v2): save bundle, config GET/PUT;
   `internal/agentfiles` sync layer feeding the existing parser, editor and
   backup archiver. Retires the save/config/install mounts.
3. **Supervisor mode**: process management, log streaming, exit handling,
   `/v1/power/*`.
4. **Compose-snippet generator** + docs.

## Accepted tradeoffs

- A second published image to build/version (multi-arch), with `/health`
  reporting `apiVersion` so an old agent keeps working with a new palcon.
- Token management UX (generate → paste → env).
- Save reads while the game is writing stay best-effort, same as the
  current backup semantics.
- SteamCMD in the agent image is routine (every server image bundles it);
  supervisor mode is Linux-only, like the containers themselves.
