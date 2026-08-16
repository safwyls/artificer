# Drift ledger

> **Execution status.** Phase 2a (core bootstrap) landed: all take-F
> rows are in `core/`, with seams 2, 3, 4 (single-port form), 7, and 9
> implemented; §A and §B take-F rows are done unless named below.
> Phase 2b landed the merge-backs: the advisor restored to core from P
> (prompt/docs/console-name injected — `advisor.Prompt`, `Server.DocsFS`;
> the Palworld prompt text and browser tools stay with games/palworld),
> config regained the legacy-provisioner and advisor-key fields, seam 6
> (`api.RosterSource`) and seam 1 (`game.SaveLayout`) exist with F's
> behavior as documented defaults, and P's admin-bypass/per-switch
> visibility rules are asserted core-shaped. Phase 2c landed the agent kit:
> `core/agent` now holds the full sidecar implementation (supervision,
> jobs, steam and file verbs, launch profiles) parameterized on an
> `agent.Game` spec — stop signal, grace, app id, config path/validator,
> save dir, profile assembly, PrepareRuntime hook, and game routes —
> and the five deferred agent-backed test files are back, running
> against the kit with a fake game. **Phase 2 is complete.** Phase 3
> (flametender on core) is code-complete: games/enshrouded carries the
> codec, banqueue (seam 2's implementation), esapi contributed routes
> (seam 5's first user), esagent's agent.Game spec, and the returned
> test suites (moderation, ban queue end-to-end on the real kit+spec,
> enshrouded api, esquery, the enforcement suite, the app-id agreement
> test). **Gate passed 2026-08-16** against the live Enshrouded server; the legacy flametender tree is deleted and :latest publishes from the monorepo. Open: the §F
> guards not yet checked off (palworld/dragonwilds rows land with their
> ports; Ilmari parity checks with Phase 6 wiring).

Per-file reconciliation decisions for Phase 2 (core extraction), per
`docs/unification-plan.md`. This is the working checklist: Phase 2
executes it row by row, and any bug fix landing in an old repo during
the freeze must update its row here the same day.

**Method.** Three-way diff of `internal/` across palcon (P),
wildskeeper (W), flametender (F), *normalized* — module paths
(`safwyls/<repo>`), agent names (`palagent`/`wkagent`/`flameagent`),
and console brand strings replaced with placeholders before diffing, so
rows reflect real drift, not renames. Dates: P's shared layer is oldest
(mostly 2026-07-22..08-08), W's next (08-09..15), F's newest
(08-15..16), which is why F is the baseline trunk.

**Scorecard.** 127 shared-layer Go files across the three consoles:
40 have no true drift (identical after normalization), 48 are genuinely
drifted, 39 exist in only some consoles. Verdict of the file-by-file
review: **flametender's baseline loses nothing from wildskeeper** (every
W improvement is already in F, except the Dragonwilds-specific bits that
relocate) **and exactly one whole feature from palcon** (the advisor —
see §"Lost feature"). Everything else P has that F lacks is
game-shaped and relocates to `games/palworld`.

Decision vocabulary:

- **take-F** — F's file becomes core verbatim (modulo naming
  parameterization); nothing elsewhere to carry.
- **take-F-merge-P / -merge-W** — F is the base; the named console has
  hunks that must be carried in (see notes).
- **three-way** — real merge, notes say how.
- **relocate:`<dest>`** — file (or the named part) is game-shaped and
  moves to a game module.
- **promote-core** — exists in only one/two consoles but is
  game-agnostic; enters core so all consoles gain it.
- **retire** — superseded; delete after the guard in its notes is met.
- **restore** — feature F lost; re-enters the baseline from P/W.

---

## New core seams this ledger forces

The recurring pattern across rows: shared logic is fine, but a game
fact is hardcoded where a game hook belongs. Phase 2 must introduce
these seams (each row below references them):

1. **Save-layout hook** (backup, agentfiles, collector): which file is
   the world (Palworld `Level.sav` / Dragonwilds newest `*.sav` /
   Enshrouded newest non-sidecar hex blob), what counts as a sidecar,
   archive membership (`IncludeInArchive`, default all — `.sav`-only
   for Palworld), and save verification (size floor default; palworld's
   magic-bytes check — the only mid-write guard for a non-atomic
   writer — survives as its implementation).
