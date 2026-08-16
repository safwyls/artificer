# Enshrouded dedicated server — recon

Compiled 2026-08-15 from web sources and the two dominant community docker
projects' tracked code, then **partly verified the same day against a real
server** (build `b466cef15`, game version 1024233, raised through the
wizard on the NAS with a real client joining). The "Verification ledger"
at the bottom is the running record of what that server has and has not
settled; until a fact is checked off there, treat it exactly as its
confidence marker says.

Confidence markers: **[verified]** = observed on our own server, with the
date; **[official]** = Keen docs (enshrouded.zendesk.com) or first-party,
directly or via mirrors; **[code]** = read directly from the docker
projects' tracked source ([jsknnr/enshrouded-server], [mornedhels/
enshrouded-server]); **[community]** = hosting guides / wiki / forums,
plausible but not first-party; **[uncertain]** = conflicting or
unverifiable.

The first capture already overturned one section outright: the log line
vocabulary (see Logs) bore little resemblance to the community quotes,
which is why the roster read empty on a server with someone playing on
it. Treat every remaining **[community]** marker with that in mind.

[jsknnr/enshrouded-server]: https://github.com/jsknnr/enshrouded-server
[mornedhels/enshrouded-server]: https://github.com/mornedhels/enshrouded-server

## Steam app id & platform

- **Dedicated server tool app id: `2278520`** (game client is `1203620`).
  **[code]** — hardcoded in both docker projects.
- **Windows-only binary: `enshrouded_server.exe`. No native Linux build
  exists.** Every Linux path runs the Windows exe through a compat layer;
  steamcmd needs `+@sSteamCmdForcePlatformType windows` (which must
  precede `+login` or it is ignored). **[code]** One SEO'd community page
  claims a Linux server exists — contradicted by both docker projects'
  current code; folklore.
- Community compat layers: jsknnr (most-pulled image) uses **GE-Proton**
  on debian-slim; mornedhels maintains **Proton** (latest) and **Wine**
  (`stable-wine`) variants in parallel; bare-metal guides (ZAP-Hosting,
  pimylifeup) use plain `wine64` with `WINEDEBUG=-all` and a dedicated
  `WINEPREFIX`. **[code + community]** flameagent takes the plain-Wine
  path — no Proton runtime to manage, and the wine variant is proven by
  mornedhels' image — with `FLAMEAGENT_WINE_BIN` as the escape hatch.
- Install: `+force_install_dir … +login anonymous +app_update 2278520
  validate`. A `testing` beta branch exists. **[code]**
- steamcmd quirk: some hosts need a `CPU_MHZ` env workaround (mornedhels
  sets `CPU_MHZ=1500.000`). **[code]**

## Config: `enshrouded_server.json`

Next to the exe in the install dir; auto-generated **with defaults — an
open, password-less server** — on first start when absent. **[official
via mirrors]** That default is why flameagent seeds a complete config
*before* first boot (esconfig.Seed). The install also ships
`enshrouded_server_readme.txt` documenting the format. **[community]**

Top-level schema as of v0.9.x (verbatim from jsknnr's tracked example,
updated 2025-12 "for current version"; matches official mirrors) **[code]**:

| Key | Type | Notes |
|---|---|---|
| `name` | string | server-browser name |
| `saveDirectory` | string | default `./savegame`, resolved against the working directory |
| `logDirectory` | string | default `./logs` |
| `ip` | string | bind address, default `0.0.0.0` |
| `queryPort` | int | default `15637`. **The only port key** — see Ports |
| `slotCount` | int | 1–16 (16 is the hard cap) |
| `tags` | []string | server-browser tags |
| `voiceChatMode` | enum | `Proximity` \| `Global` |
| `enableVoiceChat` | bool | default false |
| `enableTextChat` | bool | default false |
| `gameSettingsPreset` | enum | `Default` \| `Relaxed` \| `Hard` \| `Survival` \| `Custom` — **`gameSettings` is only honored when `Custom`** |
| `gameSettings` | object | ~38 tunables, below |
| `userGroups` | []group | role groups, below |
| `bannedAccounts` | []string | persistent ban list (SteamID64s), maintained by the in-game kick/ban UI, hand-editable **[code + community; element format unverified]** |
| `password` | string | **legacy, ignored** since roles arrived — mornedhels marks its env for it deprecated **[code]** |

