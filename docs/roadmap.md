# Flametender roadmap

The plan is deliberately shaped like the way this console will actually be
used: **Phase 1 gets a private server online so play can start**, and
every later phase is something that can land incrementally while the
server is live, roughly in the order the pain will be felt. Each phase
names its gate — the thing that must be true before the phase is worth
starting — because half of this game's surface is still
community-sourced (docs/enshrouded-recon.md) and some phases hang on a
fact the first real deployment has to confirm.

## Phase 0 — recon (done)

`docs/enshrouded-recon.md`: app id, config schema, ports, logs, saves,
shutdown semantics, and the confidence marker on every fact. Its
**verification ledger** is the working checklist for the first
deployment; a recon fact that fails moves code, not just the doc.

## Phase 1 — bare-minimum hosting (this branch)

**Goal: raise a private Enshrouded server from the dashboard and play on
it.** Everything here is inherited machinery pointed at the new game plus
the game-specific packages.

- Console: auth, users, server rows, metrics history, activity, audit,
  Discord notices, scheduled restarts, crash watchdog, backups — all
  inherited, game-blind, and kept.
- `internal/games/enshrouded`: the game definition, the derived client
  (liveness from flameagent health, players from the log tail via
  `eslog`), honest 501s for every command with reasons that say where the
  ability actually lives (kick/ban are in-game; saves are automatic).
- `esconfig`: read/edit/seed/enforce `enshrouded_server.json` — including
  seeding *before first boot* so a provisioned server is never the
  game's own open-by-default config, and role-password enforcement by
  capability.
- flameagent: Wine launch profile (single build), SteamCMD windows-depot
  install, SIGINT-then-KILL stop with a save-honoring grace, json config
  verbs with validation, save bundle over the `savegame/` layout.
- Provisioning: **Ilmari only.** The Raise-a-server wizard speaks to the
  shared host service; this console ships no provisioner mode and holds
  no Docker rights (the one wrinkle: register flametender as an Ilmari
  client — see `deploy/`).
- Frontend: the Flametender theme (docs/design.md), Overview / Flameborn
  (players) / Configuration / World saves / Server log pages.

**Exit criterion:** the recon ledger's Phase-1 rows are checked against a
real server raised through the wizard, and a real client has joined and
played. **Met 2026-08-15**, with one bug found and fixed by it: the
log-line vocabulary was community-sourced and wrong, so the roster read
empty on a live server (`eslog` RulesV2 is written from our own capture
now). The same capture showed player names *are* in the log — see the
Phase 2 re-scope below.

## Phase 2 — live presence and moderation surface

**Gate: a live server (Phase 1 deployed); the A2S off-host answer in the
ledger.**

1. **A2S query client** (`esquery`, pure Go, ~200 lines: A2S_INFO +
   A2S_PLAYER with the challenge handshake). **Re-scoped 2026-08-15**:
   names now come from the log, so this no longer gates the roster's
   readability, and it drops below the moderation items in value. What it
   still buys: authoritative presence ("right now", independent of log
   inference), the real configured `slotCount` for charts (Metrics
   reports the 16-slot cap today), and a roster that survives a console
   restart later than the agent's ~80-minute log ring. Console-side,
   querying `host:gamePort` directly; falls back to log-derived when the
   port isn't reachable. Keep both honest: the log tracker owns join and
   leave *history*, A2S owns the present.
2. **Ready state**: surface eslog's `HostOnline` in the Overview
   ("starting" vs "accepting players"), since a booting server binds its
   port well before it accepts joins.
3. ~~**Ban list editor**~~ — **done 2026-08-16.** `bannedAccounts` is a
   first-class list (`esconfig/bans.go`, `GET/PUT /bans` behind
   `PermModerate`, the Bans panel on Flameborn), and the roster's Ban
   button is live as **Add to ban list**. The ledger row on the element
   format was never gating after all: the editor reads whichever of the
   two plausible shapes the file uses, writes new entries in that same
   shape, and preserves entries it can't model instead of dropping them.
   The running server owning the file too turned out to be more than a
   caveat: first real use showed a mid-session ban erased at the next
   stop. So edits made while the game is up are queued and written during
   the restart (`internal/banqueue`), not raced into the file — see
   `docs/state-of-play.md` and the recon ledger.