2. **Offline-config-work hook** (sched, power, banqueue): a generic
   "pending edits that must apply between stop and start" interface
   (`Pending(ctx,srv) bool` / `Apply(ctx,srv)`). Core owns the queue
   store and the stop→apply→start orchestration on both power paths;
   `games/enshrouded` owns the `bannedAccounts` editor that is today's
   only implementation. Other games register a no-op.
3. **Config codec registry** (api/config): W/F's `configCodec` seam
   stays; the codec instances (`palconfig`/`dwconfig`/`esconfig`)
   register from game modules; `codecFor` becomes a registry lookup;
   the config *filename* comes from the codec (also fixes `agentfiles`
   and two stale F comments still saying `DedicatedServer.ini`).
4. **Provisioning profile per game** (provision, provisioner, stacks):
   port arity is the key variance — P claims 4 named ports (game,
   RCON, REST, agent), W a UDP pair + agent, F one UDP + agent. Core
   asks the game module for: contiguous-port count, extra request
   fields + validation (`ownerId`+`worldName` required for W,
   `joinPassword` for F, `serverDesc`/REST/RCON trio for P), the
   `AGENT_*` env lines, mount path, image family. F's `controlChars`
   validation is a security fix P lacks — core, unconditional.
5. **Game-contributed routes** (server.go): pals/guilds/inventory/
   storage/achievements (palworld), world + bridge/install + PUT
   launch (dragonwilds), roles + bans (enshrouded) mount via the
   game-routes hook; core keeps everything else including
   `/capabilities`, rotate-admin-password, `/login/cloudflare`.
6. **Roster reader capability** (visibility): P's palsave-backed
   `roster()` becomes a game capability with an "unavailable" default —
   F's stub error must NOT become core behavior or Palworld's working
   roster dies.
7. **A2S query split** (agentctl/query): transport half (8s budget,
   404-tolerance for old agents, partial-failure `PlayersError`) is
   core; the payload decoder is game-provided. Palworld also answers
   A2S — promote, don't confine to enshrouded.
