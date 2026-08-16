# State of play

Written 2026-08-15, at the end of the transplant that turned a copy of
wildskeeper into Flametender. Read this first, then
[`enshrouded-recon.md`](enshrouded-recon.md) — between them they hold
every fact the code rests on and every place it is still guessing.

## What this is

**Flametender** (module `github.com/safwyls/flametender`) is a management
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
   registration for flametender (`deploy/truenas-app.yaml` shows it).
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
port — and as of Phase 2 the console uses it (`esquery`, run by the
agent). It buys the real `slotCount`, the running build, and a player
count that owes nothing to log inference. It does *not* buy identity:
A2S rows carry a name and no account id, so the roster stays
log-derived. The table above gains one row — "how many are on right
now" comes from the game itself.

## Where things stand

`go test ./...` and `cd web && npm test` green; production build fine.

**Done:** the whole Phase 1 transplant — rename
(wildskeeper→flametender, wkagent→flameagent, WKAGENT_*→FLAMEAGENT_*),
the strips above, `internal/games/enshrouded` (definition + derived
client + honest 501s), `esconfig` (parse/edit/seed/enforce with
json.Number so int64 nanosecond durations survive), `eslog` (peer-based
join/leave tracker, versioned rules table), agent rework (Wine launch
profile, windows-depot SteamCMD, json config verbs with validation,
savegame/ bundle, SIGINT stop), Ilmari-only wizard, deploy files, docs,
and the Flametender frontend theme.

**Deployed and played on 2026-08-15.** A server was raised through the
wizard on the NAS, installed itself, booted under Wine, and a real client
joined and played on it. The recon doc's **verification ledger** now
records what that settled and what it didn't — read it before trusting
any remaining **[community]** marker.

What the first deployment proved, in the order the risks were ranked:

1. **The stdout ring does see the game log** — the top risk, closed. No
   logfile tail needed; Wine's own messages land in the same ring.
2. **Wine-from-Debian runs the server headless** on the flameagent image,
   building its prefix inside the install volume as configured.
3. **The seeded config is accepted and the role groups work** — a joining
   player's logged permissions are exactly the non-admin group's.
4. **The log vocabulary was wrong**, and this was the one real bug: the
   community-sourced lines (`Added Peer #0.`) bear no resemblance to what
   a current build emits (`Added peer 0(1) (steamid:…)`), so the roster
   read empty while someone was playing. `eslog` RulesV2 is now written
   from our own capture — and the capture also showed that **player names
   are in the log**, which the community sources denied.

Still unproven: the SIGINT stop's clean save-on-shutdown, the ban list's
element format, whether the game honours a config-written ban at all,
and the save's rolling-copy rotation. All are ledger rows. (A2S from
off-host was one too; running the query on the agent retired it as a
blocker.)

### Since then (2026-08-16)

- **Cloudflare Access SSO** — `internal/cfaccess` verifies the tunnel's
  assertion, `api.handleCloudflareLogin` creates accounts on first
  sign-in with no permissions, `CF_ACCESS_ADMIN_EMAILS` is the lockout
  rescue. Password login stays; `docs/cloudflare-access.md` is the
  contract, including why the audience check is mandatory.
- **PWA install** — manifest plus a service worker with a real fetch
  handler (network-first shell, cache-first assets, never `/api/` or
  `/cdn-cgi/`), so the browser offers the install prompt.
- **Launch mode removed.** Enshrouded ships one server build, so a
  chooser offered a choice that does not exist. The agent still assembles
  the one profile plus the custom escape hatch; `GET /launch` survives
  read-only, because "can this agent's image actually run the game?" is
  still worth asking. What the card became is **Rebuild agent**, in the
  SteamCMD maintenance strip.
- **Phase 2's moderation surface** — the ban list and the role-group
  editor, both landed. See the roadmap's Phase 2 items 3 and 4 for what
  shipped and what deliberately didn't. Two design notes worth carrying:
  roles sit behind `PermSettings` because a group carries its join
  password in the clear, while bans sit behind `PermModerate` because
  they carry no credential; and the bans editor never imposes an element
  format on the file, which is why that ledger row stopped gating
  anything.
- **Phase 2 is complete.** The Steam query landed last (`esquery` +
  `GET /v1/query` on the agent + the console wiring), and with it the
  three facts the log could never carry: how many people are on right
  now, the server's real `slotCount`, and the build it is running. The
  design decision worth keeping is the lane split — the query owns the
  *present*, `eslog` owns *identity and history*. They are not
  interchangeable and neither is a fallback for the other in general: a
  silent query falls back to the log-derived count, but A2S can never
  produce a roster row, because it carries names and no account ids.
- **The phase gate was dissolved, not answered.** "Does A2S work from
  off-host" blocked this phase on the recon ledger. Running the query
  agent-side against `127.0.0.1` made the question irrelevant — and it
  also honours the standing constraint that the agent is the only
  transport. If a future need really is off-host, the ledger row is
  still there.
- **Readiness has three values, and that is deliberate.** A booting
  server reads "running" minutes before it accepts joins, so the Overview
  now shows Starting / Online / Offline off eslog's `HostOnline` marker.
  But that marker is logged once at boot and the agent's ring holds ~80
  minutes, so a console that starts watching a long-running server never
  sees it. Absence past a 15-minute window therefore means *unknown*, not
  "still starting" — and the query answering is not accepted as proof of
  readiness either, since the game and the query share one port and a
  reply only proves the socket is up.
- **The signature element had been lying.** `FlameSigil` draws
  lit-of-total against what was the game's 16-slot hard cap, so a full
  4-slot server rendered as one-quarter lit. It now draws against the
  queried slot count. Worth remembering as the pattern: the number that
  is easiest to hardcode is the one most visible at a glance.

- **The ban list has two writers, and the game wins.** Same day, the
  first real use found it: a ban added while the server was up was gone
  after the restart. `bannedAccounts` is also maintained by the in-game
  kick/ban UI, and the game holds it in memory and writes it back out on
  shutdown — so anything the console writes mid-session is erased on the
  way down. The console stopped competing for the file. An edit made
  while the game is up is recorded as intent (`pending_bans`) and written
  by `internal/banqueue` during the next restart, between the stop and
  the start; both restart paths the console drives (the power handlers
  and the scheduler) run the two halves themselves when work is queued.
  A queued row survives until the *file* agrees with it while the game is
  running — which is also the diagnosis for the one case still open: if
  a change written to a stopped server's config is missing once the game
  comes up, the game keeps its bans somewhere else entirely, and the
  panel says so instead of losing the ban a second time in silence.

## Running it locally

There is no local Enshrouded dev loop on this box yet (the game needs
Wine and a ~5 GB Windows depot). `scripts/dev-local.sh` is the sibling
consoles' pattern and needs its Enshrouded pass — treat it as unported
until the first real deployment; the fake-driven test suite is the dev
loop meanwhile.

## Suggested next steps

1. **Finish the ledger**: the open rows are a clean stop (does SIGINT
   reach the exe through Wine and save?), A2S reachability, whether a
   running server overwrites a hand-edited ban list, and the save's
   rolling-copy rotation after a few days. Each one gates or softens
   something already shipped.
2. **The rest of Phase 2** (`docs/roadmap.md`): the moderation surface is
   done, so what's left is the A2S client and the ready-state signal.
   A2S is worth less than originally planned — names come from the log —
   but still buys authoritative presence, the real `slotCount`, and a
   roster that survives a console restart mid-session.
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
