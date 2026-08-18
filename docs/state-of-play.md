# State of play

The handoff document: what exists, what has been verified against a real
server, what is still inference, and what to do next. Last updated
2026-08-18.

If you are picking this up mid-flight, read this first, then
`docs/unification-plan.md` for why the repo is shaped the way it is.

## Where the project is

The unification is **done**. Three consoles and one host service, all
building from this repo, all verified against live servers:

| Console | Game | Gate passed | Deployed |
|---|---|---|---|
| flametender | Enshrouded | 2026-08-16 | yes |
| wildskeeper | RuneScape Dragonwilds | 2026-08-16 | yes, since 2026-08-10 |
| palcon | Palworld | 2026-08-17 | yes |
| anvil | — (host service) | 2026-08-13 | yes |

No legacy tree remains in this repo, every image publishes from here, and
the four old repositories are archived with pointer commits.

Phase 6 closed the last four §F guards in the drift ledger on 2026-08-18 —
two of which turned out to be live defects, not just missing tests. See the
ledger's §F rows.

The host dashboard landed 2026-08-18: every console has an admin-only
Host page (`GET /api/host`) rendering what Anvil manages on the machine —
its containers (every console's) with lifecycle state, and the images
behind them (Anvil's new `/v1/images`, additive to API v1; a console
against an older Anvil says "upgrade to see images" rather than failing
the page). Deliberately scoped to Anvil's stack: on a shared box the
unrelated apps and their images stay out of the browser entirely
(`?managed=1` on `/v1/containers`, allowlist-scoped `/v1/images`), while
the wizard keeps its host-wide port view internally so proposals still
cannot collide. Containers Anvil labels as this console's but that match
no server row are flagged as adoptable orphans; both lists filter and
sort. Read-only by design: every mutation stays with the flow that owns
it.

## What is verified, and what is not

This distinction is the most valuable thing in this document, because every
game in this project turned out to differ from its public documentation in
ways that mattered.

**Verified against real servers**, and safe to build on:

- Dragonwilds: the save format is **SPUD, not GVAS**; the server does
  **not** save on shutdown; join/leave log lines, player id shape, and the
  ban location (ini `KnownPlayerList`) are all confirmed against a real
  client that joined on 2026-08-09. `games/dragonwilds/docs/recon.md`,
  "Empirical findings", outranks everything above it in that file.
- Enshrouded: the log vocabulary, the readiness marker and the config
  schema, all confirmed 2026-08-15. Saves on shutdown plus a 10-minute
  autosave; graceful stop is SIGINT; one UDP port carries both game traffic
  and Steam A2S. `games/enshrouded/docs/recon.md` holds the verification
  ledger, and the rows still marked open are genuinely open.
- Palworld: the whole surface, longest-running of the three.
- The data-directory ownership trap (see below), hit for real on
  2026-08-17.

**Still inference**, and marked as such where it matters: the open rows in
Enshrouded's verification ledger, and everything Dragonwilds' recon doc has
not moved into "Empirical findings". Facts marked `[uncertain]` are not
relied on anywhere in code — keep it that way.

## Traps that have actually bitten

- **The data-directory ownership trap.** Anvil's data root must be mounted
  into the Anvil container at the same path it is registered as `dataRoot`.
  Without that mount, Anvil chowns a directory inside its *own* filesystem,
  Docker then creates the real bind source owned by root, the game
  container cannot write it, and **SteamCMD exits 0 having installed
  nothing**. The only symptom is Start answering "game is not installed"
  forever, after an install job that reported success. Anvil structurally
  cannot detect this; the agent settles it from inside the container
  instead (`installDirOk` means exists *and writable*, and an install that
  produces no game says so). Full writeup in
  `docs/palcon-port-verification.md`.
- **Two writers on one config file.** Enshrouded's in-game ban UI rewrites
  `bannedAccounts` on shutdown. Writing that key while the game is up loses
  the edit; the queue-and-apply-during-restart path (`OfflineConfigWork`)
  exists for exactly this and must not be bypassed.
- **Deploy collisions on a shared host.** Two consoles on one machine will
  propose the same host ports unless the wizard asks Anvil what the machine
  already publishes. It does now.

## Known gaps

- **dwbridge is one command deep.** Dragonwilds reaches the game through a
  UE4SS mod; `Save` works end to end and everything else returns
  `game.UnsupportedError` (HTTP 501) until the mod implements it. That is a
  deliberate honest-501 rather than a hidden feature.
- **Enshrouded has no save reader.** The save index reader and rollback are
  Phase 3 on its roadmap; the world blob has no public parser, so only
  metadata is reachable.
- **No update watcher anywhere.** SteamCMD update exists as a verb on every
  agent; nothing polls Steam build ids to say "you are behind". Version-
  gated joins make this the feature that keeps a server *playable* rather
  than merely up.
- **TLS between console and agent** is still deferred; today it is plain
  HTTP on a trusted network, with a bearer token.

## Working rules

- **Frozen API.** Image names, environment variables, ports and volume
  layouts are what running deployments depend on. A rename is a migration
  with a documented path, not an edit — see the ilmari→anvil label
  compatibility in `anvil/internal/host/client.go`, and the retired-name
  warning in `core/config`.
- **A game that cannot do something says so**, with a 501 naming where the
  ability actually lives. Hiding the feature makes the console look broken;
  saying so tells the operator what to do instead.
- **Core owns what `gametest` can exercise.** Core must build and pass its
  whole suite with only the test game registered.
- **Anvil holds the Docker rights, and only Anvil.** No console grows them
  back. Adding a fourth console must need no code in Anvil.

## What to do next

`docs/roadmap.md` has the per-game detail. The near-term shape:

1. **Enshrouded Phase 3** — the save index reader and rollback, the biggest
   single gap in any of the three consoles.
2. **The update watcher**, shared: one implementation over the agents'
   existing SteamCMD verb, useful to all three games.
3. **Dragonwilds' dwbridge command surface** — each command the mod gains
   turns a 501 into a working button with no console-side work.

## Tests

```sh
go build ./... && go vet ./... && go test ./...
./scripts/checkbounds.sh && ./scripts/checkdocs.sh
cd anvil && go test ./...
cd web/<console> && npm ci && npm run build && npm test
```

The save-backed Palworld tests need `palworld-save-tools` importable by
`python3`; they skip without it rather than failing.