4. ~~**Role-group editor**~~ — **done 2026-08-16.** userGroups CRUD in
   `esconfig/roles.go` (`GET/PUT /config/roles` behind `PermSettings`,
   since each group carries its join password in the clear) and the Role
   groups panel on Configuration. Validation refuses the three
   self-inflicted wounds: no kick/ban group at all, an admin group with
   an empty password, two groups sharing one password. Per-group password
   *rotation* did not ship — the fields are editable in place, and the
   existing admin rotate covers the one credential worth generating.
5. **Version surfacing**: the log's build hash + Steam build id from the
   update job, so "a game update dropped" is visible before friends hit
   the version-mismatch join error.

## Phase 3 — saves, rollback, and world lifecycle

**Gate: the ledger's save-layout row; a few days of real autosave rotation
to test against.**

1. **Save index reader** (`essave`, a `savecache.Source`): parse
   `<hex>-index` (the `latest` pointer and save time) and the `-info`
   sidecar (world name) — metadata only, the world blob has no public
   parser. Wire the `/world` endpoint back up (it was dropped in the
   transplant) and give the World-saves page real facts: which copy is
   live, when it was written, how deep the rollback window goes.
2. **Rollback**: restore a rolling copy or a Flametender backup — stop,
   swap the pointed copy in (mornedhels' approach: place the copy, write
   a fresh index with `latest: 0`), start. The UI must say what will be
   lost (up to 10 minutes since the chosen copy).
3. **World import/export**: singleplayer → server migration (upload a
   world file over the hex slot + fix the index) and the reverse, since
   the community does this by hand today.
4. **Pre-update/pre-restart snapshot**: the scheduler and the Steam
   update flow take a backup first; Enshrouded saves on shutdown so this
   is cheap insurance, not a correctness need.

## Phase 4 — running-it-for-months quality

No gate; each item independent.

1. **Update watcher**: poll the Steam app build id; when a game update
   ships, notify (Discord) and offer/schedule stop → update → start.
   Version-gated joins make stale servers unjoinable, so this is the
   feature that keeps the server *playable*, not just up.
2. **Scheduler honesty pass**: restart warnings cannot reach players
   in-game (no broadcast channel exists) — make the Discord notice the
   first-class warning path and say so in the UI copy.
3. **Log deep links**: the Activity page's join/leave history enriched
   with A2S-resolved names, Steam profile links off SteamID64s.
4. **Resource telemetry**: the game's RAM appetite grows with terrain
   edits; surface container memory (via Ilmari's fleet view or agent-side
   sampling) with the 16 GB context so "the server is getting heavy" is
   visible before the OOM.

## Phase 5 — the 1.0 wave (2026-10-15)

Enshrouded 1.0 lands PC + PS5 with crossplay planned. Assume churn:
config schema, networking, possibly the log vocabulary (eslog rules are
versioned tables for exactly this), possibly a real query/admin surface
(watch for it — a first-party API would obsolete chunks of Phase 2 and
should win if it appears). Budget a recon refresh against the 1.0 server
before touching code, same drill as Phase 0.

## Standing constraints

- The agent remains the only transport; nothing may bypass it
  (docs/architecture.md).
- Ilmari owns container placement; this console must never grow Docker
  rights back. "Adding a third console needs no code in Ilmari" is a
  promise this repo relies on — keep it true from this side too. The
  deploy stacks therefore carry no docker socket at all (the sibling
  consoles' scoped socket proxy was dropped, 2026-08-15): every power
  path tries the flameagent first and an agent-supervised server never
  reaches the docker branch. `DOCKER_HOST` stays supported in code for
  an agentless server — a shape this console does not create — and if
  that ever becomes common, the honest fix is teaching the watchdog to
  work through the agent, not restoring the proxy by default.
- Every capability claim in the UI must be probe-derived or honestly
  501-reasoned; a button that cannot work is a bug even when it renders.
