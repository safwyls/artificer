# Dragonwilds recon (Phase 0)

Findings that gate the Dragonwilds port. Where this document and
`dragonwilds-plan.md` disagree, this document wins — it is the verified
layer.

Two tiers, both dated 2026-08-09. The first sections are **web-sourced**
(cited, but second-hand). "Empirical findings" near the end is **observed
on a real server** and supersedes anything above it that it contradicts —
notably the save format, which the sources got wrong. Items still marked
UNVERIFIED are the gaps a headless server cannot close.

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
  (copy button). **This is platform-agnostic**: the game links both
  `libsteam_api.so` and `libEOSSDK-Linux-Shipping.so`, and EOS treats Steam
  as a login provider, so a Steam player has an EOS-backed id and does not
  need a separate Epic account. Whatever that screen shows is the value
  `OwnerId` wants.
- **Format confirmed against a live account: 32 lowercase hex characters**
  (the EOS ProductUserId shape), e.g. `0a1b2c3d4e5f60718293a4b5c6d7e8f9`.
  Examples here and in tests are synthetic — a real account identifier
  doesn't belong in a repo.
- **The case differs by context, and that is what `CanonicalUID` is for.**
  The Settings screen shows the id lowercase, while the same 32-hex shape
  written by the server itself is uppercase (`ServerGuid=6E8B93DD…` in the
  ini, `WorldSaveGuid` uppercase in the log). So `CanonicalUID` lowercases
  a value that matches `^[0-9a-fA-F]{32}$` and only trims anything else:
  folding hex is lossless, folding an unknown format could collide two
  distinct ids. Table-driven tests cover both branches.

### Saves
- Server world saves: `RSDragonwilds/Saved/SaveGames/*.sav` — but **casing
  differs across sources** (`SaveGames` vs `Savegames`); on Linux, detect the
  directory rather than hardcoding.
- The server loads the **most recently modified** `.sav` on start; an empty
  folder creates a fresh world; **the filename must exactly match the world
  name** or the server starts a new world — restore tooling must preserve
  names, not invent them.
- Format: sources assumed UE5 GVAS. **This turned out to be wrong** — see
  Gate 5 under "Empirical findings": it is the SPUD plugin's chunked
  format, and GVAS tools will not read it.

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
  line as noise. The tests use synthetic lines built from the two verified
  markers, declared as constants and labelled as such; a real capture
  should replace them with committed fixtures (plan §2.3 still stands).
- Autosave and chat log formats: NOT FOUND. No save/chat events in v0.

### Shutdown, bans, admin
- Save-on-SIGTERM was unverified in the sources. **Now measured**: the
  server does *not* save on shutdown, and stops cleanly in ~2 s with exit
  143 — see Gate 3 under "Empirical findings".
- Roles confirmed: Owner (id matches `OwnerId`), Admin (password session,
  revoked by rotating `AdminPassword` + restart), Regular. Owner may ban/unban
  anyone incl. offline; Admins ban online regulars only and cannot unban.
- **Ban list location on disk: NOT FOUND.** Until located, ban/unban is
  in-game only; the UI says so instead of offering dead buttons.
- Moddable vs "Vanilla" (secure) servers is official; UE4SS works server-side
  on Win64 (post-0.12 needs the `version.dll` proxy — the `dwmapi.dll` one
  stopped loading), and has no practical Linux path.

## Codebase deltas (verified against palcon before the split)

Recorded while the port was still a branch inside palcon. The plan's §0
decision — a second in-binary game — was later overridden by the
maintainer in favour of this standalone repo, so the notes below describe
the base as it was inherited, not the current layout. Corrections and gaps
found:

- Next free migration number is **0022** (0020/0021 exist).
- **No typed unsupported-capability error existed.** Added in this change:
  `game.UnsupportedError` (internal/game/client.go), mapped to HTTP 501 by
  `writeClientError` (internal/api/actions.go), so "this game can't" is
  distinguishable from 502 "server didn't answer".