### Role groups (`userGroups`, since Update #2, 2024-06)

Joining players type one of the group passwords; the matching group
defines their permissions. Fields per group **[code, both repos]**:
`name`, `password`, `canKickBan`, `canAccessInventories`, `canEditBase`,
`canExtendBase`, `canEditWorld` (newer — 2025-era), `reservedSlots` (int).
The game's own generated default is one group `"Default"` with an empty
password and full permissions — i.e. open. flameagent seeds two groups
("Admins" with `canKickBan`, "Friends" without) and enforces the
configured passwords onto them by *capability*, not name, on every start.

### `gameSettings` (since Update #3, 2024-07)

Durations are **nanosecond int64s** — which is why esconfig parses with
`json.Number`; a float64 round trip would corrupt them. Full table
(mornedhels `docs/SERVER_DIFFICULTY.md`, mirroring the official zendesk
table, cross-checked against jsknnr's example) **[official via mirrors +
code]**:

Note the log's own dump renders these oddly — floats as quoted decimal
strings, durations as `{"value": 1800000000000}`, and
`perkUpgradeRecyclingFactor` as a float32 hex bit pattern. That is the
*log* format, not the file's; `enshrouded_server.json` takes plain JSON
numbers.

floats 0.25–4ish (default 1 unless noted): `playerHealthFactor`,
`playerManaFactor`, `playerStaminaFactor`, `playerBodyHeatFactor`
(0.5–2), `playerDivingTimeFactor` (0.5–2), `foodBuffDurationFactor`
(0.5–2), `shroudTimeFactor` (0.5–2), `miningDamageFactor` (0.5–2),
`plantGrowthSpeedFactor` (0.25–2), `resourceDropStackAmountFactor`
(0.25–2), `factoryProductionSpeedFactor` (0.25–2),
`perkUpgradeRecyclingFactor` (0–1; default **0.5 [verified]** — a real
server logs it as `"3f000000"`, the float32 bit pattern for 0.5, settling
the jsknnr-vs-mornedhels disagreement),
`perkCostFactor` (0.25–2), `experienceCombatFactor` (0.25–2),
`experienceMiningFactor` (~0–2), `experienceExplorationQuestsFactor`
(0.25–2), `enemyDamageFactor` (0.25–5), `enemyHealthFactor` (0.25–4),
`enemyStaminaFactor` (0.5–2), `enemyPerceptionRangeFactor` (0.5–2),
`bossDamageFactor` (0.2–5), `bossHealthFactor` (0.2–5), `threatBonus`
(0.25–4).

bools: `enableDurability` (true), `enableStarvingDebuff` (false),
`enableGliderTurbulences` (true), `pacifyAllEnemies` (false).

enums: `tombstoneMode` (`AddBackpackMaterials` | `Everything` |
`NoTombstone`), `weatherFrequency` (`Disabled`|`Rare`|`Normal`|`Often`),
`fishingDifficulty` (`VeryEasy`…`VeryHard`), `randomSpawnerAmount` /
`aggroPoolAmount` (`Few`|`Normal`|`Many`|`Extreme`),
`tamingStartleRepercussion` (`KeepProgress`|`LoseSomeProgress`|
`LoseAllProgress`), `curseModifier` (`Off`|`Normal`|`Hard`).

int64 ns: `fromHungerToStarving` (3e11–1.2e12, default 6e11),
`dayTimeDuration` (1.2e11–3.6e12, default 1.8e12 = 30 min),
`nightTimeDuration` (1.2e11–1.8e12, default 7.2e11 = 12 min).

Schema timeline: diving/fishing/curse keys arrived with Wake of the Water
(v0.9.0.0, 2025-11) **[inference]**; Forging the Path (v0.9.1, 2026-04)
explicitly required **no config changes** **[community]**.

## Ports & protocols