8. **Agent kit parameters** (core/agent): stop signal (SIGTERM vs
   Enshrouded's SIGINT-to-save), grace period (30s vs 120s), launcher
   command, app id, platform override (`UpdateArgsFor` ordering
   comment is load-bearing — keep), optional config validator on PUT
   (F's `esconfig.Validate` guards against the game regenerating an
   open server; generalize as a hook).
9. **Console identity as config**: session cookie name
   (`<console>_session`), registry `DefaultID` (can't stay a package
   const with three games registered — make it settable or required),
   advisor console-name parameter, `AGENT.DefaultGamePort` indirection
   everywhere literals appear.

---

## Lost feature: the advisor (restore from P)

F has no advisor but still ships its schema (migrations
`0020_app_settings.sql`, `0021_user_advisor_keys.sql` — dead tables
applied on every F install). Restore into the baseline:

| File | Decision | Notes |
|---|---|---|
| `internal/store/settings.go` (P,W identical) | **restore** | Advisor credential store over the schema F already carries. Depends only on secretbox + db, both in F. |
| `internal/advisor/{advisor,claude,gemini}.go`, `internal/api/advisor.go`, `advisor_test.go` (P,W) | **promote-core, source = P** | P and W are normalized-identical, but W's 08-10 "retire Palworld naming" pass was a partial sed — its prompt still says Palworld, still registers `search_palcon_docs`. P (08-08) is self-consistent. Core keeps transport/loop + both providers; console name becomes a constructor param; docs tool renamed neutral. The Palworld payload (system prompt body, `palworld_wiki`, breeding/inheritance tools) → `games/palworld` as an injected tool set + prompt fragment. |
| `config.go` advisor keys (`AnthropicAPIKey`, `GeminiAPIKey`) | **restore** | See config.go row. |
| Web: `AdvisorPanel/Overlay` components (P, W) | **promote-core** | Follows the backend; port-phase work. |

---

## A. No true drift — 40 files, take-F verbatim

Identical across all three after normalization; naming
parameterization only:

`api/activity.go` `api/activity_test.go` `api/automation.go`
`api/automation_edges_test.go` `api/automation_test.go`
`api/handlers_test.go` `api/metrics_history.go` `api/misc_test.go`
`api/public.go` `api/ratelimit.go` `api/servers.go` `api/spa.go`
`api/steamcache.go` `api/steamupdate.go` `api/users.go`
`backup/sweep_test.go` `collector/collector.go` `collector/events.go`
`collector/events_test.go` `collector/saves_test.go`
`crypto/secretbox.go` `db/db.go` `dockerctl/dockerctl.go`
`dockerctl/dockerctl_test.go` `savecache/savecache.go`
`savecache/savecache_test.go` `sched/sched_test.go`
`store/activity.go` `store/activity_test.go` `store/automation.go`
`store/automation_test.go` `store/metrics.go` `store/migrate_test.go`
`store/store.go` `store/store_test.go` `store/visibility.go`
`store/visibility_test.go` `watchdog/sweep_test.go`
`watchdog/watchdog.go` `watchdog/watchdog_test.go`

## B. Drifted files — decisions

All paths under `internal/`. Seam numbers reference §"New core seams".

| File | Decision | Notes |
|---|---|---|
| `backup/backup.go` | take-F-merge-P | F's fix (extensionless world discovery, sidecars, `LastFailure` recording) is trunk. Seam 1 carries P's magic-bytes verify + `.sav`-only archive filter as palworld's implementations — F's "every regular file" default would sweep strays into Palworld archives. |
| `backup/backup_test.go` | three-way | F's structure; fixture becomes a table over the seam-1 hook, one case per game. Restore P's `stray.txt`-excluded assertion under the palworld case only. |
| `backup/edges_test.go` | take-F-merge-P | Keep F's `TestAFailedSnapshotRecordsWhy` + sidecar-never-wins in core. `TestSnapshotsARealEnshroudedSaveDirectory` becomes table-driven with a real-layout case per game. P's `TestVerifySavMagic`/`TestLevelSavPath` → `games/palworld`. |
| `sched/sched.go` | take-F | Strict superset of W and P. Seam 2 replaces the `banqueue.Queue` param; `saveBeforeRestart` doc de-Dragonwildsed. Signature ripple to all callers. |
| `sched/sweep_test.go` | take-F | Keep the `statuses`/`answer` fake-game plumbing (tests 501-vs-fault). No-op seam-2 default in the helper. |
| `collector/saves.go` | take-F | W/F's nil-reader guard is required for parser-less games. Drop both leaked doc sentences (P's "pals pages", F's "Dragonwilds has no reader yet"). |
| `steamcmd/steamcmd.go` | take-F | `UpdateArgsFor` platform override (W-origin). Ordering comment is load-bearing (after `+login` it's silently ignored). Platform string stays a games-module decision — no `"windows"` default in core. |
| `api/backups.go` | take-F | Adds `lastFailure` to status payload. Front-end must consume it at port time or the fix stays invisible. |
| `api/auth.go` | take-F | F's cfaccess rewrite drops nothing from P==W. Cookie name → seam 9. Verify store signatures at promotion. |
| `api/config.go` | take-F | Seam 3. F's `editConfigFile`/rotate-password stay core; hardcoded `enshrouded_server.json` in the ErrNotExist branch → `codec.filename`. Palworld's nil rotate = existing 501 path. |
| `config/config.go` | take-F-merge-P | F's Access fields + helpers promote verbatim. Carry back: `ProvisionerURL/Token` (P/W still run legacy agents until their ports) and the advisor keys (see Lost feature). |
| `config/config_test.go` | take-F-merge-P | Restore provisioner env assertions beside Ilmari's. Add missing coverage: `normalizeTeamDomain`, `splitEmails`. |
| `api/server.go` | three-way | Seams 5, 9. Core keeps the union of shared route groups + F's SSO routes + W/F's `/capabilities`; `Provisioner` takes the interface form; `palReader` leaves the `New()` signature; banqueue wiring becomes seam-2-conditional. |
| `api/actions.go` | take-F | W/F's `writeClientError` (501 vs 502 split) + capability probe. Needs `UnsupportedError`/`CommandProber` from game/client.go (union = F). |
| `api/visibility.go` | take-F-merge-P | Seam 6 — P's working palsave roster must survive as the palworld capability; F's stub error is the default, not the behavior. |
| `api/visibility_test.go` | take-F-merge-P | P's three tests exist nowhere else (admin bypass, per-switch hiding, all-views-off closes shared endpoint). Game-agnostic assertions rewritten against a core endpoint; pals specifics → `games/palworld`. Promote P's `docs/visibility.md` to core docs. |
| `api/middleware.go` | take-F | SSO context plumbing; additive. |
| `notify/notify.go` | take-F | `SaveOutcome` + honest `RestartingNow(…, save)`. P's scheduler passes `SaveDone`. "No command bridge" phrasing → game-supplied reason string. |
| `notify/notify_test.go` | take-F | Moves with notify.go. |
| `api/review_fixes_test.go` | take-F | Interface-level `fakeProvisioner` beats the wire-level agentctl stub post-Ilmari. Verify `agentctl.ErrNotFound/ErrRejected` exist in P/W or backport the sentinel refactor. |
| `api/agentfiles_test.go` | three-way | Shared sync/push/round-trip body parameterized over per-game `seedAgentWorld` fixtures (which relocate to game modules). Keep P's `TestSaveEndpointsUnconfigured` re-pointed at a core endpoint. |
| `agentfiles/agentfiles.go` | take-F | Seam 3: config filename from the codec, not hardcoded. |
| `api/provision.go` | take-F-merge-P | Seam 4. F's shape + `controlChars` gate (security fix P lacks). P's REST/RCON port trio + 4-way distinctness → palworld profile; W's pair-stride/neighbour-reservation → dragonwilds profile; `ownerId` required-400 preserved. |
| `api/provision_test.go` | take-F | F's `fakeProvisioner` harness is the only one that survives docker retirement. Keep F's negative leak-assertions, generalized to "reject any other game's env keys". Carry W's arity collision cases + P's duplicate-ports row into game profiles. |
| `api/provisioner.go` | take-F | Ilmari-only form; W's legacy dual-impl assertion is exactly what retires. Env-building block → seam 4. P's concrete `*agentctl.Client` field must become the interface at its convergence. |
| `api/provisioner_ilmari_test.go` | take-F | Keep F's phantom-second-port negative check parameterized on declared arity; preserve W's port-pair reasoning in the dragonwilds test. |
| `api/power.go` | take-F | F-only ban-queue orchestration (stop→Apply→start on both agent and docker paths) is a real lost-write-race fix; both branches survive (docker *power* path outlives docker *provisioning*). Behind seam 2. |
| `api/power_agent_test.go` | take-F | `supervisorServerWithInstall` + explicit `GameCommand` de-hardcodes launcher names; `trap INT TERM` is the superset. |
| `api/power_docker_test.go` | take-F | Signature follows api.New (docker provisioning param removed). |
| `agentctl/agentctl.go` | take-F | P==F. W's four dwbridge methods (with their deliberate 30s/60s budgets) relocate to `games/dragonwilds` as an extension embedding the core client. |
| `agentctl/client_test.go` | take-F | Deleted tests died with provisioner-mode. Guard: re-assert `TestMissingIsDistinctFromRefused`'s invariant (ErrNotFound vs ErrRejected) against the Ilmari client before this lands. |
| `agentctl/agentctl_test.go` | take-F | Pure game constants → fixtures. W's copy has a stale assertion message F already fixed. |
| `agentctl/extract_test.go`, `agentctl/files.go` | take-F | P lacks the mtime-preservation work (W-origin, in F). Fix F's stale `DedicatedServer.ini` comments while touching. |
| `agentctl/power.go` | take-F | F keeps only the wire vocabulary + `RecreateAgent` (15-min Wine-image budget, W-origin, already in F). Carry the Destroy-abort failure-mode reasoning to the Ilmari client docs. |
| `dockerctl/client_test.go` | take-F | Deletion-follows-deletion; dockerctl.go itself is identical everywhere. Guard: `TestContainerRemoveKeepsTheVolume`'s promise (world survives removal) moves to an Ilmari destroy test before this file's deletion is accepted. |
| `game/client.go` | take-F | Strict nesting P⊂W⊂F — union is F verbatim (`Readiness` + `UnsupportedError` + `CommandProber` + agent fields on `Conn`). Rewrite stale palworld-referencing doc comments. |
| `game/registry.go` | take-F | Union is F (adds `FeatureSaves`/`FeatureLogs`; ten feature keys total). Seam 9: `DefaultID` cannot stay a package const. |
| `store/servers.go` | take-F | W/F's normalize-onto-struct fix (response must agree with the row written). Generalize the ini-name comment. |
| `store/users.go`, `store/gameclient.go` | take-F | Comment fix; `Conn` gains agent fields per client.go union. |
| `api/api_test.go` | take-F | api.New signature + cookie name (seam 9). |
| `api/actions_test.go`, `collector/sample_test.go`, `sched/agent_precedence_test.go` | take-F | `gametest.ID` on fixtures; agent_precedence's banqueue arg becomes the seam-2 no-op default. |
| `api/game_test.go` | relocate | Per-game `CanonicalUID` cases → each game module's tests. |
| `api/steamcache_test.go`, `api/steamupdate_test.go`, `steamcmd/steamcmd_test.go` | take-F | App-id/launcher fixtures parameterize per game. |

## C. Partial presence — decisions

| File(s) | In | Decision | Notes |
|---|---|---|---|
| `cfaccess/cfaccess.go` + `cfaccess_test.go`, `api/cfaccess_test.go` | F | promote-core | Cleanest promotion in the set; zero game coupling. Adds `golang-jwt/jwt/v5` dep. `TestSSOAccountsCannotUseThePasswordForm` is load-bearing. Carry `docs/cloudflare-access.md` (audience check mandatory; password login stays as break-glass). |
| `api/agentimage.go` | F | take-F (core) | W's launch.go minus the game-specific chooser; rationale is game-independent. P gains `RecreateAgent` for free; wire its route at P's convergence. |
| `agentctl/query.go` | F | promote-core (split) | Seam 7. |
| `api/roles.go`, `api/moderation_test.go` | F | relocate:games/enshrouded | Bound to `esconfig` groups + per-capability passwords. Two policies lift to core tests: credential-bearing config behind PermSettings; audit records ids, never secrets. |
| `store/bans.go`, `banqueue/banqueue.go`, `api/banqueue_test.go` | F | generalize (seam 2) | Core owns the pending-edit queue + orchestration; enshrouded owns the `bannedAccounts` editor. Schema migration conditional/harmless when unused. |
| `api/enshrouded_test.go`, `api/esquery_test.go` | F | relocate:games/enshrouded | |
| `api/pals.go`, `pals_internal_test.go`, `pals_save_test.go`, `storage_internal_test.go` | P | relocate:games/palworld | Via seam 5 — the first user of game-contributed routes. |
| `advisor/*`, `api/advisor.go`, `store/settings.go` | P,W | promote-core (source P) / restore | See §Lost feature. |
| `rcon/{rcon.go,rcon_test.go,rcontest/}` | P,W | relocate:games/palworld | W's copy is confirmed vestigial (zero importers) — drop. Single-consumer rule says games/palworld, not core; revisit only if a second RCON game is planned. |
| `dockerctl/provision.go`, `provision_test.go` | P,W | retire | Guard: W's `InspectSpec`+`Networks` logic (skip dynamic publishes; exclude default bridge) is the engine behind agent recreate — confirm Ilmari's recreate path reproduces both edge cases before deletion, or image swaps silently drop networks. P's copy deletes outright. |
| `api/launch.go` | W | split | `handleGetLaunch`/`handleRecreateAgent` already exist in F's agentimage.go (core — take F's post-retirement wording). `handleSetLaunch` + `handleInstallBridge` (+ `SetLaunchProfile`/`InstallBridgeKit` clients) → games/dragonwilds. |
| `api/world.go`, `world_test.go`, `dragonwilds_test.go` | W | relocate:games/dragonwilds | dwsave world endpoint. |
| `api/actions_internal_test.go`, `api/stack_test.go`, `api/servers_create_test.go`, `sched/save_before_restart_test.go`, `game/gametest/gametest.go` | W,F | take-F | gametest and save_before_restart are zero-drift. stack_test: keep F's scaffolding, table over games (carry W's `ownerId`/`worldName` + 3-port case). servers_create_test: read expectations from the registered Definition, run per game. |
| `internal/ilmari/ilmari.go` | W,F | promote-core as `core/ilmariclient` | W and F are the same file (one comment differs). **Gap found**: no client methods for `GET /v1/containers` / `GET /v1/ports` — the fleet view that matters with three consoles on one Ilmari. Add both; also assert `APIVersion` at the seam. |

