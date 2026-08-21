# The sidecar agent

Every game server this project manages has an agent container next to it —
`palagent`, `wkagent` or `flameagent`. They are one program: the kit in
`core/agent`, plus a `agent.Game` value holding everything that differs by
game, plus a `main`. This document is the design and the reasoning; the
per-game facts are in `games/<game>/<name>agent/`.

The pattern originated in palcon and was copied into the other two consoles
before the unification; the copies drifted, which is part of why `core`
exists. Anything here that reads as game-agnostic is genuinely so — it is
the same code for all three.

## Why

Everything a console does beyond the game's own admin surface rests on one
assumption: that the control plane shares a filesystem and a Docker host
with the game server. The save viewer, settings editor, backups and
SteamCMD repair each need another bind mount wired into the console's
container, and none of them work when the game server lives on a different
machine.

The failure that motivated the design: a game update corrupts the game
container's SteamCMD manifests and package cache, and the container's own
embedded updater then fails on every start. A console can only patch around
that from the outside.

The agent inverts the privilege direction. A small trusted container sits
*next to* each game server, holding the mounts and the SteamCMD tooling,
and the console becomes a pure control plane speaking HTTP to it.

## Shape

- **One agent per game server.** A console plus a fleet of agents, never
  one agent supervising many servers. Each agent owns exactly one server:
  its own volume, its own token, its own stack. Blast radius, updates and
  restarts stay per-server by construction.
- **Fixed verbs, not an exec agent.** Same rationale as the Docker socket
  proxy: the agent exposes only console-shaped operations. A compromised
  console, or a leaked token, can bounce or repair one game server and
  touch its files — nothing else. There is no generic exec and no arbitrary
  path parameter in any mode.
- **One kit, three binaries.** `core/agent` holds the supervision
  skeleton, the job runner, the Steam and file verbs, and auth. A game
  contributes an `agent.Game`: app id, ports, config path, save-directory
  lookup, launch profiles, stop signal and grace, a `PrepareRuntime` hook
  and any extra routes. An agent never imports its console-side game
  package — `scripts/checkbounds.sh` enforces it — so a game's console
  features can't accidentally link into the sidecar.

### What differs per game

| | palagent | wkagent | flameagent |
|---|---|---|---|
| Game | Palworld | RuneScape Dragonwilds | Enshrouded |
| Steam app | 2394010 | 4019830 | 2278520 |
| Install dir | `/palworld` | `/dragonwilds` | `/enshrouded` |
| Config | `PalWorldSettings.ini` | `DedicatedServer.ini` | `enshrouded_server.json` |
| Ports | 8211/udp + REST 8212 + RCON 25575 | 7777–7778/udp | 15637/udp only |
| Launch | `PalServer.sh` | `RSDragonwildsServer.sh` (native or Wine) | `wine64 enshrouded_server.exe` |
| Stop | SIGTERM | SIGTERM | SIGINT — the game saves on it |
| Admin channel | REST, RCON fallback | agent + the dwbridge mod | agent only |

The last row is why the agent matters more for some games than others.
Palworld has a REST API and RCON; Dragonwilds and Enshrouded have neither,
so for those the agent's own API is the only channel a console has, and
anything it can't do is a 501 naming where the ability actually lives.

## Two modes

**Companion.** The existing game image keeps running the server; the agent
mounts the same volume and absorbs the file-side features:

- SteamCMD repair: clear `steamapps/*` and `steam/packages/*`, then run
  `app_update <appid> validate` against the shared volume — fixing the
  update-corruption class properly rather than restart-and-pray.
- File verbs: the world save directory as a tar bundle (ETag/304, so an
  unchanged poll transfers nothing) and the settings file GET/PUT (atomic
  write, refuses to create). The console mirrors these into
  `DATA_DIR/agentfiles/<id>/` via `core/agentfiles`, and the save parser,
  settings editor and backup archiver consume that local cache — they never
  learn agents exist.

Container power stays with the Docker socket proxy in this mode. The agent
shares a volume with the game but not a PID namespace, so it cannot see the
game process; the console — which can — refuses SteamCMD updates while the
container is running.