- **Current model: ONE UDP port.** `queryPort` (default **15637/udp**)
  carries game traffic *and* the Steam query. The old `gamePort` (15636)
  was **removed in Update #2 (2024-06)**; current builds reportedly delete
  a stale `gamePort` key from the json at boot. **[official-adjacent,
  well corroborated — jsknnr commit "Remove game port and rename query
  port to port", mornedhels template history]** Many older guides still
  say "open 15636 + 15637"; outdated.
- **The port speaks Steam A2S**: mornedhels polls player counts with
  `python-a2s` against `127.0.0.1:queryPort`. **[code]** This is the one
  native query surface the game has, and the roadmap's Phase 2 transport.
- 27015/udp appears in jsknnr's compose but nothing configures it —
  Steamworks default heartbeat; treat as optional. **[uncertain]**
- Discovery: Steam master server (in-game browser + Steam server list),
  standard Steam auth, no crossplay yet (planned around 1.0).
  **[community]**

## Runtime behavior

- **Graceful shutdown = SIGINT to `enshrouded_server.exe`.** A SIGTERM to
  the wine/proton wrapper is *not* reliably propagated — both docker
  images translate their stops into SIGINT on the exe (`pkill -2`),
  escalating INT → TERM → KILL on 60 s timeouts; recommended
  `stop_grace_period: 90s`. **[code]** flameagent signals SIGINT to the
  process group and defaults its grace to 120 s.
- **Saves on shutdown: yes** — "a new copy is created every 10 minutes or
  when shutting down the game". **[official via mirrors]** So a power
  stop is a clean save, the opposite of the Dragonwilds situation.
- **Autosave: every 10 minutes**, rotating copies; not configurable.
  **[official]**
- Exit codes: undocumented. Both images avoid relying on them — jsknnr's
  liveness check is "is the UDP port still bound" via `/proc/net/udp`.
  **[code]**
- Requirements: ~6 cores, **16 GB RAM floor** (idle ≈ 4.4 GB, grows with
  terrain edits), 30 GB SSD, no GPU. Updates need up to 2× game size in
  temp disk. **[official via mirrors + code]**

## Logs

**Rewritten 2026-08-15 from a real capture** (build `b466cef15`, game
version 1024233, a real client joining and leaving). Everything in this
section is now **[verified]** unless marked otherwise; the community
quotes it replaces are kept at the bottom because the difference is the
whole reason the rules table is versioned.

- The exe writes `<logDirectory>/enshrouded_server.log`, **and the same
  stream reaches stdout** — flameagent's supervisor ring captures it,
  Wine's own startup chatter included. No logfile tail is needed.
- **There is no timestamp or level prefix.** Lines begin directly with
  their component tag: `[os]`, `[app]`, `[online]`, `[server]`,
  `[Session]`, `[savedata]`, `[ecss]`. (The community-sourced
  `[I 00:00:14,325] …` shape is not what a current build emits.)
- Boot is thousands of lines: `[Server][Water] Added Water Dispenser: …`
  per body of water, `[guid_registry]` entries, the full task-system
  dump. An idle server then prints a session table every ~10 s (four
  lines) plus `[ecss] Stats:` every minute. **Consequence:** the agent's
  2000-line ring holds roughly 80 minutes of idle server, so anything
  that must not be missed has to be read from the ring promptly rather
  than reconstructed later.

Load-bearing lines, verbatim (ids synthetic here):

```
enshrouded_server(detached HEAD) - version b466cef15… (master)   build hash
Game Version (SVN): 1024233                                      the version number
Config Parsed: 'Z:/enshrouded/enshrouded_server.json'            config read, Wine path
[server] Game Settings 'Default'                                 preset in force, then a full dump
[savedata] Start 'Open Container' on container 3ad85aea          the save slot
[online] Server SteamId: 90291000105583638
[OnlineProviderSteam] 'Initialize' (up)!                         steam up
[Session] 'HostOnline' (up)!                                     READY — accepting joins
[online] Session accepted with peer (steamid:76561190000000001)
[online] Added peer 0(1) (steamid:76561190000000001)             JOIN — peer(machine) + id
[online] Client '76561190000000001' authenticated by steam
[server] Machine '1': Player '0(0)' logged in                    a player *handle*, not a name
[server] Player 'Ember' logged in with Permissions:              NAME, then the role's perms
[server] Start Saving … [server] Saved                           a world write, bracketed
[server] Remove Player 'Ember'
[online] Removed peer 0(1)                                       LEAVE
```

Three things this settles for the parser (`eslog`):

1. **The join line carries the SteamID64 itself**, so nothing has to be
   paired across lines to know who joined.
2. **Names are in the log** — `[server] Player '<name>' logged in with
   Permissions:` — which is why the A2S query is no longer the only route
   to them. The name line carries no peer index, so names attach FIFO to
   joins still awaiting one.
3. **`[server] Machine 'M': Player 'P(x)' logged in` is a trap**: its
   quoted value is a handle (`0(0)`), not a name. The `with Permissions`
   tail is what tells the two apart.

Also confirmed here: the permissions block printed at login lists exactly
the capabilities of the role group whose password the player used
(`CanAccessInventories`, `CanEditBase`, `CanEditWorld`, `CanExtendBase`,
`CanReceiveEXP` for a non-admin group) — a direct read-back that the
seeded `userGroups` are in force.

**Not yet seen:** a failed save (`[server] Failed to save` is still
community-sourced), a kick/ban, a second simultaneous joiner, and a
version-mismatch join rejection.

**Superseded community quotes** (jsknnr issue #16 and hosting KBs,
2024-era builds) — `[online] Session accepted with peer ( id … ).`,
`[online] Added Peer #0.`, `[online] Removed Peer #0.`, and "no player
name in the log". `eslog` RulesV1 was written from these and matched
nothing on a live server, which is exactly what the versioned rules table
exists to absorb.

- How the popular images derive state: mornedhels greps `'HostOnline'
  (up)!` for readiness and uses A2S for counts; jsknnr parses nothing and
  checks the UDP bind. **[code]**

## Query/admin surface

- **No RCON, no HTTP API, no server console, no chat commands.**
  Corroborated by every panel/docker implementation and still-open
  feature-request threads. **[strongly corroborated]**
- Admin model: `enshrouded_server.json` (edit + restart) plus the
  in-game player list for players whose group has `canKickBan` (kick /
  ban / unban UI; bans persist to `bannedAccounts`). **[official via
  mirrors]**
- Player counts for panels: A2S on the query port, or log parsing. No
  first-party API. A2S_PLAYER may return blank names **[uncertain]**.

## Saves

- Layout under `saveDirectory` (default `./savegame`): world files have
  **fixed hex names by creation slot** — world 1 = `3ad85aea` (hardcoded
  as *the* dedicated-server savefile in mornedhels' backup tooling),
  world 2 = `3bd85c7d`, world 3 = `38d857c4`. **[code + community]**
- **Rolling copies**: base file plus `<hex>-1` … `<hex>-10` (new copy
  every 10 min or on shutdown, overwriting the oldest ⇒ ~90 min of
  rollback depth).
- **`<hex>-index` is JSON**: `{"latest": N, "time": <ts>, "deleted":
  false}` — `latest` selects which copy loads (`0` = the bare hex file).
  Rollback = rewrite `latest`. **[code — mornedhels parses/writes exactly
  this]** Small `-info` sidecars carry world metadata (world name;
  format publicly unparsed). **[community]**
- World blob format: proprietary binary; **no community parser reads the
  world contents** (kfc-parser and kfc-tools are for the client's game
  files, not server saves). Plan on metadata from `-index`/`-info` only.
- **Player characters are not stored on the server** — each client keeps
  its own character file; the server save is world state only.
  **[community, well corroborated]**
- Backup-while-running: everyone copies the directory live; the risk
  window is the 10-minute save moment. mornedhels' careful variant zips
  the `latest`-pointed copy and synthesizes a fresh index. No official
  statement on crash-consistency. **[code + community]**
- Singleplayer → server migration: copy the local world file over the
  server's hex file and fix the index. **[community]**

## Version status (mid-2026)

- Early Access, current build **v0.9.1.2** (late June 2026).
  **[community]**
- **1.0 releases 2026-10-15** (PC + PS5; Xbox spring 2027). Expect config
  and networking churn at 1.0 (console + crossplay) — budget a recon
  refresh then. **[official]**
- Server-relevant update history: #2 Melodies of the Mire 2024-06
  (userGroups added, gamePort removed) · #3 Back to the Shroud 2024-07
  (gameSettings + presets) · #7 Wake of the Water 2025-11 (v0.9.0.0, new
  gameSettings keys) · #8 Forging the Path 2026-04 (v0.9.1, zero config
  changes).

