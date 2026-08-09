# State of play

Written 2026-08-09 as a handoff. Read this first, then
[`dragonwilds-recon.md`](dragonwilds-recon.md) — between them they hold
every fact the code rests on and every place it is still guessing.

## What this is

**Wildskeeper** (module `github.com/safwyls/dwcon`) is a management console
for a self-hosted **RuneScape: Dragonwilds** dedicated server. It was built
by copying [palcon](https://github.com/safwyls/palcon) — its Palworld
sibling — and removing Palworld. The maintainer wanted a **separate repo**,
not a second game inside palcon; if you find advice to the contrary in
`dragonwilds-plan.md` §0, that decision was overridden and the plan says so
at the top.

The architecture is palcon's, kept structurally identical so fixes can
travel between the two. `porting-to-another-game.md` describes the seam.

## The one fact that shapes everything

Dragonwilds has **no RCON, no REST API, and no query protocol**. There is no
way to ask the game anything. Every piece of live state is *derived*:

| What the UI shows | Where it actually comes from |
|---|---|
| Server up/down, uptime | palagent's `/v1/health` → supervised process state |
| Player list | a state machine (`dwlog`) over the agent's stdout log ring |
| Config | `DedicatedServer.ini` read at rest (`dwconfig`) |
| Saves | files on disk, synced through the agent |

So **the agent is not optional here** — it is the only transport. Anything
that bypasses it isn't testing the real path. Commands that have nowhere to
go (`Broadcast`, `Kick`, `Ban`, `Unban`, `Save`, `Shutdown`) return
`*game.UnsupportedError`, which `internal/api/actions.go` maps to **HTTP
501** — deliberately distinct from 502, so the UI can say "this game can't"
rather than "the server is unreachable".

## Where things stand

`go test ./...` and `cd web && npm test` green, production build fine.

**Done:** Phase 0 (recon), Phase 1 (game package + client + config + log
tracker), Phase 2 (agent launch profile + provisioning + Raise-a-server
wizard), Phase 3 (the `dwsave` save reader — world *metadata*, see below),
Phase 5 (the Wildskeeper frontend).

**Not done:** Phase 4 (the dwbridge UE4SS mod that would unlock the
command tier), and everything in the save beyond the header — `dwsave`
reads the INFO chunk and level names, not players or inventories, so the
visibility roster still reports unavailable.

### Verified by hand against a real server

A live server was stood up and the whole stack driven through it. These
aren't assumptions:

- The agent starts/stops the game; config enforcement rewrites exactly
  `ServerName` / `AdminPassword` / `OwnerId` and leaves `ServerGuid`,
  `WorldPassword` and the `;METADATA` comment alone.
- `info` / `players` / `metrics` derive correctly (`transport: agent`,
  `maxplayernum: 6`, real uptime).
- All six commands answer 501 with their real reasons.
- Log tail, the ini editor, admin-password rotation, and a real backup of
  an actual SPUD save all work. Stop is clean and reports `stopped`, not
  `crashed`.
- `dwsave` (Phase 3) parses both the committed fixture and the live,
  five-autosaves-later world from the same install, and the GUID it
  renders is byte-for-byte the `WorldSaveGuid` the server writes in its
  own log — the decode is checked against the game, not just itself. Two
  clock facts worth knowing: the header's Z-suffixed timestamps actually
  record host-local time (trust the file's mtime instead), and
  `Meta_SaveFileRevision` counts up once per save — an autosave odometer.

## Things that will waste your time if you don't know them

1. **Saves are SPUD, not GVAS.** Magic `SAVE`, chunked, readable
   length-prefixed strings. `uesave` and gvas libraries will not open it.
   A real 57 KB fixture is at
   `internal/games/dragonwilds/testdata/world-empty.sav`.
2. **The game does not save on shutdown.** Clean stop is ~2 s, exit 143,
   world file byte-identical. Autosave is a CVar
   (`dom.StateSaveFrequencyMins:5`), not an ini key, and was observed
   firing exactly five minutes after a world-load save on an idle server.
   So a restart costs up to ~5 minutes of play — the UI says so, and it
   must keep saying so.
3. **`OwnerId` is required to boot but not validated.** The server refuses
   to start with it empty, yet boots happily on the literal string
   `test123`. So never reject an id for failing the shape.
4. **Player IDs are 32 hex chars, and the case varies by context** — the
   Settings screen shows lowercase, the server writes uppercase
   (`ServerGuid`, `WorldSaveGuid`). `CanonicalUID` folds case for
   `^[0-9a-fA-F]{32}$` and only trims anything else. Getting this wrong
   fails *open* on visibility checks.
5. **An idle server logs almost nothing** — an EOS session heartbeat every
   ~30 s and an autosave every 5 min, and that is all. Liveness must never
   be inferred from log activity.
6. **`RSDragonwildsServer.sh` is only a wrapper.** Killing it leaves the
   binary running; signals go to the process group.
7. **Steam and Epic both work.** The binary links both SDKs and EOS
   federates Steam logins, so "Epic auth" does not mean "needs an Epic
   account".

## Still unknown, and why

All four need **a player actually in the world** — a headless server cannot
produce them:

1. **Join/leave log lines.** The last regexes in `dwlog` resting on
   community report rather than observation. Consequence: the Adventurers
   list may show nothing even when people are online. Highest-value gap.
2. **Where bans live on disk** — decides whether offline ban/unban can be
   a file edit or stays in-game only.
3. **Autosave with players present** — the 5-minute interval is confirmed
   idle; whether activity changes it is untested.
4. **Whether a second well-known UDP port opens.** Idle, the server binds
   only 7777 plus an *ephemeral* port — never the 7778 the sources claim.
   Provisioning still reserves the pair defensively; the docs no longer
   state it as fact.

Capture them by getting a client connected and running
`./scripts/dev-local.sh logs 200`.

## Running it locally

```sh
./scripts/dev-local.sh install   # one-off, ~5 GB (already done on this box)
DWDEV_OWNER_ID="<32-hex id>" ./scripts/dev-local.sh up
./scripts/dev-local.sh start
./scripts/dev-local.sh status
./scripts/dev-local.sh down
```

Dashboard on `:8080` (`admin` / `localadmin123`). The install lives at
`~/dwtest/server`; the stack's runtime state at `~/dwtest/local`. **As of
this handoff all three processes are running.**

Two environment quirks, both already applied on this machine: SteamCMD is
32-bit and needs `glibc.i686` plus a `/etc/ssl/certs/ca-certificates.crt`
symlink to `/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem`, or it
reports "needs to be online" and gives up.

**A Windows game client cannot reach this server at `127.0.0.1`.** WSL2 in
default NAT mode forwards TCP only, and the game is UDP. Either set
`networkingMode=mirrored` in `C:\Users\<user>\.wslconfig` and
`wsl --shutdown`, or install the Windows depot (`4019831`) natively. This
is unresolved and is what currently blocks closing the four gaps above.

## Suggested next steps

1. **Get a client connected** (see above) and capture join/leave lines.
   Everything else about the player list is blocked on this.
2. **Deepen the save reader if it earns it.** Phase 3 shipped as world
   metadata: `dwsave` implements `savecache.Source`, the refresher warms
   it, `GET /servers/{id}/world` serves it, and the Saves page shows the
   world panel. Player/inventory state lives in the GLOB/LVLS property
   chunks `dwsave` inventories but does not decode — worth attempting only
   once a real client has written player data into a save to verify
   against (same blocker as the log lines).
3. ~~Build both images before deploying to the NAS.~~ **Done, and the
   doubt was warranted.** Both images now build (first fix: FROM
   references needed registry qualification — podman-style engines
   enforce short-name resolution), and the whole stack ran containerized
   under rootless podman: agent healthy, **game boots and loads the world
   in-container**, metrics/world/logs derive across the container
   network, clean stop in ~1 s. The catch found on the very first run:
   **the game refuses to boot as root** ("Refusing to run with the root
   privileges", exit 134 crash loop), so `Dockerfile.palagent` now bakes
   a `palagent` user (uid 1000) and the generated compose warns that the
   `/dragonwilds` volume must be writable by that uid. The `-healthz`
   healthcheck is verified under docker-format builds (OCI builds drop
   HEALTHCHECK silently). Still untouched by real infrastructure: the
   provisioner (fake API in tests only) and an actual SteamCMD install
   from inside the container (the test bind-mounted the existing one).

## Loose ends

- **~1,100 lines of orphaned advisor backend** (`internal/advisor`,
  `internal/api/advisor.go`) — a Palworld pal advisor whose UI was deleted
  and whose `/pals` data source no longer exists. It compiles and is
  unreachable. Removing it was offered and not yet answered.
- `internal/rcon` and `rcontest` ship with no importer outside their own
  tests — kept as inherited generic packages, currently inert.
- Shared-layer tests use a test-only REST game in `internal/game/gametest`,
  because Dragonwilds itself is a poor instrument for testing the shared
  plumbing. **Production code must never import it.**
- The maintainer's real Player ID is deliberately **not** in this repo;
  every committed example is synthetic.
