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
  **run native Linux**. The Win64-under-Wine route is only needed if/when
  the UE4SS command bridge (Phase 4) happens, because UE4SS is Win64-only in
  practice (no usable Linux port for this game) — and that route is now
  **proven to work**; see "Phase 4 unblocked" below.
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
  reaches stdout, which is what wkagent's supervisor ring captures.
- Production-verified markers (used by the AltSystem42 docker image to count
  players):
  - join: lines containing `LogNet: Join succeeded:`
  - leave: lines containing `LogDominionPlayerController: ClientRequestDisconnect`
- **Line shapes now VERIFIED against a real join/leave** (2026-08-09, a
  client on this LAN; see the closed gate below). The best lines are the
  session pair, which carry the player id and name symmetrically:
  - `LogDomMatcherSession: Player ADDED to session [<32hex>]-[<name>]`
  - `LogDomMatcherSession: Player Removed from session [<32hex>]-[<name>]`
    (the ADDED/Removed case difference is the game's own)
  - the disconnect line also identifies the player:
    `ClientRequestDisconnect : DisconnectMe : PlayerStateSave result[true] -
    state saved for Account[XP:<32hex>] Character Name[<name>]
    Guid[DCG:<32HEX>]` — note `PlayerStateSave result[true]`: **a leave
    writes state**, and a world `Save completed SUCCESSFULLY` fired at the
    same instant.
  Both v0 markers also fired ("Join succeeded" 1 ms after ADDED), so a
  parser must not treat them as separate joins. `dwlog`'s RulesV1 is
  written from this capture (committed corpus:
  `dwlog/testdata/player-session.log`, real shapes with synthetic
  ids/names) and keys sessions by the real player id.
- Chat log format: NOT FOUND (no chat happened in the capture).

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
  `game.UnsupportedError` (core/game/client.go), mapped to HTTP 501 by
  `writeClientError` (core/api/actions.go), so "this game can't" is
  distinguishable from 502 "server didn't answer".
- **The frontend had no per-game page dispatch** — App.tsx hardcoded the
  Palworld page components and `FeatureGate` only decided *whether* to
  render, never *which* component. That mattered while two games shared a
  binary; in this single-game repo the routes point straight at the
  Wildskeeper pages, and a second game would reintroduce a dispatch layer.
- Feature keys are two hand-maintained parallel lists (Go
  `core/game/registry.go`, TS `web/wildskeeper/src/lib/api.ts`) — additions touch both.
- `game.Conn` carries only host/ports/passwords. Dragonwilds' state is
  derived via the sidecar agent (health → process liveness, log tail →
  players), so `Conn` grows `AgentURL`/`AgentToken` with the dragonwilds
  client as first reader. The agent's `/v1/health` already reports
  `Game.StartedAt`, which is the tracker's restart-reset key; `/v1/power/logs`
  serves the stdout ring (2000 lines, pull-based — no streaming exists).
- wkagent's launch half was Palworld-hardcoded; retargeted in Phase 2 to
  the native Linux build (see "Phase 2 decisions" below).
- `core/agentfiles` is the base-side syncer; the agent-side file service
  is `core/agent/files.go` (plan's naming was loose).

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
  configured (`WKAGENT_OWNER_ID`), and the provisioning API rejects a
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
`games/dragonwilds/dwlog/testdata/server-lifecycle.log` and is now
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
`Dockerfile.wkagent` bakes a non-root user and the provisioner's compose
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
`games/dragonwilds/dwsave`, which also mapped the layout further —
the container is RIFF-style FourCC chunks, the INFO header carries a
name/offset/values field table, and the GUID quads render `%08X` each into
exactly the `WorldSaveGuid` the server logs.)

Save-file behaviour confirmed: the directory is `SaveGames` (capital G),
the file is named for `DefaultWorldName` (`World-75058.sav`), and a fresh
world is written once at creation.

## Closed 2026-08-09: a real client joined

A player joined and left the local server, which closed most of what was
blocked on "needs a real client":

**Join/leave lines: closed.** The shapes are in the Logs section above and
in `dwlog`'s committed corpus; `dwlog` RulesV1 parses them and carries the
real player id into the player list.

**Bans at rest: located, enforcement untested.** After the join,
`DedicatedServer.ini` gained a `KnownPlayerList` entry:

```
KnownPlayerList=(UserId=<32hex lowercase>,UserName="<name>",Privileges=(PrivilegeMask=14),LastAdminPassword="",bIsBanned=False)
```

So the player roster — id, name, privileges, and a `bIsBanned` flag — is
an ini line. That makes an offline ban *plausibly* a config edit, but
whether the server honors a hand-set `bIsBanned=True` (and when it re-reads
the file) is untested. The log also shows a second, EOS-side layer: on
connect the server runs a `SanctionCheck` ("Checking to see if user has
any BAN sanctions"), which is Epic's service, not this file. dwconfig's
never-add-never-remove policy already preserves the KnownPlayerList line.

**Player state in the world save: located.** The save now embeds a JSON
character record — `"char_guid"` (the `DCG:` guid from the disconnect
line), `"char_name"`, `"worlds_playtime"` keyed by the world's save GUID,
`"SaveCount"`, `"Customization"`. The **EOS player id appears nowhere in
the save**: the id↔character mapping lives in the ini's KnownPlayerList
and the log lines, so save-side identity matching must route through
those (which is what dwlog v1 + CanonicalUID provide).

## Phase 4 unblocked: UE4SS runs under Wine (2026-08-09)

The dwbridge feasibility chain was tested end to end on the WSL2 box and
every link held:

1. **The Windows server runs headless under plain Wine** (`wine-core`
   11.0, no Proton, no display tricks): same app id via
   `+@sSteamCmdForcePlatformType windows`, boots to a loaded world with
   EOS sessions registered and heartbeating. Only noise: `LogNNERuntimeORT`
   failing to create a D3D12 device, harmlessly.
2. **UE4SS's stock injection does not fire — on any OS.** The server exe
   imports no `dwmapi.dll` (checked in its PE import table; servers have
   no window manager), so the shipped proxy never loads. `version.dll` is
   imported, and a minimal version.dll shim that loads UE4SS from DllMain
   works — build and usage in `tools/ue4ss-wine-shim/`, run with
   `WINEDLLOVERRIDES="version=n,b"`.
3. **The UE4SS nightly handles UE 5.6.1.** The 2026-08-08
   `experimental-latest` build (v3.0.1-1021-g1c1a1497) detects Wine
   ("local disabled due to wine"), and its pattern scan finds every core
   signature — GUObjectArray, GMalloc, FName, StaticConstructObject,
   ConsoleManager, GameEngineTick — in ~250 ms.
4. **Lua mods reach live game state.** A probe mod resolved the running
   `World` and the `BP_GameMode_C` instance from inside the server —
   the handles a dwbridge mod would drive kick/ban/broadcast through.

Phase 4 is therefore a mod-authoring task, not a platform gamble. The
costs it brings: the game must run as the Windows build under Wine (a
wkagent launch-profile variant), and the 1.0 launch (expected
2026-09-15) may shift signatures — pin the UE4SS build that works.

## Command surface (mapped live via UE4SS, 2026-08-09)

Enumerated by dumping UFunctions and signatures from inside the running
server. This is what the dwbridge mod calls (or will call):

- **save — implemented and verified headless.**
  `PersistenceSubsystem:SaveGame(bAdditionalLogging: bool)` is a subsystem
  method, so it runs with no player connected. Calling it from Lua wrote the
  world and logged `Save completed SUCCESSFULLY` — the same path autosave
  takes. `SpudSubsystem:SaveGame(SlotName, Title, ...)` sits under it; the
  Persistence wrapper is the right level to call.
- **kick / ban / unban — mapped, and BLOCKED. Do not retry the obvious path.**
  Tested against a connected client 2026-08-09; see "Why the command tier
  stops at save" below. In short: `Server_RequestAdminAction(Action, UserId)`
  exists (`EAdminAction { Kick=0, Ban=1, Unkick=2, Unban=3 }`, verified off
  the live UEnum), but calling it from the server does nothing — a
  `Server_` RPC invoked *on the server* is a no-op. Bans also surface at
  rest in the ini `KnownPlayerList` (`bIsBanned`), which remains the most
  promising route for offline ban/unban.
- **broadcast — mapped, and blocked the same way.**
  `PlayerChatComponent:Server_SendChatMessage(ChatMessageData)` where
  `ChatMessageData = {SenderId, CharacterGuid, PlayerId: int, Color,
  MessageBody: string}` (dumped from the live struct).
  `Client_ReceiveSystemMessage(Tag: FGameplayTag)` carries **no text** — the
  game's system messages are canned tags, so free text has to go through
  `ChatMessageData`. Reading works: a hook on `Server_SendChatMessage`
  captured a real player's message verbatim. Sending does not — see below.

### Why the command tier stops at `save`

Three findings from driving a live modded server with a connected player.
Each is repeatable, and together they explain why only `save` shipped:

1. **`Server_` RPCs are no-ops when invoked on the server.** The function
   the reflection exposes is the *client-side send stub*: it serialises the
   arguments and hands them to the net driver. Called on the authority there
   is nobody to send to, so it returns cleanly having done nothing — which
   is exactly what a kick looked like (`ok=true`, player still connected,
   no session-removal line in the log). The real behaviour lives in
   `Server_RequestAdminAction_Implementation`, native C++ that is not
   reflected and so cannot be called. Submitting the admin password first
   (`Server_SubmitAdminPassword`, same no-op class) changes nothing.
2. **Native UFunctions hang the UE4SS Lua VM in this build.** Calling
   `PlayerController:ClientMessage` (3-arg native) or `ClientWasKicked`
   wedges the Lua thread: the *game* keeps running normally and players
   stay connected, but the mod stops responding — the heartbeat freezes and
   the bridge correctly reports itself unavailable. Recovery needs a server
   restart. Only Blueprint-exposed functions are safely callable, which
   rules out UE's whole standard kick path.
3. **No Blueprint-exposed kick/ban exists anywhere.** `DominionGameSession`,
   `BP_GameMode_C` and `BP_GameState_C` were each walked in full: not one
   `Kick`/`Ban`/`Admin` function among them. The GameState does carry
   replicated `KickedUsers` / `BannedUsers` arrays (readable, empty on a
   fresh server) whose `OnRep_` handlers exist client-side — mutating those
   directly is the one untried lead, though the actual disconnection is
   still native code, so replicating the array may only move UI state.

Consequence for the mod: `kick`/`ban`/`unban` were implemented, tested
against a live player, found to silently do nothing, and **removed**. A
verb that reports success while nothing happens is worse than an honest
501 — the console would tell an operator a troublemaker was kicked when
they were still in the world. The mod advertises only what it can do.
- **shutdown — not a bridge command.** Stopping the process is the agent's
  supervisor job, with a grace period; a mod-driven shutdown would be
  strictly worse (it can't stop a hung game). Left pointed at the agent.

### The bridge transport

The mod (`tools/dwbridge`) and wkagent share a directory
(`<install>/dwbridge/`); the mod writes a heartbeat there and answers
`request.json` with `response.json`. Single-flight, fixed filenames — a
management console issues one command at a time, and it dodges the fact that
`io.popen('dir')` and rename-over-existing are both unreliable under Wine
(the mod removes a file before renaming onto it, or the heartbeat freezes at
its first value). wkagent exposes it as `POST /v1/bridge/command`, reports
freshness as `health.bridge`, and the dragonwilds client routes a command
through it only when the heartbeat lists that command — otherwise the honest
501 stands.

## Closed with a live player on the modded server (2026-08-09)

**Autosave is 5 minutes regardless of activity.** A server with a player
connected and actively playing saved 5 min 13 s after boot — the same
cadence measured idle. Player activity does not tighten or loosen it, so
the "a restart costs up to ~5 minutes of play" figure holds under load
too. The save additionally writes a player chunk, and the game logs its
own cosmetic bug doing it: `LogSpudData: Error: Chunk ID Players is more
than 4 characters long, will be truncated`.

**No second well-known UDP port, even with a player connected.** With one
player in-world the server held exactly two UDP sockets: the configured
game port, and one *ephemeral* high port (observed 45453). The 7778 the
web sources claim never appears. Provisioning still reserves the pair,
which is now knowingly defensive rather than required.

**Chat lines: readable at the source, absent from the log.** A player's
chat message was captured live by hooking `Server_SendChatMessage` (body
verbatim), but chat does **not** appear in the server's stdout log, so
`dwlog` cannot see it. Chat monitoring would have to come through a mod.

## SPUD object layer mapped; character records readable (2026-08-19)

The layout below GLOB/LVLS — the object state `dwsave` originally declined
to parse — is now byte-mapped, from the committed capture
(`games/dragonwilds/testdata/world-empty.sav`) cross-checked against the
open-source SPUD plugin's serialization code. Everything in this list was
verified by decoding the capture and matching known ground truth (the
DomPersistence global object's fields reproduce the INFO header's
WorldName/GUID/HardcoreState exactly):

- **GLOB** = CurrentLevel FString, then `META` and `GOBS` chunks (plus a
  Dominion-specific `GLAI` this mapping leaves alone). **LEVL** = Name
  FString, then **two u32 version stamps (522, 1017 in this capture)** —
  a Dominion addition over stock SPUD — then `META`, `LATS`, `SATS`,
  `DATS`.
- **META** holds `VERS` (u32), `CNIX`/`PNIX` (u32 count + FStrings; class
  and property name tables), and `CLST`. Dominion wraps each class def in
  a **`CDVE`** chunk (one unknown byte, then a stock `CDEF`) — stock SPUD
  has CDEFs directly under CLST. CDEF = ClassName FString, u16 count,
  then (u32 propNameID, u32 prefixID, u16 storageType) per property.
- **NOBJ** = u32 classID, Name FString, then — Dominion again — a
  **count-prefixed u32 array of component class ids** (the save's own
  record of which saved components ride the actor), the same two version
  stamps, then `CORA`/`PROP`/`CUST` chunks.
- **CORA** = TArray<u8> holding: u16 version (1), u8 hidden, FTransform
  (quat 4×f64, translation 3×f64, scale 3×f64), then velocity, angular
  velocity and control rotation at 3×f64 each — 155 bytes, matching the
  stock SPUD writer line for line. The translation is the actor's world
  position.
- **PROP** = u32 offset count + offsets (fence-posted against the data
  length), then u32 data length + data; property *i* spans
  offsets[i]..offsets[i+1], decoded by the class def's storage type.
- **Opaque records** (storage type 64) are UE 5.4 tagged-property
  streams: u32 record count, then per record a stream of property tags
  terminated by a property named `None`. Each tag: name FString, a
  recursive **type-name tree** (FString + u32 param count + params — so
  `StructProperty(DomCharacterGuid(/Script/Dominion))` is three nested
  names, and a primitive's lone u32 0 is its empty param list, not an
  array index as an earlier revision of this section guessed), u32
  payload size, u8 flags (UE's EPropertyTagFlags: 0x08 = the payload is
  native binary, 0x10 = the bool value, 0x01/0x02 = array index /
  property guid follow), then the payload. A struct payload without the
  native flag is itself a tagged stream; native ones are raw (Guid 16
  bytes, Vector 3×f64, Quat 4×f64). Verified against the capture's
  LastSavedByEntries (Branch="++dominion+live" / Changelist=232224 /
  bFullSave) and against a real played save's transform records, which
  is where the flag semantics became unambiguous.

**Where player state lives — and how it moved between builds.** The
level actors include `Dominion.CachedCharacterStates` (property
`CachedCharacterStates`, opaque record) and
`Dominion.SavedCharacterTransformsManager` (property
`SavedCharacterTransforms`, opaque record). A real played save from a
**current** game build (4 MB, four characters, examined 2026-08-19)
settled what they hold: `SavedCharacterTransforms` carries one record
per character — `CharacterGuid` (a DomCharacterGuid wrapping a native
Guid), a full `Transform` whose `Translation` is the character's last
position, and a `LastUpdated` float on the game's own clock —
`CachedCharacterStates` was empty even with four characters, and **the
JSON character records the 2026-08-09 recon saw embedded in the world
save are gone**: no `char_guid`, `meta_data`, `Skills` or player names
appear anywhere in the file (the only embedded JSON is container
inventories — chests — as pretty-printed slot maps).

**Corrected 2026-08-20 — the save is empty of sheets only when nobody
is on.** The reading above was taken from a single snapshot and
generalised too far. Watching a live server settled the real rule:
character data lives client-side on each player's machine *and* the
server caches a connected player's full record, writing it into the
world save on the next autosave; when that player disconnects the
cache entry is dropped and the following save no longer carries it.
The 4 MB snapshot had `CachedCharacterStates` at record count zero
because it was taken with nobody connected. So a world save read at
any instant carries:

- a `SavedCharacterTransforms` record for **every** character who has
  played — guid, last position, freshness — which persists across
  sessions, and
- a full JSON character sheet for **only those connected as of that
  save** — skills, inventory, vitals, progression — found by the same
  scan that reads old-build embedded records.

Consequences for anything reading this: a sheet is not missing because
the server never had it, it is missing because that player is offline;
and a sheet that is present is at most one autosave (~5 min) old.
`dwapi`'s `records.go` remembers each sheet it sees — persisting the
save-read ones — so an offline player's does not vanish from the
console, stamped with when it was true. Names come back independently through the disconnect log line
(`Character Name[<name>] Guid[DCG:<32HEX>]`), which `dwlog` harvests —
still the only source for a character the console has never seen
online. The 0.12-era "Players" chunk the server log complained about
has no counterpart in this save.

**The character record itself is JSON**, embedded in the save as string
data — the recon of 2026-08-09 saw `char_guid`, `char_name`,
`worlds_playtime`, `SaveCount` and `Customization` in a real played world
save. The schema was first known from community editors of the old-build
client character saves, and on 2026-08-19 **verified against a real
current-build client record** (`Saved/SaveCharacters/<name>.json` — the
extension is `.json` now, not the `.sav` community tooling knew).
Between builds the record moved and shifted, and the parser handles both
layouts:

- Old builds: `Character`/`Inventory`/`Loadout`/`Skills` at the top
  level; `char_guid` is 22-char base64url; `worlds_playtime` maps world
  guid → **seconds played**; positions are plain FVector::ToString
  (`X=… Y=… Z=…`).
- Current builds: the same body nests under **`GameProgress`**;
  `char_guid` is 32-hex (matching the world save's transform records
  directly); `worlds_playtime` values became **last-played unix
  timestamps** — the real duration is `Character.Playtime_wall` — and
  positions wear the compact `V(X=…, Y=…, Z=…)` spelling. Loadout slots
  can be hotbar references (`PlayerInventoryItemIndex`) rather than
  items. Cross-check that closed the loop: the real record's guid and
  `LastAccessibleLocation` matched one of the same world's binary
  transform records to the meter.

`dwsave` reads player state through both doors: it **parses the
`SavedCharacterTransforms` records** (guid, position, freshness — the
current build's whole story), and it **scans the save for embedded JSON
documents carrying the character-record identity keys** for builds that
embed them, merging the two by guid — the JSON's `char_guid` is the same
16 bytes base64url-encoded that the binary record renders as 32 hex.
Shapes the JSON scan must tolerate, both hit for real: **the game
pretty-prints its embedded JSON** (newlines and tabs between every token
— a scan that assumes `{"` adjacency finds nothing, which is exactly how
a live multi-player server briefly showed an empty character list on
2026-08-19), and **UE serializes a whole string as UTF-16LE the moment
one character in it is non-ASCII**, so a record mentioning an accented
name goes wide and an ASCII-only scan misses it. Records reached through a wrapper document as escaped
strings are also found. A build that stops using JSON degrades to an
empty player list, never a misread. The console serves XP raw; the
frontend derives the displayed level on the classic RuneScape curve —
an assumption, with its caveat and the hover-for-exact-XP escape hatch
documented in `games/dragonwilds/docs/vendored-game-data.md`.

## Still open

1. **Ban enforcement** — does the server honor a hand-edited
   `bIsBanned=True` in the ini's `KnownPlayerList`, and does it re-read the
   file to see it? This is now the most valuable open question, because it
   is the only plausible route to offline ban/unban given the command-tier
   findings above.
2. **The `KickedUsers` / `BannedUsers` GameState arrays** — whether pushing
   a net id into them actually disconnects a player, or merely replicates
   UI state. The one untried lead for a live kick.
(A third question — where a current-build dedicated server keeps its
players' character records — was opened 2026-08-19, answered wrongly
that day from a single snapshot ("it doesn't"), and settled 2026-08-20
by watching a live server: it caches them while players are connected
and drops them at logout. See "Where player state lives" above, and
treat one snapshot of a save as one moment, never as the rule.)
