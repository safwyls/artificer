# Wildskeeper (dwcon)

A management console for a self-hosted **RuneScape: Dragonwilds** dedicated
server — built on the reusable base extracted from
[palcon](https://github.com/safwyls/palcon), its sibling project for
Palworld.

Dragonwilds has no RCON, no HTTP admin API and no query protocol; all
native administration is the in-game Server Management menu. Wildskeeper
therefore *derives* everything: process liveness and uptime from its
palagent sidecar, the live player list from a state machine over the
server's log tail, configuration from `DedicatedServer.ini` at rest.
Commands that have no transport (broadcast, kick, ban) answer HTTP 501
with the honest reason instead of pretending — they light up when the
planned UE4SS command bridge exists.

## What works today

- **Overview** — the rune sigil (six segments, one per player slot), power
  controls through the agent, uptime/player vitals, log preview
- **Adventurers** — who's online (log-derived), join/leave history and
  playtime via the collector
- **World saves** — snapshot, download, delete, scheduled backups of
  `Saved/SaveGames`
- **Configuration** — `DedicatedServer.ini` editor (never adds or removes
  keys, type-validated, one-level `.bak`, atomic swap) plus one-click
  admin-password rotation — the game's one real remote-admin lever
- **Server log** — live tail through the agent
- Shared base: users/roles/permissions, audit trail, Discord notifications,
  scheduled restarts, crash watchdog, SteamCMD update jobs (app id
  4019830), public status page

`docs/dragonwilds-recon.md` records every externally-verified game fact and
the open empirical gates (log line shapes, player-id format, save format,
SIGTERM behavior, on-disk ban list). Facts marked UNVERIFIED there are not
assumed anywhere in the code.

## Running it

```sh
cp .env.example .env && export $(cat .env | xargs)
go run ./cmd/dwcon          # backend on :8080
cd web && npm install && npm run dev   # frontend dev server
```

Production: `cd web && npm run build`, then `go build ./cmd/dwcon` (the Go
binary embeds the bundle), or use the `Dockerfile` / `docker-compose.yml`.
The game server itself runs under the `palagent` sidecar
(`Dockerfile.palagent`, `PALAGENT_INSTALL_DIR=/dragonwilds`,
`PALAGENT_GAME_CMD=./RSDragonwildsServer.sh`).

Tests: `go test ./...` and `cd web && npm test`.

## Lineage

The game-agnostic architecture — `game.Definition` registry, sidecar agent,
savecache, collector, backup/notify/sched/watchdog — is palcon's, kept
structurally identical so improvements can flow between the two projects.
`docs/porting-to-another-game.md` describes the seam. Palworld-specific
code was removed rather than ported; a second game would register beside
Dragonwilds the same way Dragonwilds registered beside Palworld.
