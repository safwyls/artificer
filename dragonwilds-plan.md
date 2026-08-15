# Plan of Attack: Dragonwilds Support in Palcon ("Flamekeeper")

> **Status 2026-08-09 — Phases 0, 1 and most of 5 landed**, as this
> standalone repo (flamekeeper/Flamekeeper) built from palcon's reusable base —
> superseding §0's in-binary decision, per the maintainer: flamekeeper is a
> sibling project, the porting doc's recommended path. Palworld itself was
> removed; the shared-layer tests run against a test-only REST game
> (`internal/game/gametest`). Recon findings live in
> `docs/dragonwilds-recon.md`; where they contradict this plan, that
> document wins. Corrections to facts asserted below:
>
> - Server app id is **4019830**; a **native Linux server build ships**
>   (`RSDragonwildsServer.sh`), so Win64/Proton is only a Phase 4
>   consideration (UE4SS is Win64-only in practice).
> - The config key is **`OwnerId`**, not `OwnerID`; there are **no
>   `MaxPlayers` or `Port` ini keys** (port is the `-Port=` launch arg; cap
>   is a flat 6). Section is `[/Script/Dominion.DedicatedServerSettings]`.
> - Logs: `Saved/Logs/RSDragonwilds.log`; verified markers are
>   `LogNet: Join succeeded:` and
>   `LogDominionPlayerController: ClientRequestDisconnect`. Full line
>   shapes, autosave and chat formats remain unverified — dwlog ships a
>   versioned v0 table pending a real corpus.
> - Save dir casing varies (`SaveGames`/`Savegames`) — detect, don't
>   hardcode. Save-on-SIGTERM is **unverified**; ban list location on disk
>   was **not found** (ban/unban stays in-game-only for now).
> - Player-id wire format is undocumented; `CanonicalUID` is trim-only
>   until real ids are captured.
> - §1's premises that palcon already had per-method capability
>   degradation and per-game frontend page replacement were wrong — both
>   were built as shared-layer work (`game.UnsupportedError` → HTTP 501).
>   Next free migration is 0022, and build/test commands live in README.md.
> - v1 feature set is pals/saves/logs (new canonical `saves` and `logs`
>   view keys). A Bans page is deferred until a ban data source exists —
>   a ledger with nothing real behind it would violate the mock's own
>   capability-honesty rule.
>
> **Phase 2 is also done** (2026-08-09): the agent launches
> `./RSDragonwildsServer.sh -log -Port=`, seeds `DedicatedServer.ini` with
> a required Owner ID, and provisioning stands a server up end to end —
> Dragonwilds container template (7777/7778 udp pair, `/dragonwilds` bind,
> no REST/RCON), generated compose stack, and a Raise-a-server wizard.
>
> **The parse probe is done** (2026-08-09, on a real server): the save is
> the SPUD plugin's chunked `SAVE` format, **not GVAS** — so §5's plan to
> use `uesave` or a Python `gvas` library is void, but the news is good:
> SPUD is open source and world metadata is readable in the header, so a
> Go-native reader with no Python dependency is realistic. Also measured:
> the server does not save on shutdown, and `OwnerId` is not format-
> validated. See the recon doc's "Empirical findings".
>
> Remaining: Phase 3 (save reader, now unblocked), Phase 4 (dwbridge), and
> four gaps that need real game clients — join/leave log lines, a real
> player id, the ban list at rest, and the autosave trigger.

**Audience:** Claude Code, working in this repo (originally written for `safwyls/palcon`).
**Goal:** A fully featured management console for a self-hosted RuneScape: Dragonwilds
dedicated server (moddable, 6-player, friends-only), reusing palcon's game-agnostic
core and flameagent architecture. The visual target is the Flamekeeper mock
(`dragonwilds-dashboard.html`), which defines the theme, layout, and the honest
capability set of the game.

---

## 0. Architectural decision (read first)

`docs/porting-to-another-game.md` says the *expected* reuse path is a sibling
project that copies the generic packages, because Go's `internal/` rule blocks
external imports. That advice targets third parties. We are the maintainer, and
the repo already contains the in-binary path end to end: the `game.Definition`
registry, `internal/games/games.go`, feature-key gating, and migration
`0019_game.sql` (a `game` column on servers defaulting to `palworld`).

**Decision: implement Dragonwilds as a second game inside the palcon binary**,
following the porting doc's checklist. Rationale: one deploy on Messier, shared
auth/users/audit/backups/notify/agent with zero package extraction, and the
frontend shell is already designed for per-game page replacement. Honor the
doc's constraint: do **not** grow `game.Definition` with fields nothing reads.
If Dragonwilds needs something the contract lacks, first check whether it can
live entirely inside `internal/games/dragonwilds/` or the agent.

## 1. Hard constraints from the game (verified, Aug 2026)