- **The frontend had no per-game page dispatch** — App.tsx hardcoded the
  Palworld page components and `FeatureGate` only decided *whether* to
  render, never *which* component. That mattered while two games shared a
  binary; in this single-game repo the routes point straight at the
  Wildskeeper pages, and a second game would reintroduce a dispatch layer.
- Feature keys are two hand-maintained parallel lists (Go
  `internal/game/registry.go`, TS `web/src/lib/api.ts`) — additions touch both.
- `game.Conn` carries only host/ports/passwords. Dragonwilds' state is
  derived via the sidecar agent (health → process liveness, log tail →
  players), so `Conn` grows `AgentURL`/`AgentToken` with the dragonwilds
  client as first reader. The agent's `/v1/health` already reports
  `Game.StartedAt`, which is the tracker's restart-reset key; `/v1/power/logs`
  serves the stdout ring (2000 lines, pull-based — no streaming exists).
- palagent's launch half was Palworld-hardcoded; retargeted in Phase 2 to
  the native Linux build (see "Phase 2 decisions" below).
- `internal/agentfiles` is the base-side syncer; the agent-side file service
  is `internal/palagent/files.go` (plan's naming was loose).

## Phase 2 decisions (agent + provisioning)

Built on the facts above; recorded here because each one is a place a
wrong guess would be expensive.

- **Launch line.** `./RSDragonwildsServer.sh -log -Port=<port>`. `-log` is
  load-bearing rather than cosmetic: it puts the game's log on stdout,
  which is the stream the supervisor's ring buffer captures and the only
  source `dwlog` derives players from. The port is a command-line flag
  because the ini has no port key.
- **Port pair.** The game binds `Port` and `Port+1` (7777/7778 by
  default), so ports are allocated, validated and published in pairs
  everywhere: the provisioner template, the compose generator, and the
  wizard's free-port proposal (which steps by 2).
- **In-container port is fixed.** Every provisioned container binds 7777
  internally and varies only the host side of the mapping, so the
  template stays one shape.
- **No REST/RCON anywhere in the deployment.** Provisioned rows are
  created with no RCON/REST ports or passwords, the compose file
  publishes neither, and the add-server form no longer asks. The agent
  port is the only management port.
- **Owner id is required to provision.** The game writes its own
  `DedicatedServer.ini` on first run but refuses to start until `OwnerId`
  has a value, so an unattended install would loop. The agent therefore
  *seeds* a minimal ini when the install has none and an owner id is
  configured (`PALAGENT_OWNER_ID`), and the provisioning API rejects a
  request without one. Seeding never overwrites an existing file.
  **This is the one place the port leans on unverified detail**: the
  section name and key spellings come from the multi-source-but-not-
  empirically-verified table above. Kept minimal (owner id, server name,
  admin password, optional world name) so the game can add its own keys;
  if a spelling turns out wrong the symptom is "server still won't
  start", and the fix is one string.
- **Identity enforcement.** ServerName / AdminPassword / OwnerId are
  re-applied to an existing ini on every start under dwconfig's never-add
  policy, so the password the dashboard shows is the one the in-game menu
  accepts.
- **Save layout.** The agent locates saves by trying both `SaveGames` and
  `Savegames`, and the backup runner archives the newest `*.sav` (the
  game's own load rule) with a truncation-only sanity check — no magic
  bytes, because the format is unverified.

## Empirical findings (live server, 2026-08-09)

A throwaway server was stood up from app 4019830 (5.0 GB installed, Linux
depot) and run on an idle world. Everything below is observed, not sourced
— where it contradicts the web-verified section above, this wins.

**Confirmed as written.** The launcher is `RSDragonwildsServer.sh`, whose
last line is `.../RSDragonwildsServer-Linux-Shipping RSDragonwilds "$@"` —
so the agent's `-log -Port=<n>` lands exactly as intended. `libEOSSDK-Linux-
Shipping.so` sits beside it (Epic auth confirmed). Steam's own app metadata
reports `oslist: windows,linux`, confirming the native Linux build.

**The config the game writes itself**, verbatim, on first boot:

```ini
;METADATA=(Diff=true, UseCommands=true)
[/Script/Dominion.DedicatedServerSettings]
AdminPassword=0H9Q8K8DX6M2KYPJ
OwnerId=
ServerGuid=6E8B93DDB72D4F9E95D04A5E31ED22B8
ServerName=Server-3955
WorldPassword=
DefaultWorldName=World-75058
```

Section, `;METADATA` header and every key name match the recon. There is no
`Port` or `MaxPlayers` key, as expected. `Public` was **not** written — it
was single-sourced and may not exist. The game generates its own admin
password, server name and world name, so a seeded config only has to supply
what it can't invent.

**Gate 1 — log corpus: mostly closed.** The UE line shape is confirmed
exactly as the synthetic fixtures assumed:
`[2026.08.09-19.37.07:655][120]LogSpudSubsystem: Save to slot ...`. A
sanitized capture is committed at
`internal/games/dragonwilds/dwlog/testdata/server-lifecycle.log` and is now
asserted against in tests. Newly discovered, previously NOT FOUND:

- save: `LogPersistence: [DedicatedServer] SaveGame() : Starting save (Guid[..] WorldName[..] SlotName[..] ...)`,
  then `LogSpudSubsystem: Save to slot <name>: Success`, then
  `LogPersistence: [DedicatedServer] operator()() : Save completed SUCCESSFULLY (slot: <name>)`.
  `dwlog` now recognises the last of these; it is the only verified
  vocabulary in the table.
- shutdown: `LogDomServerSettings: Shutdown detected, do not save contents to disk`,
  then `LogExit: Exiting.` and `RequestExit(bForce=false, ReturnCode=143)`.
- refusal banner (not UE-formatted) when `OwnerId` is empty.

**Still open:** join and leave lines. Producing them needs real game
clients (the paid game on a machine that can run it — Steam or Epic, since
Steam logins are federated into EOS), so the two community
markers `dwlog` matches remain unverified — the one part of the parser
still resting on someone else's report.

**Port binding does not match the sources.** With `-Port=7777` the running
server binds **7777/udp and one ephemeral high port** (45453 in this run) —
*not* 7778. The "7777 + 7778" pairing is community-sourced and was not
reproduced. Caveats: this was an idle server with no players, and the game
uses Epic relays, so a second well-known port may only appear under
conditions this test didn't create. Engineering stance: provisioning still
publishes the `Port`/`Port+1` pair, because a published-but-unused port
costs nothing while a missing one would be an obscure "players can't
connect" bug — but the pair is defensive, not confirmed. Do not present it
as fact.

**An idle server is completely silent.** After world load it emitted no log
lines at all for ten minutes. Liveness must come from the agent's process
state, never from log activity — which is what the client does.

**Gate 2 — Player ID: closed.** Two separate facts, and they pull in
opposite directions. The format is **32 lowercase hex characters** (EOS
ProductUserId), confirmed against a live account — so the `OwnerGuid[]`
hint was right. But `OwnerId` is **not format-validated**: the server
accepted the literal string `test123` and booted normally. So the
dashboard must *not* reject a value for failing to match the shape — the
game doesn't — while `CanonicalUID` may safely fold case for values that
do match it. A wrong-but-well-formed id fails silently: the server starts,
you simply are not the Owner.

**Gate 3 — shutdown: closed.** SIGTERM to the shipping binary produced a
clean shutdown in **2 seconds**, exit code **143**, and the world save was
**byte-identical before and after** (same md5, mtime and size). The server
does not save on shutdown. Caveats worth keeping: the world was idle, so
"never saves" and "skips a clean world" cannot be told apart here — but the
conservative reading is the same, and the log line above says so in the
game's own words. Consequences already applied to the code: the 30 s stop
grace is generous rather than necessary, exit 143 is correctly read as
clean, and the UI no longer claims a restart saves the world.

Also found: `dom.StateSaveFrequencyMins:5`, alongside `dom.EnablePersistence`
and `dom.PersistenceStrictMode` — CVars, not ini keys, which is why the
autosave interval was never found in config documentation.

**The server refuses to run as root.** Found on the first containerized
run: as uid 0 the binary prints `Refusing to run with the root privileges`
and aborts (exit 134) before touching config or saves — a crash loop under
a supervisor. As any unprivileged uid it boots normally. This is why
`Dockerfile.palagent` bakes a non-root user and the provisioner's compose
warns about volume ownership; the same world then loaded, served, and
stopped cleanly inside the container, so containerized deployment itself
is confirmed viable.

**Autosave is wall-clock, and a later run caught it.** A world-load save at
`21:19:17` was followed by another at `21:24:17` — exactly five minutes,
on a server with nobody connected and nothing happening but EOS session
heartbeats. An earlier run saw no save in a comparable window and this
document briefly claimed the timer was activity-driven; that was wrong,
and the corrected reading is the simpler one: it fires on the interval.
The discrepancy in the earlier run is unexplained (possibly no initial
save to start the timer from, since it loaded rather than created a
world), so treat five minutes as the expected interval rather than a
guarantee.

Practical effect: a stop or restart costs **up to about five minutes** of
play — bounded and predictable, which is better than the earlier reading
implied, but still real given the server never saves on the way out.

**Gate 4 — ban list: not closable here.** Banning requires a second player
in-game. The install tree after boot holds only `Saved/Config`,
`Saved/Logs`, `Saved/SaveGames` and `Saved/PersistentDownloadDir/EOSCache`
— no ban-shaped file exists before anyone is banned, so the hunt has to
happen around a real ban.

**Gate 5 — save format: closed, and the recon was wrong.** The world save
is **not GVAS**. It is the SPUD plugin's chunked format
(`LogPluginManager: Mounting Project plugin SPUD`), magic `SAVE`:

```
53 41 56 45 | 93 de 00 00 | 49 4e 46 4f | 28 02 00 00 ...
 S  A  V  E |   version?   |  I  N  F  O |  chunk length
```

Inside are length-prefixed UE strings — an ISO timestamp, then `VERSION`,
`GUID_A..D`, `WorldName`, `WorldMapName`, `FriendlyFire`,
`SurvivalDifficulty`, `HardcoreState`, `TimeOfSave`, `SessionPrivacy`,
`SessionPasswd`, `CrossplayEnabled`. This is **better news than the plan
assumed**: `uesave` and GVAS walkers will not work, but SPUD is open source
and documented, the container is uncompressed enough to read field names
directly, and world metadata sits in the header rather than behind an
opaque blob. A Go-native reader is realistic for Phase 3 — no Python
dependency, no Oodle. (That reader now exists:
`internal/games/dragonwilds/dwsave`, which also mapped the layout further —
the container is RIFF-style FourCC chunks, the INFO header carries a
name/offset/values field table, and the GUID quads render `%08X` each into
exactly the `WorldSaveGuid` the server logs.)

Save-file behaviour confirmed: the directory is `SaveGames` (capital G),
the file is named for `DefaultWorldName` (`World-75058.sav`), and a fresh
world is written once at creation.

## Still open (need real game clients)

1. **Join/leave log lines** — the last unverified regexes in `dwlog`.
2. **Ban list at rest** — whether offline ban/unban can be done by editing
   a file, or stays in-game only.
3. **Autosave with players present** — the 5-minute interval is confirmed
   on an idle server; whether player activity changes it is untested.
4. **Whether a second well-known UDP port opens** once a player connects
   (only 7777 plus an ephemeral port was seen idle).

All four need a player actually in the world; none can be closed from a
headless server alone. (The Player ID gate is now closed — see "Player
identity".)
