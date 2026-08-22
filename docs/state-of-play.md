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
cannot collide. Both scopings hold against an older deployed Anvil too:
the console re-filters containers by their `managed` flag, and re-scopes
an images answer that lacks the newer `scoped: true` declaration — hit
for real on 2026-08-18, when updated consoles ran against the
pre-scoping Anvil still deployed on the host and every image on the
machine reached the page. Containers Anvil labels as this console's but that match
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
  "Empirical findings", outranks everything above it in that file. The
  SPUD object layer below the world header is byte-mapped too, verified
  against a real 4 MB played save (recon, 2026-08-19): on current game
  builds the world save carries each character's guid and last position
  as binary transform records for every character who has played, and a
  **full JSON sheet for whoever is connected at that moment** — the
  server caches a player's record while they are on and drops it at
  logout (corrected 2026-08-20; the earlier "the server never holds
  it" came from one snapshot taken with nobody online). `dwsave` reads
  both; `dwapi`'s record memory keeps an offline player's sheet on
  screen, stamped with when it was true, and persists what it read from
  the save (core's neutral `game_state` table) so a console restart
  does not empty the view. Names come back through the
  disconnect log line's guid↔name pairing, which `dwlog` learns and the
  world endpoint overlays. The Artificer Companion app (`cmd/companion`,
  born wkcompanion — `games/dragonwilds/docs/companion.md`) is now the
  supplement rather than the only path: it keeps a sheet current for a
  player who has not logged in, covers characters this console never saw
  online, and gives each player a local view.
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

## Save sync (2026-08-21)

Shared-world custody — checkout/check-in of peer-hosted saves with one
holder at a time, versioned archives, claim-next and lease renewal —
per `docs/save-sync-architecture.md` (that document is the contract;
read it before touching custody semantics), built first inside
wildskeeper and moved the next day to its own service (the doc's
"option-B pivot" section says why). The shape now: **reliquary**
(`cmd/reliquary`, its own image) holds worlds, versions, users and
tokens over the `core/savesync` engine (the lock is the unique active
session row; only the active session moves the head; late check-ins
become flagged conflicts, never overwrites) — deliberately game-blind,
with game knowledge arriving as metadata the companion reports. The
Artificer Companion (`cmd/companion`, born wkcompanion; GitHub releases
+ bundled in the reliquary image, `docs/companion.md`) discovers
installed games, links their save folders to worlds, and moves the
saves; the agent's `PUT /v1/files/save` lets a dedicated server hold a
world through the world's own agent link. Consoles host none of it.

Both UIs are React frontends now — `web/reliquary` for the service and
`web/companion` for the player-side app — built and embedded like the
three consoles' and structured the same way — app shell, router,
`lib/api`, per-component tests. It replaced the single 702-line vanilla
page on 2026-08-21 to the plan in `docs/reliquary-ui-rebuild.md`; the
vault's visual identity was kept rather than redesigned; the companion
followed on 2026-08-22 (`docs/companion-ui-rebuild.md`), wearing the
same palette because it is the player-side half of one system. What
those pages had learned is now enforced by tests rather than by
comments: the whole
record goes to the user-update API (a partial write clears the fields it
omits — and the same is true of the world-settings API, which the
Settings and Server link tabs both write through), covers are fetched
once per *set* of worlds and never on the poll, and a value that
contains quotes or angle brackets renders as text with its verbs still
working. On the companion the same list runs: covers and save-location
hints are asked for when the *set of installed games* changes rather
than once at boot (the bug that made good IGDB credentials look dead),
the scan trail's open state belongs to the player and survives the
five-second poll, a half-filled link form is never clobbered by that
poll, link failures render inside the dialog, and each panel fails
alone. **Verified in tests only so far** — no real friend-group rotation has
run through it yet, and the phase 0 recon items below gate calling it
done.

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
- **Save-sync recon is open.** Where the player-hosted Dragonwilds world
  save lives is unverified — the companion's discovery marks its
  candidates as guesses and the player confirms the folder; the settle
  window is the torn-save guard (the game-blind companion has no process
  check). Discord slash commands (phase 5) and the Witchspire decision
  (phase 6) are on the roadmap.

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
