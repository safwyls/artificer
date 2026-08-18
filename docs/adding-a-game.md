# Adding a game

This repo is a game-agnostic console framework (`core/`) plus one module
per game (`games/<game>/`) plus a thin binary each (`cmd/`). Three games
are in tree — Palworld, RuneScape Dragonwilds, Enshrouded — and this is
the checklist for a fourth.

It used to be a porting guide: how to lift the generic packages into a
sibling project for a second game. That is no longer the shape of the
problem. Everything it warned about — "still Palworld-shaped", the save
views hardcoded in the API, the agent that only knew how to launch
`PalServer.sh` — went away when the three consoles were unified onto
`core`, and each of those became a named seam instead.

## The dependency rule

```
core/api, collector, sched, watchdog, backup, notify, store, agent
        │  depend on
        ▼
core/game               ← contracts only: Client, Definition, ConfigCodec,
        ▲                  SaveLayout, the registry
        │  implemented by
games/<game>/           ← everything that knows the game
```

`scripts/checkbounds.sh` enforces it in CI, along with three more rules:
games never import each other, an agent never imports its console-side
game package, and production code never imports `core/game/gametest`.

If you find yourself wanting core to import your game, the thing you need
belongs on the Definition or behind an optional interface instead. Do not
fatten the *required* interface for one game's feature — that is the
failure mode this structure exists to avoid, and the rule that keeps it
true is: **core owns what `gametest` can exercise.** Core must build and
pass its whole suite with only the test game registered.

## What you have to write

### 1. A client — `game.Client`

Eight methods: `Info`, `Players`, `Broadcast`, `Kick`, `Ban`, `Unban`,
`Save`, `Shutdown`. Whatever transport the game gives you — REST, RCON, a
log tail plus a mod bridge, or the sidecar agent's own API when the game
has no admin surface at all.

A game that genuinely cannot do one of these returns
`*game.UnsupportedError` with a `Reason`, and the API answers **501** with
that reason rather than 502. The distinction matters: 501 says "this game
can't", 502 says "the server didn't answer", and an operator debugging the
second when it was the first wastes an afternoon. Write the reason to name
where the ability actually lives — "Enshrouded has no remote console; kick
from the in-game player list" beats "unsupported".

### 2. A definition — `game.Definition`

```go
var Definition = &game.Definition{
    ID: "yourgame", Name: "Your Game",
    DefaultGamePort: 1234,
    NewClient:       func(c game.Conn) game.Client { … },
    Features:        []string{game.FeatureMap, game.FeaturePlayers, …},
    Config:          &game.ConfigCodec{…},  // nil: no settings editor
    Save:            &game.SaveLayout{…},   // nil: the permissive default
}

func init() { game.Register(Definition) }
```

`Features` names *dashboard views*, not game concepts. A fourth game reuses
the ones that fit rather than inventing synonyms — ARK's tames are Pals,
its tribes are Guilds, its dino dex is Paldex — and renames them in its
frontend profile. That keeps one set of routes, one visibility schema, and
one set of stored switches. A view whose feature is absent is never
offered, which is how a game without a creature collection avoids shipping
an empty tab.

### 3. A config codec (optional)

`game.ConfigCodec` is the settings editor's seam. The parser is yours; the
*policy* is not, and it is the same for every game because every deviation
from it has cost someone a config:

- never add or remove keys the file didn't have,
- validate each new value against the existing one's type,
- keep one `.bak`,
- swap atomically.

If the game's server rewrites the file itself, say so in the codec's
comments and check whether it has a second writer — see "offline work"
below.

### 4. A save layout (optional)

`game.SaveLayout` tells the backup archiver what a save *is*. Match on
what the game actually writes, not on what looks like a save: Enshrouded's
are extensionless hex blobs, so an extension match would archive nothing.
If the game's files can be caught mid-write, give the layout a magic-bytes
check — archiving a torn save is worse than skipping a cycle, because it
looks like a backup.

### 5. An agent spec — `agent.Game`

