# State of play

Written 2026-08-15, at the end of the transplant that turned a copy of
wildskeeper into Flamekeeper. Read this first, then
[`enshrouded-recon.md`](enshrouded-recon.md) — between them they hold
every fact the code rests on and every place it is still guessing.

## What this is

**Flamekeeper** (module `github.com/safwyls/flamekeeper`) is a management
console for self-hosted **Enshrouded** dedicated servers. It was built by
copying [wildskeeper](https://github.com/safwyls/wildskeeper) — itself
copied from [palcon](https://github.com/safwyls/palcon) — and swapping
the game, exactly the move `docs/porting-to-another-game.md` describes.
The architecture is the siblings', kept structurally identical so fixes
can travel between the three.

Two deliberate departures from the wildskeeper starting point:

1. **Provisioning is Ilmari-only.** The built-in provisioner mode
   (agent-side Docker rights, `PROVISIONER_URL`, the game-agnostic spec
   endpoint) was deleted, not carried: Ilmari is deployed and proven on
   the target host, and a second Docker-socket holder is exactly the
   blindness Ilmari exists to end. `api.IlmariProvisioner` holds all the
   game-shaped provisioning knowledge; Ilmari needs only a client
   registration for flamekeeper (`deploy/truenas-app.yaml` shows it).
2. **Inherited dead weight went with it**: the orphaned pal advisor
   (~1,100 lines, UI already deleted upstream), the inert `internal/rcon`
   pair (Enshrouded has no RCON and never had), the dwbridge/UE4SS
   command channel (no injection surface exists on Enshrouded's
   proprietary engine), and the dwsave/world endpoint (Enshrouded's world
   blob has no public parser; the Phase 3 replacement reads the save
   *index* instead).

## The one fact that shapes everything

Enshrouded has **no RCON, no admin API, no server console**. Same class
of game as Dragonwilds, so the same architecture answer: the flameagent
sidecar is the only transport, and every piece of live state is derived —

| What the UI shows | Where it actually comes from |
|---|---|
| Server up/down, uptime | flameagent `/v1/health` → supervised process state |
| Player list | a state machine (`eslog`) over the agent's stdout log ring |
| Config | `enshrouded_server.json` read at rest (`esconfig`) |
| Saves | files on disk (`savegame/`), synced through the agent |

Two Enshrouded-specific differences from the Dragonwilds situation, both
load-bearing:

- **The game saves on shutdown** (SIGINT = clean save), so a power stop
  costs nothing — the opposite of Dragonwilds, where a restart could
  lose 5 minutes. The supervisor sends SIGINT (not SIGTERM: the Wine
  wrapper doesn't reliably propagate it) with a generous 120 s grace.
- **The game's own first-boot config is an open server.** So flameagent
  seeds a complete `enshrouded_server.json` before first start — name,
  port, an Admins group and a Friends group with the configured
  passwords — and enforces those identity settings on every start,
  matching groups by capability (`canKickBan`) rather than by name.

There IS one native query surface — Steam A2S on the game's single UDP
port — deliberately deferred to Phase 2 (`docs/roadmap.md`), where it
buys player *names* (logs only carry SteamID64s), the real slotCount,
and log-independent liveness.

## Where things stand

`go test ./...` and `cd web && npm test` green; production build fine.

**Done on this branch:** the whole Phase 1 transplant — rename
(wildskeeper→flamekeeper, wkagent→flameagent, WKAGENT_*→FLAMEAGENT_*),
the strips above, `internal/games/enshrouded` (definition + derived
client + honest 501s), `esconfig` (parse/edit/seed/enforce with
json.Number so int64 nanosecond durations survive), `eslog` (peer-based
join/leave tracker, versioned rules table), agent rework (Wine launch
profile, windows-depot SteamCMD, json config verbs with validation,
savegame/ bundle, SIGINT stop), Ilmari-only wizard, deploy files, docs,
and the Flamekeeper frontend theme.

**Verified against a real server: NOTHING YET.** This is the most
important sentence in the file. Every game-facing fact is
community-sourced (the recon doc's markers say which kind), and the recon
doc's **verification ledger** is the first deployment's checklist. The
riskiest assumptions, in order:

1. **Does the supervisor's stdout ring see the game log at all under
   Wine?** The exe writes `logs/enshrouded_server.log`; jsknnr symlinks
   it to stdout, which suggests the exe itself may write little or
   nothing to stdout. If the ring stays empty, the fix is small and
   contained — the agent tails the log file into the same ring — but
   until then the player list and eslog derive from an unproven source.
2. **Wine-from-Debian actually runs the server headless** on the
   flameagent image (guides prove wine64 works; this image's exact
   package set is unproven — expect the first container boot to find a
   missing dependency or a prefix quirk).
3. **The log line vocabulary** (eslog RulesV1) is written from community
   captures of 2024–2025 builds, not our own capture on v0.9.1.x.
4. **The seeded config is accepted** and the role passwords work at the
   join screen.

## Running it locally

There is no local Enshrouded dev loop on this box yet (the game needs
Wine and a ~5 GB Windows depot). `scripts/dev-local.sh` is the sibling
consoles' pattern and needs its Enshrouded pass — treat it as unported
until the first real deployment; the fake-driven test suite is the dev
loop meanwhile.

## Suggested next steps

1. **Deploy for real** (the Phase 1 exit): build the two images, register
   flamekeeper as an Ilmari client on the NAS, raise a server through
   the wizard, join it, and work through the recon doc's verification
   ledger — checking rows off with dates, and moving code where a fact
   fails. Budget for the stdout-vs-logfile finding (risk #1 above).
2. **Then Phase 2** (`docs/roadmap.md`): the A2S client is the highest
   value-per-line item in the plan — names, real slotCount, and honest
   liveness for a couple hundred lines of pure Go.
3. Keep `docs/porting-to-another-game.md` honest if the seams move: it
   is the document the *fourth* console will be built from.

## Loose ends

- `internal/savecache` is currently importer-less (the dwsave world
  endpoint went with the transplant). Kept deliberately: it is generic,
  tested, and Phase 3's save-index reader is its next consumer.
- `scripts/dev-local.sh` unported (above).
- `docs/architecture.md`, `docs/sidecar-agent.md`, `docs/visibility.md`
  and `docs/porting-to-another-game.md` are inherited: structurally
  accurate, but their worked examples still speak Palworld/Dragonwilds.
  Rewrite opportunistically, not urgently.
- The store's SQL migrations are inherited wholesale (including tables
  only the deleted advisor used). Harmless; a fresh DB just carries a
  few empty tables.