## Verification ledger

What a real server has settled, and what is still resting on someone
else's report. First deployment: **2026-08-15**, TrueNAS, provisioned
through Ilmari, one client joined and played.

Confirmed 2026-08-15:

- [x] SteamCMD installs app 2278520 with the windows platform override,
      from flameagent's install job.
- [x] The exe boots under plain wine64 (no Proton) on the flameagent
      image, headless. Wine builds its prefix at
      `/enshrouded/.wineprefix` on first run, as configured.
- [x] The supervisor's stdout ring captures the game log — Wine's own
      messages included. **No logfile tail is needed**, which was the
      riskiest open assumption.
- [x] The seeded `enshrouded_server.json` is read and accepted:
      `Config Parsed: 'Z:/enshrouded/enshrouded_server.json'`, and the
      Wine `Z:` mapping of the install root works.
- [x] Role groups are in force: a joining player's logged permission list
      is exactly the non-admin group's capabilities, so the seeded
      passwords and `canKickBan` split work at the join screen.
- [x] `[Session] 'HostOnline' (up)!` appears verbatim — readiness
      detection is sound.
- [x] Join, name and leave lines — **but not in the shape the community
      quotes claimed**; see Logs. `eslog` RulesV2 is written from this
      capture, and player *names* turn out to be in the log after all.