## D. Game modules and agents — wholesale moves

`internal/games/palworld/**` (incl. palsave, palconfig, vendored web
data), `internal/games/dragonwilds/**` (dwconfig, dwlog, dwsave),
`internal/games/enshrouded/**` (esconfig, eslog, esquery) move to
`games/<game>/` unchanged in their port phases. `tools/dwbridge` → `tools/`.

Agent kit (`internal/{palagent,wkagent,flameagent}` → `core/agent` +
`games/<game>/agent`): newest kit is F's. Extract verbatim: `jobs.go`
(198 lines, zero drift across all three), `diskfree_*.go`,
`proc_other.go`, and the common HTTP skeleton (healthz, steam verbs,
jobs, files, power routes + auth middleware). Extract parameterized:
`proc_unix.go` (stop signal, seam 8), `appid_test.go` pattern (values
per game). Stay game-shaped: `files.go` (save-dir discovery, config
rel-path, content type, F's validate-on-write → seam 8 hook),
`supervisor.go` (lifecycle skeleton shared; signal/grace/args/password
enforcement per game). Relocate: W's `bridge*.go` + `/v1/bridge/*` →
games/dragonwilds; F's `query*.go` + `/v1/query` → games/enshrouded.
Retire: P and W's in-agent `provisioner.go` (+ W's `spec.go`) — keep
F's `provision_types.go` vocabulary, env set per game.