**Supervisor.** The agent image *is* the server container. `*_MODE=supervisor`
makes it install the game via SteamCMD on first boot and run the game as a
child process in its own process group, so signals reach the real binary
rather than a wrapper script. Start/stop/restart, crash auto-restart with
backoff, and the game's stdout become agent verbs; the console routes
power, status and logs to the agent whenever `/v1/health` reports supervisor
mode, and no Docker proxy is required.

Desired state persists in the install volume, so a recreated agent resumes
what the operator last asked for — Docker's `unless-stopped`, one level
down. That also means scheduled restarts and the in-game-shutdown flow work
unchanged: the game exits, the supervisor brings it back. SteamCMD updates
and a running game are mutually exclusive, enforced agent-side.

Don't combine supervisor mode with a `containerName` and the watchdog on
the same server — the supervisor owns restarts, and the watchdog would race
it.

### PrepareRuntime: the seed-and-enforce step

Before every start the kit calls the game's `PrepareRuntime` with the
install dir, the config path, the profile about to run and the operator's
identity settings. This is where a fresh install is made bootable and
dashboard-issued settings are made authoritative. What each game does with
it is a fact about that game, and the reason the hook is a hook:

- **Palworld** seeds the ini from the game's own defaults, applies name and
  description once, then on every start enforces the admin password and
  `RCONEnabled`/`RESTAPIEnabled` — otherwise the console has no channel.
- **Dragonwilds** seeds `OwnerId`, because the game writes its own config
  on first run and then refuses to start until that field has a value.
- **Enshrouded** writes the whole JSON before first boot, because the
  server's own generated default is an **open** server — seeding after the
  fact means a window where anyone can join.

## Lifecycle coupling

- **A console restart never touches a game server**, in either mode. The
  agent is not a child of the console and holds no live connection; the
  relationship is request/response. Long-running verbs are jobs: `POST`
  starts the work and returns immediately, the console polls status. A
  restart mid-update orphans nothing.
- **Companion mode has zero lifecycle coupling anywhere.**
- **Stopping is asked before it is imposed**, in both modes — but the two
  differ in what "imposed" reaches. In companion mode `docker stop` signals
  PID 1, an entrypoint script that typically swallows SIGTERM, so a
  graceful exit completes untouched. The supervisor signals the game's
  whole *process group* — the launcher is usually a wrapper, and signalling
  only the script leaves the game running — which lands on the engine
  directly. Sent on top of an in-flight shutdown that would cut the final
  save short, so the console passes `?graceful=` when its in-game shutdown
  was accepted, and the supervisor waits that out before escalating to the
  stop signal, then the grace period, then SIGKILL. An operator-initiated
  stop is recorded as a stop regardless of exit code: never a crash, and
  never counted toward the restart backoff.
- **Supervisor mode couples the game to the *agent image* only.** Updating
  the agent restarts the game — it is the parent process, and containers
  cannot re-exec across an image update. Mitigation: keep the agent small
  and boring so it updates rarely. A console can update weekly while its
  agents update a few times a year.

## Deployment rule: separate stacks

A console's stack and each game server's stack are **separate compose
files**. `docker compose down` on the console must be structurally unable
to take a game server with it. The wizard emits standalone per-server stack
files, never service blocks to paste into the console's own stack.

Supervisor-mode stack, as the wizard generates it (Dragonwilds shown):

```yaml
# dragonwilds-main/docker-compose.yml — the agent IS the server
services:
  wkagent:
    image: ghcr.io/safwyls/wkagent:latest
    environment:
      - WKAGENT_TOKEN=${WKAGENT_TOKEN}
      - WKAGENT_MODE=supervisor
      # Enforced into DedicatedServer.ini before every start: the password
      # the in-game Server Management menu accepts.
      - WKAGENT_ADMIN_PASSWORD=${DW_ADMIN_PASSWORD}
      # Required: the game refuses to start until OwnerId has a value.
      # In game: Settings, bottom-left "My Player ID".
      - WKAGENT_OWNER_ID=${DW_OWNER_ID}
    volumes: ["./dragonwilds:/dragonwilds"]
    ports:
      - "7777:7777/udp"   # game
      - "7778:7778/udp"   # the server also binds the port above its own
      - "8811:8811"       # agent API — the console's only channel
    restart: unless-stopped
```

