# Dragonwilds recon (Phase 0)

Findings that gate the Dragonwilds port, per `dragonwilds-plan.md` (kept in the
wildskeeper workspace beside this repo). Where this document and that plan
disagree, this document wins — it is the verified layer. Facts below are dated
2026-08-09 and sourced; items marked **UNVERIFIED** are the empirical gaps that
still need a live server to close, with the interim engineering stance noted.

## External facts (web-verified)

### Steam / install
- Dedicated server app id: **4019830** ("RuneScape: Dragonwilds Dedicated
  Server", free, anonymous login works):
  `steamcmd +login anonymous +app_update 4019830 validate +quit`.
  Game client app id is 1374490 (don't confuse the two; one community repo
  wrongly says 2949930).
  Sources: dragonwilds.runescape.wiki/w/Dedicated_Servers, steamdb.info/app/4019830.
- **Native Linux and Windows server builds both ship.** Linux launcher is
  `RSDragonwildsServer.sh` at the install root; the real binary is
  `RSDragonwilds/Binaries/Linux/RSDragonwildsServer-Linux-Shipping`, run as
  `./RSDragonwildsServer-Linux-Shipping RSDragonwilds -log -Port=<port>`.
  This resolves the plan's Win64-vs-Linux ambiguity for the *base* server:
  **run native Linux**. The Win64-under-Proton route is only needed if/when
  the UE4SS command bridge (Phase 4) happens, because UE4SS is Win64-only in
  practice (no usable Linux port for this game).
- Current game version 0.12.1.4 (2026-08-05). Community sources expect a 1.0
  launch 2026-09-15 — assume format churn around it.

### Config
- Path: `RSDragonwilds/Saved/Config/<LinuxServer|WindowsServer>/DedicatedServer.ini`.
- Section: `[/Script/Dominion.DedicatedServerSettings]`. Real files also carry
  a `;METADATA=(...)` comment header and a `[SectionsToSave]` section — an
  editor must preserve both (dwconfig does: it never touches non-`Key=Value`
  lines).
- Known keys: `OwnerId` (**exact spelling — not `OwnerID` as the plan and mock
  wrote**), `ServerName`, `DefaultWorldName`, `AdminPassword`, `WorldPassword`,
  `ServerGuid` (auto-generated, 32 hex chars, do not edit), `Public`.
- **The server refuses to start without `OwnerId`** (official guide).
- **There are no `MaxPlayers` or `Port` ini keys** (plan was wrong): the port
  is the `-Port=7777` launch argument (game uses 7777/udp + 7778/udp), and the
  player cap is a flat 6.
- The wiki states the list of players who have used the admin password is
  recorded *inside* `DedicatedServer.ini` (likely repeated keys — dwconfig
  already serves duplicate keys read-only).
- Edits made while the server runs are lost; every config write needs the
  "restart to apply" framing the mock uses.
- Memory sizing: 2 GB base + 1 GB per player (official guidance) — the mock's
  vitals hint is accurate.

### Player identity
- A player's id is shown in-game at Settings → bottom-left "My Player ID"
  (copy button). **The wire format is undocumented — no source shows a real
  value.** UNVERIFIED. Consequence: `CanonicalUID` is a trim-only identity
  function for now; no case-folding or reformatting until we've captured real
  ids from a live server (the plan's fail-open warning applies — an invented
  normalization is worse than none). Table-driven tests exist and grow as
  real spellings arrive.

### Saves
- Server world saves: `RSDragonwilds/Saved/SaveGames/*.sav` — but **casing
  differs across sources** (`SaveGames` vs `Savegames`); on Linux, detect the
  directory rather than hardcoding.
- The server loads the **most recently modified** `.sav` on start; an empty
  folder creates a fresh world; **the filename must exactly match the world
  name** or the server starts a new world — restore tooling must preserve
  names, not invent them.
- Format: UE5 `.sav`; standard-GVAS parse **UNVERIFIED** (community editor
  Elleandria/RS-Dragonwilds-Editor and trumank/uesave are the candidate
  tools). Phase 3 stays gated on an actual parse probe.

### Logs
- `RSDragonwilds/Saved/Logs/RSDragonwilds.log`; with `-log` the same stream
  reaches stdout, which is what palagent's supervisor ring captures.
- Production-verified markers (used by the AltSystem42 docker image to count
  players):
  - join: lines containing `LogNet: Join succeeded:`
  - leave: lines containing `LogDominionPlayerController: ClientRequestDisconnect`
- **The full line shapes (name/id fields, timestamps) are UNVERIFIED** — no
  committed corpus yet. Consequence: `dwlog`'s v0 parser table matches the two
  markers above and extracts a trailing player name where present, is
  versioned so a corpus can replace it wholesale, and treats every unmatched
  line as noise. `internal/games/dragonwilds/dwlog/testdata/logs/` holds
  synthetic lines built from the two verified markers, clearly labeled; the
  first real capture replaces them (plan §2.3 still stands).
- Autosave and chat log formats: NOT FOUND. No save/chat events in v0.

### Shutdown, bans, admin
- **Save-on-SIGTERM is unverified.** No official statement; community images
  stop with a plain kill and a ~30 s grace window. Stance: `Client.Save` and
  `Client.Shutdown` return the typed unsupported error; power stop/restart
  goes through the agent with a generous grace period, and backups cover the
  gap. Revisit after empirical testing (plan §2.2 gate).
- Roles confirmed: Owner (id matches `OwnerId`), Admin (password session,
  revoked by rotating `AdminPassword` + restart), Regular. Owner may ban/unban
  anyone incl. offline; Admins ban online regulars only and cannot unban.
- **Ban list location on disk: NOT FOUND.** Until located, ban/unban is
  in-game only; the UI says so instead of offering dead buttons.
- Moddable vs "Vanilla" (secure) servers is official; UE4SS works server-side
  on Win64 (post-0.12 needs the `version.dll` proxy — the `dwmapi.dll` one
  stopped loading), and has no practical Linux path.

## Codebase deltas (verified against this repo)

The plan's §0 decision — second in-binary game — holds: `game.Definition` +
`game.Register`, `internal/games/games.go`, feature gating, and
`0019_game.sql` all exist as described. Corrections and gaps found:

- Next migration number is **0022** (0020/0021 exist), not 0020.
- **No typed unsupported-capability error existed.** Added in this change:
  `game.UnsupportedError` (internal/game/client.go), mapped to HTTP 501 by
  `writeClientError` (internal/api/actions.go), so "this game can't" is
  distinguishable from 502 "server didn't answer".
- **The frontend has no per-game page dispatch** — App.tsx hardcodes Palworld
  page components; `FeatureGate` decides whether to render, never which
  component. The Wildskeeper pages therefore come with a small game→component
  dispatch layer. `AppShell` also unconditionally preloads Palworld map
  textures — gate on game id.
- Feature keys are two hand-maintained parallel lists (Go
  `internal/game/registry.go`, TS `web/src/lib/api.ts`) — additions touch both.
- `game.Conn` carries only host/ports/passwords. Dragonwilds' state is
  derived via the sidecar agent (health → process liveness, log tail →
  players), so `Conn` grows `AgentURL`/`AgentToken` with the dragonwilds
  client as first reader. The agent's `/v1/health` already reports
  `Game.StartedAt`, which is the tracker's restart-reset key; `/v1/power/logs`
  serves the stdout ring (2000 lines, pull-based — no streaming exists).
- palagent's launch half is env-overridable (`PALAGENT_GAME_CMD/ARGS`) but
  its config seeding, `configRelPath`, and `PalServer-Linux` process lookup
  are Palworld-hardcoded — that's the Phase 2 surface, now known to target
  the **native Linux** build:
  `PALAGENT_GAME_CMD=./RSDragonwildsServer.sh`, app id 4019830.
- `internal/agentfiles` is the base-side syncer; the agent-side file service
  is `internal/palagent/files.go` (plan's naming was loose).

## Open gates (need a live server)

1. Log corpus capture → replaces `dwlog`'s v0 synthetic fixtures; unlocks
   name/id extraction, save/chat events, world-day if logged.
2. Real Player ID samples → decides whether `CanonicalUID` needs more than
   trim; feeds the uid test table.
3. SIGTERM save behavior + clean-stop duration → decides `Save`/`Shutdown`
   via signals vs bridge-only, and the watchdog stop timeout.
4. Ban-list-at-rest hunt (diff the install/save tree around a ban) → decides
   file-edit ban/unban vs bridge/in-game-only.
5. Save parse probe (uesave / gvas / RS-Dragonwilds-Editor) → gates Phase 3.
