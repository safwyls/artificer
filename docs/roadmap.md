# Roadmap

One roadmap, per-game sections. Work that benefits every console lives in
"Shared" and should be built once in `core/` rather than three times.

Merged from the three consoles' separate roadmaps at the Phase 6 doc
consolidation (2026-08-18). Where an item was on more than one list, it
moved to Shared — which is most of the interesting ones, and the argument
for the monorepo restated as a table of contents.

## Shared

1. **Update watcher.** Poll the Steam app build id; when a game update
   ships, notify over Discord and offer or schedule stop → update → start.
   Every agent already has the SteamCMD verb; nothing polls. This matters
   most where joins are version-gated (Enshrouded, Dragonwilds), because a
   stale server is unjoinable rather than merely out of date. It also
   supplies the missing half of version surfacing: "you are behind"
   instead of only "you are on this".
2. ~~Backup restore.~~ **The verb landed with save sync** (2026-08-21):
   `PUT /v1/files/save` on every agent — stopped-game gate, `If-Match`
   precondition, verify-then-swap with one `.bak` — and a Restore button
   on wildskeeper's backups page. Remaining: the same button on
   flametender's and palcon's pages (the API is already there), and
   Enshrouded's index-aware rollback stays its own Phase 3 item.
3. **TLS between console and agent.** Plain HTTP with a bearer token on a
   trusted network today. A pinned self-signed certificate fingerprint
   stored alongside the token is the intended shape; the verb surface does
   not change. Prerequisite for genuinely cross-host deployments.
4. **Resource telemetry.** Container memory and CPU surfaced per server,
   via Anvil's fleet view or agent-side sampling, so "this server is
   getting heavy" is visible before the OOM. The surface to hang it on
   landed 2026-08-18: every console's Host dashboard already renders
   Anvil's managed fleet (containers with lifecycle state, images with
   their disk cost); a per-row memory/CPU column is the missing half.
5. **Auth beyond bootstrap admin.** Users, roles and Cloudflare Access SSO
   all exist; OIDC or proxy-auth would let a household SSO into it without
   Cloudflare.
6. **Multi-host.** Anvil is already one service per machine, which is the
   hard half. What is missing: per-host registrations instead of one
   `ANVIL_URL`, and host labels in the UI.
7. **Save sync, the remaining phases.** Implemented 2026-08-21 as the
   standalone **reliquary** service plus the game-blind Artificer
   Companion (`docs/save-sync-architecture.md`, "The option-B pivot").
   Still open: **phase 0 recon** — the player-hosted Dragonwilds save
   location/format is unverified, so discovery marks its candidates as
   guesses and the player confirms — plus the Discord interactions
   endpoint (phase 5), the Witchspire catalog entry once its save
   location is known (phase 6). Its UI is no longer the gap it was: the
   React rebuild landed 2026-08-21 (`docs/reliquary-ui-rebuild.md`),
   with the app shell, the world detail page and the admin panels the
   single embedded page could not hold.

## Palworld (palcon)

The longest-running console; its surface is the most complete of the three.

1. **Save-derived views keep pace with the game.** New pals, passives and
   items arrive with each update, and the vendored game data
   (`games/palworld/docs/vendored-game-data.md`) is what the paldex,
   breeding and team-builder pages read.
2. **Advisor cost controls.** Context caps and the model picker landed; a
   per-user budget and a cheaper default model are the next asks.

## RuneScape Dragonwilds (wildskeeper)

1. **dwbridge command surface.** The UE4SS mod is the game's only command
   channel. `Save` works end to end; every other command returns a 501
   naming the mod. Each command the mod implements turns a 501 into a
   working button with **no console-side work** — the capability probe
   (`game.CommandProber`) already drives the UI from what the bridge
   reports.
2. **World map.** `dwsave` now yields each character's last-saved
   position (the Adventurers page shows them as coordinates), so a map
   page — player last-positions, points of interest — is the showpiece.
   Needs coordinate-system recon against the real game first: the
   positions are UE units with an unmapped origin.
