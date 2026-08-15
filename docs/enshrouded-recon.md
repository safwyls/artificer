# Enshrouded dedicated server — recon

Compiled 2026-08-15 from web sources and the two dominant community docker
projects' tracked code. **Nothing in this document has been verified
against a real server yet** — that is Phase 1's first deployment's job,
and the "Verification ledger" at the bottom tracks what a live server has
since confirmed. Until a fact is checked off there, treat it exactly as
its confidence marker says.

Confidence markers: **[official]** = Keen docs (enshrouded.zendesk.com) or
first-party, directly or via mirrors; **[code]** = read directly from the
docker projects' tracked source ([jsknnr/enshrouded-server], [mornedhels/
enshrouded-server]); **[community]** = hosting guides / wiki / forums,
plausible but not first-party; **[uncertain]** = conflicting or
unverifiable.

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

floats 0.25–4ish (default 1 unless noted): `playerHealthFactor`,
`playerManaFactor`, `playerStaminaFactor`, `playerBodyHeatFactor`
(0.5–2), `playerDivingTimeFactor` (0.5–2), `foodBuffDurationFactor`
(0.5–2), `shroudTimeFactor` (0.5–2), `miningDamageFactor` (0.5–2),
`plantGrowthSpeedFactor` (0.25–2), `resourceDropStackAmountFactor`
(0.25–2), `factoryProductionSpeedFactor` (0.25–2),
`perkUpgradeRecyclingFactor` (0–1; default 0.5 vs 0.1 **[uncertain]**),
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

- `<logDirectory>/enshrouded_server.log` (latest); older logs auto-archive
  into `<logDirectory>/backup/`. The exe writes the file, not stdout —
  jsknnr symlinks the log to the container's stdout. **[official via
  mirrors + code]** Under flameagent the supervisor captures the process
  stdout ring; whether the wine-run exe also mirrors to stdout is
  **[unverified]** — if it doesn't, the agent will need to tail the log
  file into its ring (a small, planned adjustment; see the ledger).
- Line format: `[<I|W|E> HH:MM:SS,mmm] [component] message`; the
  timestamp is time-since-start.
- Exact lines (verbatim from user-posted logs in jsknnr issue #16 and
  hosting KBs — **[community, verbatim quotes]**; the eslog RulesV1 table
  is written against these):
  - first line: `[enshrouded] Create logfile`
  - version banner: `enshrouded_server(detached HEAD) - version <git-sha>
    (master)` — a commit hash, **not** the marketing version.
  - Steam up: `[OnlineProviderSteam] 'Initialize' (up)!`
  - **ready**: `[Session] 'HostOnline' (up)!` then `[Session] finished
    transition from 'Lobby' to 'Host_Online' …`
  - **join**: `[online] Session accepted with peer ( id 76561198… ).`
    then `[online] Added Peer #0.` — **SteamID64 only, no player name.**
  - **leave**: `[online] Session failed for peer #0 with error 4.` then
    `[online] Removed Peer #0.`
  - save: `[server] Start Saving`; on failure `[server] Failed to save`.
    The success-completion line is **[uncertain]** — verify on a real
    server.
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

Facts the code currently *rests on* that a first real deployment must
confirm. Check each off (with date and evidence) as it lands; a fact that
fails moves its dependent code, not just this list.

- [ ] SteamCMD installs app 2278520 with the windows platform override,
      from flameagent's install job.
- [ ] The exe boots under plain wine64 (no Proton) on the flameagent
      image, headless, and creates `logs/` + `savegame/`.
- [ ] The supervisor's stdout ring actually captures the game log under
      Wine — or the agent needs to tail `logs/enshrouded_server.log`
      instead (eslog is transport-agnostic either way).
- [ ] `[Session] 'HostOnline' (up)!` appears verbatim; a real client join
      emits the accepted/Added Peer pair with the SteamID64; leave emits
      Removed Peer. (eslog RulesV1 is written from community captures.)
- [ ] SIGINT to the process group reaches the exe and produces a clean
      save-on-shutdown within the 120 s grace.
- [ ] The seeded `enshrouded_server.json` is accepted by the game (it
      boots, binds the seeded queryPort, both role passwords work at the
      join screen).
- [ ] The game honors `queryPort` enforcement on restart, and only the
      one UDP port needs publishing.
- [ ] Save layout matches: hex-named world file + `-index` JSON under
      `savegame/`; the save bundle and backups capture a restorable set.
- [ ] `bannedAccounts` element format (bare SteamID64 strings vs
      objects) — needed before the Phase 2 bans editor.
- [ ] Whether A2S answers on the query port from off-host (Phase 2 gate).