## E. Web layer (summary — detail at each port phase)

W and F share the component set (game UIs already in per-game subdirs
of `components/` and `pages/`); their 20 differing shared components are
the same reconcile-to-F exercise as §B. P predates the convention: ~25
flat game components (Pal*, Boss*, map, save UI) + 1.1 MB vendored data
move into its game dir at its port; `AdvisorPanel/Overlay` promote with
the advisor. Theme mechanism (tokens → shadcn semantic vars) is already
uniform; `web/core` extraction happens in Phase 3 with flametender as
first mover.

## F. Must-not-lose guards

Checked off only when the protection exists in the monorepo:

- [ ] Palworld magic-bytes save verification (mid-write guard) — seam 1.
- [ ] Palworld `.sav`-only archive membership (`stray.txt` exclusion test).
- [ ] P's three visibility tests (admin bypass, per-switch, all-off).
- [ ] P's working palsave roster (seam 6 — F's stub must not win).
- [ ] W's port-pair provisioning logic + "pair swallows agent port" case.
- [ ] `TestMissingIsDistinctFromRefused` invariant re-asserted vs Ilmari client.
- [ ] `TestContainerRemoveKeepsTheVolume` promise moved to an Ilmari destroy test. (Live-verified at the Phase 3 gate — destroy left the world dir — but the test guard is still owed.)
- [ ] W's `InspectSpec` network/port edge cases verified in Ilmari recreate.
- [x] Advisor restored (P source) + F's dead migrations get a live reader (2b).
- [ ] Ilmari client gains `Containers()`/`Ports()` + APIVersion assertion.
- [x] `lastFailure` consumed by the UI — live-verified at the Phase 3 gate.
- [ ] Palcon env migration documented at its port: `PROVISIONER_URL/TOKEN` →
      `ILMARI_URL/TOKEN` (frozen-API concern; accept both for one release).