- [x] The save slot is the hex-named container `3ad85aea`, as the
      community tooling assumes.
- [x] A world write is bracketed by `[server] Start Saving` …
      `[server] Saved` (the completion line was previously
      **[uncertain]**), and one fires when a player leaves.
- [x] `perkUpgradeRecyclingFactor` defaults to 0.5.
- [x] The server binds and is reachable on the single published UDP port;
      a client found and joined it.

Still open:

- [ ] **SIGINT stop**: that the graceful stop reaches the exe through
      Wine and produces a clean save-on-shutdown inside the 120 s grace.
      (Observed only that a *player leave* triggers a save.)
- [ ] **A2S from off-host** — the Phase 2 gate. Now worth less than
      planned, since names come from the log; its remaining value is
      authoritative presence, the real `slotCount`, and a liveness signal
      that doesn't depend on log inference.
- [ ] `bannedAccounts` element format (bare SteamID64 strings vs
      objects). No longer gates anything: the bans editor
      (`esconfig/bans.go`) reads whichever shape the file uses, writes new
      entries in that same shape, and defaults to bare strings only when
      the file is empty and has no convention to read. Still worth
      settling the first time a real ban lands — the console reports the
      answer as `objectShape` on `GET /bans`.
- [ ] Whether a **running** server overwrites a hand-edited
      `bannedAccounts`. The in-game ban UI writes this file, so it is
      plausible the game holds the list in memory and persists it on
      shutdown, clobbering an edit made mid-session. The Bans panel warns
      about this whenever the game is up; confirming it either way would
      let the warning go away (or become a hard refusal).
- [ ] Whether an **absent** per-group capability key in `userGroups`
      defaults true or false in the game. It matters only for
      hand-written configs: everything flameagent seeds, and everything
      the roles editor saves, writes all five explicitly. The editor
      reads an absent key as false, which is the assumption to check.
- [ ] A failed save (`[server] Failed to save`) is still community-sourced.
- [ ] A second *simultaneous* joiner, which is the case the FIFO name
      attachment in `eslog` is reasoned about but unproven against.
- [ ] What a version-mismatch join rejection looks like in the log (the
      Phase 4 update watcher would like to recognise it).
- [ ] Whether the save layout grows the `-1`…`-10` rolling copies and the
      `-index` pointer as documented — needs a few days of autosave
      rotation, and gates the Phase 3 reader.

Known gap, not a game fact: an idle server prints a session table every
~10 s, so the agent's 2000-line ring holds roughly **80 minutes**. If the
*console* restarts more than that after a player joined, their join line
has scrolled out and they will be missing from the roster until they
rejoin. (The game process can't outlive its agent, so an agent restart
can't strand a session.) A2S closes this properly; the session table's
`m#N` machine list is a cruder fallback if it ever bites.