The agent port is 8811 in every game's stack. That is the one container-side
port fact belonging to the protocol rather than to any game.

## Agent API (v1)

All `/v1/*` routes require `Authorization: Bearer <token>`, compared in
constant time; the agent refuses to start with a token under 16 characters.
A bare `GET /healthz` (204, no body) exists for container healthchecks only.

| Verb | Route | Mode | Notes |
|---|---|---|---|
| GET | `/v1/health` | all | version, API version, mode, install/save/config status, disk free, current and last job, game state (supervisor). `installDirOk` means the directory exists **and is writable** — an unwritable bind mount is the trap that makes SteamCMD exit 0 having installed nothing |
| POST | `/v1/steam/clear-cache` | companion, supervisor | empties `steamapps/*` and `steam/packages/*` |
| POST | `/v1/steam/update` | companion, supervisor | SteamCMD `app_update` job; 202 with a job id; 409 while busy, or (supervisor) while the game runs |
| GET | `/v1/jobs/{id}` | all | state, timestamps, error, capped log tail |
| GET | `/v1/files/save` | companion, supervisor | world save directory as a tar bundle, ETag/304 |
| HEAD | `/v1/files/save` | companion, supervisor | the bundle ETag alone — the restore precondition's input (empty install answers the empty set's ETag) |
| PUT | `/v1/files/save` | companion, supervisor | replace the save with an uploaded bundle (docs/save-sync-architecture.md). The one deliberate widening of the fixed-verb posture, still a fixed location: refused while the supervised game runs, `If-Match` on the current ETag required (412 carries the current one), extract-verify-swap with one `.bak` |
| GET/PUT | `/v1/files/config` | companion, supervisor | the game's settings file; PUT writes atomically and refuses to create |
| POST | `/v1/power/{start,stop,restart}` | supervisor | game process control. `?graceful=20s` on stop means the game has already accepted an in-game shutdown — let that exit finish before signalling |
| GET | `/v1/power/logs` | supervisor | game stdout ring buffer |

Games add their own verbs through `Game.Routes` — Enshrouded's A2S query
relay, Dragonwilds' dwbridge control — under the same authenticated router.

## Auth

A per-agent bearer token, generated by an admin (the UI suggests one),
stored encrypted in the `servers` row exactly like the RCON and REST
passwords, and pasted into the agent's environment. Plain HTTP on a shared
network to start; for cross-host deployments, TLS with a pinned
self-signed certificate fingerprint stored alongside the token is the
intended next step. The reverse-connection variant — the agent dialling out
over a WebSocket for NAT'd hosts — is deferred, and the verb surface would
not change if it lands.

## Console integration

- `core/agentctl` is the client, mirroring `core/dockerctl`'s structure.
- `servers` carries `agent_url` and `agent_token_enc`.
- Feature resolution is per server: the agent when configured, else the
  local path or Docker proxy. Bind mounts remain a fully supported degraded
  mode; nothing that worked before the agent stops working.
- The one safety the agent cannot provide in companion mode lives in the
  console: a SteamCMD update is refused while the container reports running.

## Provisioning

**Provisioner mode is retired.** An agent used to be able to hold Docker
create rights and stamp out servers from a locked template; two consoles
doing that on one host could not see each other, and one would propose a
port the other already held. That role now belongs to
[Anvil](../anvil/README.md), one service per machine, and no console or
agent holds Docker create rights any more. `PROVISIONER_URL` and
`PROVISIONER_TOKEN` are no longer read; see `docs/palcon-port-verification.md`
for the migration.

What survived the move is the ordering, because it was right for a reason
worth restating. Two kinds of deploy failure are handled differently:

- A provisioner that **refused** — the name is taken, a port is held, the
  image is not allowed, the token was rejected — made nothing, and pasting
  the same stack elsewhere would collide the same way. The console
  registers no server row and returns the error.
- A provisioner that merely **could not be reached** leaves the paste flow
  intact: the row and the generated stack still describe a server the
  operator can bring up by hand, which is the point of still generating one.

Hence deploy first, register after. Registering first is what once left a
name collision showing as a server in the rail that the console could never
reach, its row carrying credentials the running container had never seen.