3. **Chat capture, if it exists in the log.** Still an open recon question.
   If chat lines appear, `dwlog` grows a rule and the UI gets a read-only
   chat panel; there is no send path.
4. **Mod management via the agent.** The Wine/UE4SS stack makes mod files
   part of the deployment; a small manifest the agent applies (dwbridge
   version pinning included) turns "the mod setup" from a document into a
   button.

### Non-goals, so they don't creep back

- **Kick and broadcast via the admin RPC.** Tested against a live player:
  silently does nothing, and the working-looking alternatives wedge the Lua
  VM. The recon doc's "Why the command tier stops at `save`" is the
  tombstone. Do not re-attempt without new information — a game update
  changing the surface *is* new information.
- **A query or RCON client.** The game has neither; everything is derived,
  and the derivations are verified. No speculative protocol code.

## Enshrouded (flametender)

Phase 2 completed 2026-08-16: moderation surface, A2S presence, ready state.

### Phase 3 — saves, rollback, world lifecycle

The biggest single gap in any of the three consoles.

1. **Save index reader** (`essave`, a `savecache.Source`): parse the
   `<hex>-index` (the `latest` pointer and save time) and the `-info`
   sidecar (world name) — metadata only, since the world blob has no public
   parser. Wire the `/world` endpoint back up and give the World-saves page
   real facts: which copy is live, when it was written, how far back the
   rollback window goes.
2. **Rollback**: restore a rolling copy or a console backup — stop, place
   the copy, write a fresh index with `latest: 0`, start. The UI must say
   what will be lost (up to 10 minutes since the chosen copy).
3. **World import/export**: singleplayer → server migration and back, which
   the community does by hand today.
4. **Pre-update and pre-restart snapshots.** Enshrouded saves on shutdown,
   so this is cheap insurance rather than a correctness need.

### Phase 4 — running it for months

1. **Scheduler honesty pass.** Restart warnings cannot reach players
   in-game — no broadcast channel exists — so the Discord notice is the
   first-class warning path and the UI copy should say so.
2. **Log deep links.** Join/leave history enriched with A2S-resolved names
   and Steam profile links off SteamID64s.

### Phase 5 — the 1.0 wave (2026-10-15)

Enshrouded 1.0 lands on PC and PS5 with crossplay planned. Assume churn:
the config schema, networking, possibly the log vocabulary (`eslog` rules
are versioned tables for exactly this), possibly a real query or admin
surface. Watch for the last one — a first-party API would obsolete chunks
of Phase 2 and should win if it appears. Budget a recon refresh against the
1.0 server before touching code.

## Anvil

1. **The §F guards are closed** (2026-08-18). What remains is convergence
   rather than parity: nothing in the ledger is outstanding.
2. **Per-console rate limiting or quotas.** One console cannot currently be
   stopped from filling a host with containers. Ownership is enforced;
   volume is not.
3. ~~Drop the `ilmari` image alias.~~ **Done 2026-08-18** — the host was
   repointed to `ghcr.io/safwyls/anvil` and the alias removed. The stale
   `ilmari` tag is left published but unmaintained; nothing pulls it, and
   the last thing it points at needs `ANVIL_*` to start.

## Structural, someday

- **A fourth game.** `docs/adding-a-game.md` is the checklist, and the
  honest test of whether the seams are real. The three current games each
  contributed one: a config codec, an offline-work queue, a save layout.
- **Public status page per console**, beyond the existing single-server
  view.

## Standing constraints

These are not roadmap items; they are the rules the roadmap is built on.

- **Anvil owns container placement.** No console grows Docker create rights
  back. "Adding a fourth console needs no code in Anvil" is a promise this
  repo relies on — keep it true from both sides.
- **The agent is the only transport** to a supervised game server; nothing
  may bypass it.
- **Every capability claim in the UI is probe-derived or honestly
  501-reasoned.** A button that cannot work is a bug even when it renders.