These are facts, not assumptions. They shape every phase.

- Dedicated servers exist since game update 0.11. Max 6 players. Sizing rule of
  thumb: 2 GB base + 1 GB per player. Epic Online Services auth (Steam players
  authenticate via Epic).
- **No RCON. No REST. No query protocol. No native console.** All native admin
  is the in-game Server Management menu (Owner/Admin/Standard roles). Owner =
  Player ID matching `OwnerID` in config; Admins are password-session based;
  Owner can ban/unban anyone incl. offline, Admins can only ban online players
  and cannot unban.
- Config: `RSDragonwilds/Saved/Config/<Platform>Server/DedicatedServer.ini`
  (ServerName, DefaultWorldName, OwnerID, AdminPassword, MaxPlayers, Port,
  etc.). Server will not start without OwnerID. UDP game port.
- Saves: `RSDragonwilds/Saved/Savegames/*.sav` (UE5 / GVAS family, format
  unverified). Server loads the latest .sav on start; empty folder = new world
  from config values. No cloud sync. Logs under `RSDragonwilds/Saved/Logs/`.
- Modding: only on **moddable** servers (vanilla/"secure" servers reject mods)
  — we run moddable, so this is fine. UE4SS is the framework; it provides a
  Lua scripting console (F10 client-side). Known churn: after game update 0.12
  the default `dwmapi.dll` proxy stopped loading UE4SS server-side and the
  community moved to a `version.dll` proxy. Expect this class of breakage every
  major patch. UE4SS server installs target the **Win64** server build; native
  Linux UE4SS ports exist but are Palworld-only and untested on other games.

Consequence: the Palworld client's transport stack (REST + RCON fallback) has
no equivalent. Dragonwilds state is **derived** (logs, config, process, saves)
and commands come only from a UE4SS bridge we write ourselves (Phase 4).

## 2. Phase 0 — Recon (no production code)

Do all of this before writing the game package. Produce
`docs/dragonwilds-recon.md` capturing findings; later phases cite it.

1. Read in full: `docs/sidecar-agent.md`, `docs/porting-to-another-game.md`,
   `internal/game/*`, `internal/games/palworld/*` (the template),
   `internal/flameagent/*`, `internal/agentctl/*`, `internal/savecache/*`,
   `internal/collector/*` (how Players polling feeds join/leave events and
   playtime), and how `internal/api` surfaces per-method client errors.