`core/agent` is the whole sidecar; your game contributes a value. App id,
default port, config path, save-directory lookup, launch profiles, stop
signal and grace, the `PrepareRuntime` seed-and-enforce hook, and any extra
routes. See `docs/sidecar-agent.md` — in particular `PrepareRuntime`, which
is where "this game refuses to boot without X" and "the game's own default
config is an open server" get handled.

### 6. A provision profile (optional)

`api.ProvisionProfile` is what the Raise-a-server wizard and the Anvil
adapter read: image repo, env prefix, mount path, the game's port run, and
its named TCP admin transports. Three shapes exist already — a single UDP
port (Enshrouded), a contiguous UDP pair (Dragonwilds), a UDP port plus a
REST/RCON trio (Palworld) — and a fourth game most likely picks one rather
than adding a shape.

### 7. Contributed routes (optional)

A game with a surface core cannot know about mounts its own routes:
palworld's pals/guilds/inventory/storage/achievements pages, enshrouded's
A2S query, dragonwilds' bridge. Register them from `cmd/<console>/main.go`
via `apiServer.GameRoutes`; core stays out of it.

### 8. A frontend

`web/<console>/`, one React app per console, themed for its game. The
shared shell is real — app chrome, server rail, auth, the API client, the
UI kit, charts, power, settings, log dialogs, public status, users — and
the domain pages are meant to be *replaced* per game, not shared.

## What you get for free

| Concern | Package |
|---|---|
| Source RCON wire protocol, plus a fake server for tests | `core/rcon`, `core/rcon/rcontest` |
| Save parse caching, single-flight, stale-serve, settle window | `core/savecache` |
| SteamCMD update args and cache repair (the app id is a parameter) | `core/steamcmd` |
| Metrics sampling, retention, pruning, charts | `core/collector`, `core/store` |
| Player join/leave events, playtime sessions, last-seen | `core/collector` |
| Scheduled restarts with in-game warnings | `core/sched` |
| Crash watchdog | `core/watchdog` |
| Discord notifications | `core/notify` |
| Backup schedules and archives | `core/backup` |
| Docker power control | `core/dockerctl` |
| Container provisioning on the host | `anvil/`, `core/anvilclient` |
| The whole sidecar agent | `core/agent`, `core/agentctl` |
| Auth, users, permissions, audit trail, per-view visibility | `core/api`, `core/store` |
| Cloudflare Access SSO | `core/cfaccess` |

## Seams worth knowing before you need them

- **Offline config work.** Some games' settings files have two writers —
  the console and the running game, which rewrites the file on shutdown.
  Writing while the game is up loses the edit. `OfflineConfigWork` queues
  it and applies it during a restart, between the stop and the start.
  Enshrouded's ban list is the worked example.
- **Roster source.** `api.RosterSource` lets a game answer "who has ever
  played here" from a save file rather than from live queries.
- **Neutral query types.** `agentctl.Query*` carry presence data from
  whatever the game answers on, without core learning the protocol.
- **Console identity.** `game.DefaultID` and the session-cookie name are
  set in `cmd/<console>/main.go`; unlabelled rows resolve to the one
  registered game.

## Checklist

1. `games/<game>/` — a client implementing `game.Client`.
2. `var Definition = &game.Definition{…}` and `func init() { game.Register(Definition) }`.
3. `games/<game>/<name>agent/` — the `agent.Game` spec, plus `cmd/<name>agent/`.
4. Optional: config codec, save layout, provision profile, contributed
   routes, roster source.
5. `cmd/<console>/main.go` — the wiring, and `web/<console>/` — the app.
6. `deploy/<console>/` — a Dockerfile for the console and one per agent
   image, plus a `.github/workflows/docker-<console>.yml`.
7. A recon document at `games/<game>/docs/recon.md`, and this is not
   optional in spirit: every game in this repo turned out to differ from
   its documentation in ways that mattered. Write down what you *measured*
   against a real server, and mark what is still guessed.
8. Run `./scripts/checkbounds.sh` and `./scripts/checkdocs.sh`.

## The rule that keeps this honest

A game that cannot support a feature **answers with a reason** — a 501
naming where the ability actually lives — rather than hiding the feature.
Hiding it makes the console look like it never had the feature; saying so
makes it clear the game is the limit, and tells the operator what to do
instead.
