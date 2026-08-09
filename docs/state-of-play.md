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
that bypasses it isn't testing the real path. Commands reach the game
through the **dwbridge** mod when it's running (Phase 4, below): `Save` is
live end to end, and the client routes each command through the bridge only
when the mod's heartbeat lists it. A command with nowhere to go — no bridge,
or a verb the mod hasn't implemented — returns `*game.UnsupportedError`,
which `internal/api/actions.go` maps to **HTTP 501**, deliberately distinct
from 502 so the UI can say "this game can't" rather than "the server is
unreachable". (`Shutdown` stays 501 by design: the agent's power controls
own stopping the process.)

## Where things stand

`go test ./...` and `cd web && npm test` green, production build fine.

**Done:** Phase 0 (recon), Phase 1 (game package + client + config + log
tracker), Phase 2 (agent launch profile + provisioning + Raise-a-server
wizard), Phase 3 (the `dwsave` save reader — world *metadata*, see below),
Phase 4 (the dwbridge command channel — its `save` verb works end to end;
see below), Phase 5 (the Wildskeeper frontend).

**Partial:** Phase 4's command surface. The bridge exists and `save` is
proven through the whole stack; `kick`/`ban`/`unban`/`broadcast` are
mapped to real game functions (recon doc, "Command surface") but not yet
implemented in the mod — they need a connected client to build against
safely. Also unfinished: everything in the save beyond the header —
`dwsave` reads the INFO chunk and level names, not players or inventories,
so the visibility roster still reports unavailable.

### Verified by hand against a real server

A live server was stood up and the whole stack driven through it. These
aren't assumptions:

- The agent starts/stops the game; config enforcement rewrites exactly
  `ServerName` / `AdminPassword` / `OwnerId` and leaves `ServerGuid`,
  `WorldPassword` and the `;METADATA` comment alone.
- `info` / `players` / `metrics` derive correctly (`transport: agent`,
  `maxplayernum: 6`, real uptime).
- Before the bridge, all six commands answered 501 with their real
  reasons; now `Save` runs through dwbridge (verified — see Phase 4) and the
  rest answer 501 until the mod implements them.
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

## A real client joined (2026-08-09), and most gaps closed

A player joined and left the local server. What that settled (details in
the recon doc's "Closed 2026-08-09" section):

1. **Join/leave lines: verified.** `dwlog` RulesV1 is written from the
   capture (committed corpus with synthetic ids), keys sessions by the
   real player id, and the Adventurers list now carries real ids through
   `CanonicalUID`.
2. **Bans: located.** The ini's `KnownPlayerList` holds id, name,
   privileges and a `bIsBanned` flag per known player. Whether the server
   honors a hand-edited flag is still untested.
3. **A leave writes state** — `PlayerStateSave result[true]` plus a world
   save at the same instant. The autosave *interval* under activity is
   still unmeasured.
4. **Player state in the save is JSON** (char record keyed by a character
   guid; the EOS id appears nowhere in the save, so identity always routes
   through log/ini). A played world save exists at
   `~/dwtest/server/.../World-75058.sav` for the deeper-parse work; it is
   deliberately not committed since it holds the maintainer's real ids.

Still open: the second UDP port question, ban *enforcement*, chat lines.

## Phase 4: the command channel exists, and `save` works end to end

The dwbridge mod (`tools/dwbridge`) is real, and one command is proven
through the whole stack: `POST /api/servers/{id}/save` in the console wrote
the world on a headless server with no player connected —
dwcon → palagent `/v1/bridge/command` → file IPC → the UE4SS Lua mod →
`PersistenceSubsystem:SaveGame`, `Save completed SUCCESSFULLY` in the game
log, save file rewritten.

The pieces, all committed:
- **`tools/dwbridge`** — the Lua mod. Heartbeat + single-flight file IPC
  (`request.json`/`response.json`; fixed names because `io.popen` and
  rename-over-existing are unreliable under Wine). Commands: `ping`, `save`.
- **`tools/ue4ss-wine-shim`** — how the modded Windows build runs under Wine
  (the server imports no dwmapi, so UE4SS loads via a `version.dll` shim).
- **palagent** — `bridge.go`: the file IPC's other half, `POST
  /v1/bridge/command`, and `health.bridge` (heartbeat freshness + command
  list). Supervisor mode only.
- **dragonwilds client** — commands route through the bridge when the
  heartbeat lists them; otherwise the honest 501 stands. `save` is live;
  the rest map to real functions but await the mod implementing them.

What's left in Phase 4: implement `kick`/`ban`/`unban`/`broadcast` in the
mod (they need a connected client to build against — see the recon doc's
"Command surface"), and a palagent launch profile that runs the Windows
build under Wine (today it's `GAME_CMD`/`GAME_ARGS` config; the e2e ran the
game by hand while a real agent drove the bridge over the shared dir).

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
`wsl --shutdown`, or install the Windows depot (`4019831`) natively.
(A client **did** join on 2026-08-09 — see above — so this was overcome
at least once; the note stays for whoever hits it fresh.)

## Suggested next steps

1. **Finish the dwbridge command set.** The channel and `save` are done;
   what remains is `kick`/`ban`/`unban`/`broadcast` in the mod. These need
   a connected client to build against (the admin RPC wants a player
   controller; the struct params want a real value to copy), and a
   palagent launch profile that runs the Windows build under Wine so the
   agent supervises the modded process directly. Pin the UE4SS
   nightly that works; expect churn at 1.0.
2. **Deepen the save reader.** Now unblocked for real: a played world
   save exists locally, and player state turns out to be JSON embedded in
   SPUD properties — find the property values, `json.Unmarshal`, done.
   Keyed by char guid, not EOS id, so the roster still routes identity
   through dwlog/ini.
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