2. Verify externally (do not guess; record sources):
   - The Steam app ID of the Dragonwilds *dedicated server* app for
     `internal/steamcmd` (app id is already a parameter there).
   - Whether a native Linux server build ships, or Win64-only. Config paths
     seen in the wild reference both `LinuxServer` and Win64 binaries; resolve
     which we run. Decision input for Phase 2.
   - Shutdown semantics: does the server save on SIGTERM, and how long does a
     clean stop take? (Determines whether `Client.Shutdown` is safe without the
     bridge, and the watchdog's stop timeout.)
   - Where the ban list persists (config vs save vs separate file) and whether
     it is editable at rest. If yes, offline ban/unban may be implementable
     *without* the bridge via agent file edits + restart.
3. Empirical log corpus: stand up the server once (throwaway world), connect
   two clients, and capture logs covering: startup, world load, player join,
   player leave, autosave, chat (if logged), clean shutdown, crash. Commit
   sanitized samples to `internal/games/dragonwilds/testdata/logs/`. Every
   regex in Phase 1 must be justified by a line in this corpus — no invented
   log formats.
4. Save format probe: copy one .sav, attempt parse with `uesave` (Rust) and a
   Python `gvas` library. Record: header magic, compression (zlib? oodle? —
   palcon already vendors `pyooz` if oodle), whether standard GVAS property
   walking succeeds. This decides Phase 3's approach and effort.
5. Audit flameagent's file/process/SteamCMD half vs. what Dragonwilds needs:
   log streaming or tailing to base (exists? add?), file read/write for ini
   and saves, process supervision hooks. List the deltas.

**Gate:** Phases 1–4 below assume recon answers. Where recon contradicts this
plan, recon wins — update this file in the same PR.

## 3. Phase 1 — Game package: definition, client, config

Target: a Dragonwilds server appears in the dashboard with live status,
players, playtime, metrics, backups, scheduled restarts, watchdog, Discord
notifications — everything the porting doc's "free" table promises — with
commands honestly disabled.

1. `internal/games/dragonwilds/`:
   - `Definition` with `ID: "dragonwilds"`, correct `DefaultGamePort`,
     `NewClient`, `CanonicalUID`, and a **minimal** feature list (see Phase 5).
     Register via `init()` + one line in `internal/games/games.go`.
   - `CanonicalUID`: normalize the Player ID across its three spellings
     (in-game Settings display, log lines, save file). The porting doc's
     warning applies verbatim: a mismatch fails *open* on visibility checks.
     Write table-driven tests from real IDs gathered in recon.
2. `Client` implementing the 8-method `game.Client` contract with derived
   backends:
   - `Info`: process/agent liveness + config (name, world, port) + world-day
     if cheaply derivable (else omit until Phase 3).
   - `Players`: from the log-derived session tracker (below). This feeds
     `internal/collector`'s join/leave events and playtime for free.
   - `Save`, `Shutdown`: per recon findings — signal-based if SIGTERM saves,
     else bridge-only.
   - `Broadcast`, `Kick`, `Ban`, `Unban`: return a typed "unsupported without
     command bridge" error in Phase 1. Confirm how `internal/api` and the
     frontend render per-method failures; if they don't degrade gracefully,
     fix that in the shared layer (it benefits every future game).
3. Log-derived session tracker (`internal/games/dragonwilds/dwlog/`):
   a small state machine over the agent-streamed log tail — join/leave/save/
   event lines per the recon corpus. Rules: state resets on server restart
   (never accumulate across process lifetimes), unknown lines are ignored not
   fatal, and parsers are versioned so a patch that changes log format breaks
   one table of regexes, not the package. Test against the committed corpus.
4. `dwconfig/`: `DedicatedServer.ini` parse + edit. Copy `palconfig`'s
   *policy* exactly (the porting doc calls it out as the reusable part): never
   add or remove keys, validate new values against existing types, keep one
   `.bak`, atomic swap. Add a dedicated "rotate AdminPassword" operation —
   it is the one real remote admin lever the game gives us (revokes all admin
   sessions) and the mock exposes it as a first-class button.
5. Migration/store: no schema work expected — `0019_game.sql` already exists.
   Verify server-create flow accepts `game='dragonwilds'`.

**Acceptance:** add a Dragonwilds server in the UI; see online status, live
player list from logs, playtime accruing, metrics charts, working backups and
scheduled restarts; Kick/Ban buttons visibly disabled with an accurate reason.

## 4. Phase 2 — flameagent: supervisor mode for Dragonwilds

The porting doc is explicit: flameagent's file/process/SteamCMD half is generic;
the *launch* half is Palworld-shaped (`PalServer.sh`, `PalWorldSettings.ini`).
Add a Dragonwilds launch profile.

1. Refactor the launch half behind a small per-game launcher interface inside
   `internal/flameagent` (keep it internal to the agent; do not grow
   `game.Definition` for this unless palcon-base actually needs to read it).
2. Dragonwilds launcher: SteamCMD install/update with the verified app id;
   start command per recon (native Linux binary, or Win64 under Proton-GE —
   see decision below); health probe = process + log heartbeat (no query
   protocol to ping); stop = the clean-shutdown sequence recon established.
3. **Runtime decision** (recon-gated): if the moddable/UE4SS path requires the
   Win64 server build, run Win64 under Proton-GE inside the agent container
   from day one, so Phase 4 is a mod install, not a replatform. If a native
   Linux build exists and recon shows UE4SS-Linux viability, prefer native and
   treat Proton as the Phase 4 fallback. Either way the compose file targets
   TrueNAS SCALE on Messier, and preserves palcon's rule: **no Docker socket**
   — power control stays behind the scoped proxy / agent supervisor.
4. Agent file services: expose `Saved/Config`, `Saved/Savegames`, `Saved/Logs`
   through the existing agent file cache (`internal/agentfiles`), with
   **saves mounted/served read-only into palcon-base** — same invariant as
   Palworld ("no code path that writes a save"). Save *restore* is an agent-
   side copy into `Savegames/` performed only while the server is stopped,
   with the displaced save archived first.
5. Log streaming: whatever recon found missing (tail-follow with rotation
   handling, backpressure) gets added to the generic half, since the log
   stream is Dragonwilds' primary state source.

**Acceptance:** one-click provision of a Dragonwilds server via flameagent
supervisor mode on Messier; update via SteamCMD with live transcript; crash
watchdog restarts it; ZFS-friendly backup schedule captures `Savegames/`.

## 5. Phase 3 — Save reader (recon-gated scope)

Only proceed on a positive Phase 0 parse probe. Implement
`savecache.Source[T]` (`Locate` = newest .sav in `Savegames/`; `Parse` per
probe results — Go-native via GVAS walking if standard, else shell out like
`palsave` does). savecache's mtime caching, single-flight, stale-serve, and
settle window come free.

Scope discipline: this is a 6-player friends server, not public hosting.
Extract in order of value, stopping when effort spikes: (1) per-character
name/ID/levels (Rune & Combat) and last-known position; (2) world state — day
count, active events if present; (3) structure/base inventory search *only if*
the property layout makes it cheap — palcon's storage search is its killer
feature, but do not reverse-engineer opaque blobs for it in v1. Serve results
from new endpoints alongside `internal/api/pals.go` per the porting doc's
guidance (domain endpoints per game; move route registration behind the
definition when the second consumer — this — exists).

## 6. Phase 4 — Command bridge (the UE4SS mod)

This is the RCON substitute and the highest-risk component. Build it late,
behind capability detection, so the console is fully useful without it.

1. `mods/dwbridge/` (Lua, UE4SS): opens a **localhost-only** TCP listener with
   a shared-token handshake and a line-delimited JSON protocol. v1 verbs:
   `broadcast`, `kick <uid>`, `save`, `shutdown <seconds> <message>`, `ping`.
   Nothing else until these are stable across one game patch.
2. Go side: `internal/games/dragonwilds/bridge.go` — a thin vocabulary client
   mirroring how `palworld/rcon.go` is "nothing but vocabulary." On connect
   success, the Client upgrades: Broadcast/Kick/Save/Shutdown route through
   the bridge; on failure it degrades to Phase 1 behavior. Scheduled-restart
   in-game warnings (`internal/sched`) light up automatically once Broadcast
   works.
3. Ban/Unban: prefer the at-rest ban-list edit from recon (agent file edit +
   restart, Owner-semantics) over bridge implementation if viable — file edits
   survive patches; UE4SS hooks don't.
4. Patch-resilience posture, learned from the 0.12 proxy break: pin the UE4SS
   release and proxy DLL per game version in the agent's launch profile;
   bridge failure must never block server launch; surface "bridge down —
   command tier disabled" in the UI exactly like the mock's log-panel note.
5. Test with a fake bridge server (the `rcontest` pattern) so CI never needs
   the game.

## 7. Phase 5 — Frontend: Flamekeeper

The shared frontend is the shell (AppShell, ServerRail, auth, MetricChart,
ServerPower, ServerSettings, log dialogs, Users, PublicStatus); Palworld's
pages are replaced per game. Use `dragonwilds-dashboard.html` as the design
spec, not code to transplant:

- Tokens: bg `#10141a`, panel `#1b2330`, brass `#8a6f3a`/`#c9a24b`, rune cyan
  `#52d8d0` (reserved for live/active state), ember `#e0704a`, parchment
  `#e6dcc4`. Type: Cinzel (display) / Alegreya Sans (body) / JetBrains Mono
  (logs, config). Signature element: the six-segment rune sigil where lit
  segments = occupied player slots (the game's hard cap) and the pulsing
  dragon-eye = online state. Respect `prefers-reduced-motion`.
- `GameProfile` in `web/src/lib/games.ts` with an honest, *small* feature
  list. Reuse existing feature keys as views per the porting doc (players/
  map-analog etc.); do not invent synonym keys. No Paldex-equivalent in v1.
- Pages: Overview (sigil, vitals incl. the 2 GB + 1 GB×player memory hint,
  event strip), Adventurers (roles Owner/Admin/Standard, kick/ban gated on
  bridge/at-rest capability, offline-ban rules matching the game's Owner-only
  semantics), World Saves (list/download/restore-while-stopped), Configuration
  (ini editor + rotate-admin-password), Logs (live tail), Bans.
- Copy tone follows the mock: plain verbs, capability-honest ("No native
  console — command tier requires the dwbridge mod").

## 8. Cross-cutting rules

- Preserve palcon's three inviolables: saves structurally read-only in base,
  no Docker socket, data stays home.
- Every parser (logs, ini, save, bridge protocol) ships with fixtures from
  real captures and a "game patched, format drifted" failure mode that
  degrades a feature rather than the server view.
- Small PRs per phase step; `go test ./... -coverpkg=./...` and `web` vitest
  green before merge; update `CLAUDE.md` with Dragonwilds-specific commands
  and the recon doc's location.
- When this plan and the codebase disagree about shared-layer behavior, read
  the code, then decide whether to adapt the plan or improve the shared layer
  — improving it is preferred when the fix is game-agnostic (e.g., per-method
  capability degradation).

## 9. Risk register (ranked)

1. **UE4SS bridge fragility** — mitigated by capability detection, pinned
   versions, and keeping 80% of features bridge-independent.
2. **Log format drift** — mitigated by versioned parser tables + committed
   corpus; worst case is stale player list, never a crash.
3. **Save format opacity** — mitigated by the Phase 0 probe gate; Phase 3 is
   skippable without harming Phases 1–2.
4. **Win64/Proton vs native-Linux ambiguity** — resolved in recon before any
   container work; wrong guess here is the expensive one, so it is a gate,
   not an assumption.
5. **UID canonicalization failing open** — mitigated by table-driven tests
   from real IDs before any visibility feature ships.