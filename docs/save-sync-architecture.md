# Save sync — architecture review and proposal

**Status: proposal, for review.** `docs/unification-plan.md` remains the
plan of record; nothing here is committed until the maintainer says so.
Written 2026-08-20 against the 2026-08-19 brainstorm ("P2P Save-Sync
Extension"), after a survey of the seams it would land on.

## The idea, restated in this repo's vocabulary

Peer-hosted games make one player the host. The brainstorm adds
checkout/check-in semantics over a canonical, versioned save held on the
operator's own storage: one player holds the save at a time, plays,
checks it back in; the next host checks out the latest version. Two
front ends (Discord, web) over one API; transfers never touch Discord's
CDN.

The brainstorm frames this as "extend the provisioner/agent/manager
architecture". That vocabulary predates the unification. What actually
exists: **consoles** (per-game managers built on `core/`), the **sidecar
agent** (one per *dedicated game server*, sharing its install volume),
and **anvil** (the host service holding the Docker rights). The
restatement matters because the brainstorm's central actor — "the agent,
already part of Artificer's per-host architecture" — is not where this
feature's saves live. That is hole 1.

## Holes

Ordered by how much each one bends the design.

### 1. The save lives where no agent runs

The sidecar agent runs in a container next to a dedicated server on a
managed host. A peer-hosted session's save lives on a **player's
Windows machine** — no container, no agent, no Docker, no anvil. "The
agent packages the save and pushes to Messier" assumes an agent at the
save; there isn't one.

The actual in-repo precedent is `wkcompanion`
(`games/dragonwilds/docs/companion.md`): a single Go tray binary on the
player's machine, auto-detecting the save directory, pushing to a
console over HTTPS with a token in the path, local-only until
configured. That is the chassis for the player-side half of save sync —
not the sidecar agent.

The sidecar agent still matters, but for a different and better reason:
if a **dedicated server can also be a holder** (check the world out of
the wildskeeper server to host it locally for a weekend, check it back
in), the server-side half needs a save *upload* verb on the agent —
which is exactly the "backup restore verb" already sitting as Shared
item 2 in `docs/roadmap.md`. Two roadmap entries collapsing into one
mechanism is the strongest signal in this review.

### 2. No lock, lease, or conflict primitive exists anywhere yet

A survey of the whole repo finds no lease, no generation counter, no
optimistic-concurrency column, no `If-Match`. Every existing "busy"
guard is in-memory and per-process: the backup runner's
`running map[int64]bool`, the agent's one-job-at-a-time lock. The only
versioning primitive at all is the agent save bundle's ETag
(`core/agent/files.go`) — sha256 over name|size|mtime per file, used
for conditional *download* only.

So the lock is not a table to add; it is the design center, and it must
be **database-backed** (the consoles' SQLite store), because it has to
survive console restarts and outlive any single process. This also
means: one console instance per game, which is true today — but the
multi-host roadmap item would expose a DB-backed-but-single-writer
assumption, so write it down as one.

### 3. `save_locks` as a separate table is a drift bug waiting to happen

The brainstorm derives `save_locks` from the active session and calls it
the single source of truth. Two representations of one fact always
drift. Drop the table: **the lock *is* the unique active session row**,
enforced by a partial unique index (`UNIQUE ON sessions(world_id) WHERE
status = 'active'`). Discord and the portal both read sessions; there is
nothing else to disagree with.

### 4. Auto-expiry without version lineage is a data-loss machine

The brainstorm leans auto-expire and proposes "last successful check-in
wins". Combined, they produce exactly the failure the feature exists to
prevent: the holder's session runs long, the lock expires, the next
player checks out and plays, the original holder finally checks in —
and *wins*, silently discarding the second player's session.

The fix is lineage, and it makes auto-expiry safe instead of merely
tolerable: every checkout records the version it delivered
(`base_version`); every check-in records its parent. A check-in whose
base is still the head **fast-forwards** the head. A check-in whose
base is *not* the head is stored as a **branch** — kept, flagged,
never the head, never pruned until resolved — and resolution is a human
picking a version in the portal. "Last check-in wins" becomes "last
check-in *based on the current head* wins; everything else is kept and
flagged", which is what the brainstorm's own open question 2 was
reaching for.

With that rule, expiry policy stops being scary: expire with a warning
ping, but expiry is only ever a **permission for the next person to
claim**, never an overwrite license. Takeover is an explicit act
("claim from expired holder") that notifies the previous holder, not a
silent lapse.

### 5. The crash-before-check-in loss window

Holder plays four hours, machine dies, nothing was checked in: the
canonical copy is a whole session stale, and the only current copy is on
a possibly-dead machine. Mitigation: the player client opportunistically
pushes **checkpoint versions** while the session is active (the game
writes saves as it runs; the client watches mtimes exactly as
`wkcompanion`'s 15-second scan already does, debounced by a settle
window). Checkpoints are session-scoped, pruned aggressively after a
clean check-in, and turn "lost the session" into "lost the last few
minutes". Per-world toggle, because it costs upstream bandwidth on
residential lines.

### 6. rsync/SFTP/MinIO to player machines is a credential problem

Every listed transport means handing friends durable server credentials
(SSH keys, S3 keys) with far more authority than "move this one game's
save". The transfer should ride **HTTPS through the save-sync API
itself**, authorized by the same per-player token as everything else,
using the bundle format the repo already has (the agent's tar stream
with PAX mtimes and the name|size|mtime ETag; `agentctl.SyncSave`'s
extract-to-tmp-then-rename swap is the download side, already
hardened against traversal and zip-bombs).

Where the bytes rest is a separate, smaller question. Simplest honest
answer: the version store is files under the console's data volume,
exactly like `core/backup` keeps its zips, and *the operator* decides
that volume lives on Messier (run the console there, or mount NAS
storage into it). Put the store behind a two-method interface so a
MinIO/S3 backend can arrive later without touching the model. Building
S3 integration in v1 buys nothing the mount doesn't.

### 7. Cloudflare Access will eat the player client's requests

Hit for real on 2026-08-19 with wkcompanion: a console behind Access
answers a token-path request with an HTML login page at HTTP 200. The
companion now refuses to count any 200 as delivered unless the body is
the console's own JSON ack. The save-sync client inherits that rule
from day one, and the deployment doc inherits the same fix (bypass or
service-auth policy for `/api/public/*`, or a LAN/direct address —
`docs/cloudflare-access.md`). New wrinkle for uploads: Cloudflare's
proxy caps request bodies (100 MB on free plans), so the per-title max
size the brainstorm already proposes is a real constraint to enforce,
not just a quota.

### 8. Torn saves

The game writes its save while running; packaging mid-write archives
garbage that *looks* like a backup — the exact lesson
`game.SaveLayout.VerifyWorld` encodes (Palworld's magic-bytes check),
and why `core/savecache` refuses to parse files younger than a settle
window. The client therefore: refuses to package while the game process
is running (per-game process name in the sync spec), waits out a settle
window on mtimes, and verifies the world file before upload. The server
verifies again on receipt (two-phase: upload to staging, verify, then
commit the version). Same on checkout: refuse to unpack while the game
runs, atomic-swap the directory, and keep one local backup of what was
replaced.

### 9. Identity is thinner than the feature needs

The companion token is per-*server* and shared — fine for "players may
push character sheets", useless for "who holds the save". Save sync
needs per-player identity for the lock, the audit trail, and
revocation. Holders should be **console users** (auth, roles, and audit
already exist in core); each gets a personal sync token for the native
client (same token-in-path tier as the companion, but minted per user,
revocable per user), and a Discord user id on their profile so the bot
can map commands to people.

And the Discord bot is genuinely new infrastructure: `core/notify` is
outbound webhooks only — there is no inbound command path of any kind
today. (Options in the proposal below.)

### 10. `game_titles` config rows duplicate the game seam

Save path patterns, verify rules, quiesce rules, transfer strategy —
that is *behavior*, and behavior in this repo lives in code seams on the
game module (`game.SaveLayout`, `agent.Game`), not in config rows. The
per-game differences the brainstorm worries about in open question 3
are real, and the answer the repo already has is "each game contributes
a spec". What legitimately stays as data per *world*: retention counts,
max size, lease duration, checkpoint toggle.

The sharper form of this question: **Witchspire is not in the tree.**
"Config rows make new games cheap" is the standalone-service argument;
"a game is a module with specs" is this repo's argument. That choice is
the next section.

## What already exists to build on

| Need | Existing piece |
|---|---|
| Bundle format + change detection | agent tar stream, PAX mtimes, name-size-mtime ETag (`core/agent/files.go`) |
| Safe unpack on the receiving side | `agentctl.SyncSave` — tmp-dir extract, atomic rename, traversal/size bounds |
| Versioned archive store, pruning | `core/backup` Runner (zips, keep-N, tmp+rename) — the shape, if not the code |
| "Game isn't holding the file" window | `OfflineConfigWork` decomposed restart (stop → apply → start) for server-side check-in |
| Save shape + torn-save guard | `game.SaveLayout` (`WorldFile`, `VerifyWorld`), `savecache` settle window |
| Player-machine client chassis | `wkcompanion`: tray app, save-dir autodetect, token push, CF-Access ack sniffing |
| Users, roles, audit, sessions | `core/api` auth; three trust tiers already named (session, token-in-path, bearer) |
| Notifications | `core/notify` — add event kinds, reuse delivery |
| Server-side restore verb | doesn't exist — and is already roadmap Shared item 2 |

## Where it fits structurally

Three options were considered; the constraint set is the standing rules
(anvil gains nothing, core never imports a game, capability claims are
honest).

**Option A — a core capability, hosted by each game's console
(recommended).** A `core/savesync` package owns the model (worlds,
sessions, versions), the store schema, the HTTP surface, and the
version store; a game contributes a small sync spec (player-side save
location, process name for quiesce, verify — mostly re-using its
`SaveLayout`). Wildskeeper hosts Dragonwilds' save sync next to the
dedicated-server console it already is. Auth, users, notify, UI shell,
deployment: all inherited. Core's gate holds: the whole capability must
build and test with only `gametest` registered.

The honest wrinkle: consoles today are server-shaped — every existing
resource hangs off a server row with a container behind it. A shared
world is a **new resource, not a server**: it has no lifecycle, no
agent, no ports. It may *optionally reference* a server row (that is
the dedicated-server-as-holder feature). Modeling it as a fake server
to fit the existing furniture would poison the watchdog, metrics, and
power surfaces; a sibling resource is more code but no lies.

**Option B — a standalone vault service** (a peer of the consoles; in
this repo's register it would want a name like *reliquary*).
Game-agnostic by configuration, one deployment for all titles,
including titles with no console. Rejected for v1: it duplicates auth,
users, notify, and UI outside core, its "games are config rows"
premise is hole 10, and it creates a second thing for a friend group to
be logged into. It remains the escape hatch if save-sync-only titles
multiply; option A's model, store schema, and client kit all survive
that move, so nothing is wasted.

**Option C — put it in anvil.** Rejected without ceremony: anvil holds
Docker rights and references nothing above it. "Adding a fourth console
must need no code in Anvil" is a promise both sides keep. Save sync
touches anvil not at all.

**And Witchspire?** Under option A, a title enters the tree the way
`docs/adding-a-game.md` says a fourth game does — except almost every
seam is legitimately nil: no dedicated server means no `game.Client`
work beyond honest 501s, no provision profile, no agent. What it
contributes is a Definition, a sync spec, and a themed shell. That is a
real cost (a console per game), and it is also the roadmap's own
"honest test of whether the seams are real". If that cost is
unacceptable, option B is the answer — decide this before phase 3, not
after.

## Proposed shape (option A)

### Data model

Three tables in the console store, migrations in the usual sequence:

- **`sync_worlds`** — the shared world: name, game (implicit — one game
  per console), optional linked server id (dedicated-server holder),
  settings (lease hours, max bytes, keep-N, checkpoint toggle),
  `head_version` id.
- **`sync_sessions`** — one row per checkout: world, holder (user id),
  `base_version`, `checked_out_at`, `expires_at`, status
  (`active | returned | expired | reclaimed | released`), and who
  released it when an admin did. The partial unique index on
  (`world_id`) `WHERE status='active'` *is* the lock.
- **`sync_versions`** — append-only: world, session, parent version,
  kind (`checkin | checkpoint | import | resolve`), archive path,
  size, sha256, uploader, created_at, `conflict` flag. Head is a
  pointer on the world, moved only by fast-forward check-ins and
  explicit resolves; both moves are audited.

### Session lifecycle

```
checkout ──► active ──► returned            (check-in, base == head: fast-forward)
                │
                ├─ renew ──► active          (expires_at extended; same session)
                ├─ expires_at passes ──► still active, now *claimable*
                │                        (warning pinged before; check-in still
                │                         works normally until someone claims)
                ├─ claimant takes over ──► reclaimed (previous holder notified)
                └─ admin force-release ─► released (audited)
```

"Expired" is deliberately not a stored state: it is the active session
past its `expires_at`, which changes what *others* may do (claim it),
not what the holder may do. Two verbs keep the common cases free of
ceremony:

- **Renew** — "still holding, extend." The same person hosting again
  tomorrow costs zero transfers; checkout is "I am the host this
  stretch", not per-play-session, and checkpoints keep the canonical
  copy fresh across a long hold.
- **Claim-next** — one queued claimant per world, set while the world
  is held. The moment the holder checks in (or the hold becomes
  claimable and the claimant confirms), the claimant's checkout happens
  automatically and they are pinged "the world is yours" — the live
  handoff needs nobody to notice the world went free.

Rules that make it safe: server-authoritative time only; expiry is
permission to claim, never an overwrite; and **only the active session
may move the head**, and only when its base is the head. A check-in
from any ended session — reclaimed, released, or racing a claim — is
stored, flagged `conflict`, exempt from pruning, and resolved only by
an explicit human pick. (Letting a late check-in fast-forward just
because the head hadn't moved yet would move it under the new holder's
feet and guarantee *their* honest check-in flags as the conflict.)

### Transfer protocol

Player client ↔ console, all HTTPS, per-player token, JSON acks only
(the CF-Access rule):

- `GET  /status` — worlds, holder, head version, freshness.
- `POST /worlds/{id}/checkout` — acquires the session or answers 409
  with who holds it and until when; response carries the download URL
  and `base_version`.
- `POST /worlds/{id}/claim` / `DELETE …/claim` — queue as (or step
  down as) the world's next holder.
- `POST /sessions/{id}/renew` — extend the active hold.
- `GET  /worlds/{id}/versions/{v}/bundle` — the tar bundle, ETag'd.
- `POST /sessions/{id}/checkin` — opens a staged upload; the client
  PUTs the bundle (single request in v1 — these saves are megabytes,
  and the per-world max caps it; chunked/resumable is a later layer
  behind the same staging), then commits with the sha256. The console
  verifies (size, hash, `VerifyWorld`) in staging before anything
  touches the version store or the head. A dead upload leaves a stale
  staging file and an active session — nothing else.
- `POST /sessions/{id}/checkpoint` — same staging path, `checkpoint`
  kind, never moves the head.

Dedicated-server check-in/checkout reuses the same model with the agent
as the transfer peer: checkout-from-server = stop (scheduler-warned) →
agent bundle download → version committed → server marked non-holder;
check-in-to-server = the new agent verb below, inside the same
stop→apply→start window `OfflineConfigWork` already decomposed restarts
for.

### The one agent change: `PUT /v1/files/save`

The agent's file surface is deliberately two fixed verbs with no
client paths ("fixed verbs, not an exec agent" —
`docs/sidecar-agent.md`), and this proposal keeps that posture while
widening it by exactly one verb: save upload, to the fixed save
location only, gated three ways — server stopped (supervisor check),
`If-Match` on the current bundle ETag (the repo's first write
precondition, so a save that changed since the console last looked is
never blindly replaced), and verify-before-swap using the same tmp-dir
+ atomic-rename dance as `agentctl.SyncSave`, with one `.bak` of what
it replaced. This verb is roadmap Shared item 2 ("backup restore")
delivered — the console's backup page gets its restore button from the
same mechanism save sync uses.

### The player client: the Artificer Companion

`wkcompanion` becomes the **Artificer Companion** (`cmd/companion`,
shipping as `artificer-companion.exe`): its scope is no longer "the
Dragonwilds character relay" but "the Artificer app that runs on a
player's machine", and save sync is its second job. Deliberately *one
app for all games*, not a kit spawning one tray binary per game — a
`cmd` binary may import game modules (the checkbounds rules constrain
core and agents, not commands), so each game contributes a client-side
spec (save locations, process name for quiesce, verify) to the same
binary, exactly the way consoles get game modules contributed.
Dragonwilds' spec carries the existing character relay; the tray, the
token plumbing, and the ack-sniffing HTTP layer are shared.

The rename is a frozen-API event and travels as a migration, not an
edit: `WKCOMPANION_EXE` stays honored (with the retired-name warning
`core/config` uses for such cases) beside the new `COMPANION_EXE`, and
the new binary reads its config from the old `wkcompanion/` config
directory when the new one doesn't exist yet — players' pasted tokens
must survive the upgrade.

A Dragonwilds bonus worth stating: characters are client-side (recon,
2026-08-19), so the world hand-off carries no character data — each
player brings their own. That makes Dragonwilds the *easy* case, not
the general one. The player-hosted world save's location is a phase 0
recon item; until it is verified the companion asks for the directory
instead of guessing one.

### Front ends

The **console UI is the portal** — versions, holder, history, rollback,
force-release land as console pages with the auth and UI shell that
already exist. Build this first; it is also the conflict-resolution
surface everything above depends on.

**Discord** is new inbound infrastructure either way; two shapes:

- *Interactions endpoint* (recommended): Discord POSTs slash commands
  to an HTTPS URL; requests are Ed25519-signed by Discord, so the
  endpoint self-authenticates the same way the token tier does — it
  mounts beside `/api/public` and needs the same Access-bypass note.
  No persistent process, no gateway connection, fits the console's
  HTTP shape.
- *Gateway bot*: a long-lived websocket process. More capable
  (presence, DMs on expiry warnings), more moving parts. Not needed
  for checkout/check-in/status.

Outbound stays `core/notify` with new event kinds: checked out,
checked in, expiry warning, conflict flagged, claim over an expired
hold.

## The brainstorm's open questions, answered

1. **Lock timeout** — auto-expire with a warning ping, *and* lineage
   (hole 4) so expiry is claim-permission, not overwrite-permission.
   The forced-explicit-release alternative recreates the hostage
   problem the feature exists to kill.
2. **Conflict/orphan** — two-phase staged check-in (a dead client
   never corrupts anything), checkpoint pushes shrink the crash
   window, and branch-on-stale-base replaces "last check-in wins".
   Stale locks are surfaced and claimable, exactly as proposed.
3. **Per-game differences** — real, and answered by per-game code
   specs, not config rows (hole 10). Recon before code, per house
   rule: the player-hosted save's location and format are *unverified*
   for every game including Dragonwilds.
4. **Retention** — keep-N check-ins plus an age horizon per world
   (the backup runner's shape); checkpoints pruned at clean check-in
   plus a grace day; conflict-flagged versions never pruned until
   resolved.
5. **Structural fit** — option A: a new core capability and store
   resource, one new agent verb, a game-side sync spec, zero anvil
   changes. The hidden coupling to watch is the one named in hole 2:
   the DB-backed lock assumes one console instance per game.

## Implementation plan

Each phase lands green on the full gate (`go build/vet/test`,
`checkbounds`, `checkdocs`, web tests) and is independently useful.

- **Phase 0 — recon** (the house rule; nothing below starts on
  guesses). Player-hosted Dragonwilds: where the hosted world save
  lives, whether it is the same SPUD layout `dwsave` parses, what the
  client writes on exit, real sizes. Witchspire: everything (Proton
  prefix save location, format, sizes) — and the option A/B decision
  for it. Write both up as recon docs in the usual places.
- **Phase 1 — the model in core.** `core/savesync`: migrations,
  session state machine, version lineage, staging + verify, prune.
  Exercised entirely with `gametest` (which gains a trivial sync
  spec) — this is the core-gate proof. No UI, no client yet.
- **Phase 2 — the HTTP surface.** Console routes (admin + per-player
  token tier), notify events, the world/holder/versions console page
  in wildskeeper's web app. At the end of this phase the feature is
  usable browser-only: check out, download, upload, check in.
- **Phase 3 — the player client.** `wkcompanion` becomes the Artificer
  Companion (`cmd/companion`, with the config-dir and env-var
  migrations above) and grows checkout/check-in/checkpoint for
  Dragonwilds. The CF-Access ack rule and torn-save guards live here.
- **Phase 4 — the server as a holder.** The agent `PUT
  /v1/files/save` verb with its three gates; the console flows for
  checkout-from-server and check-in-to-server inside the offline-work
  window. Ship the backup-restore button on the same verb (roadmap
  Shared 2 closes).
- **Phase 5 — Discord.** The interactions endpoint, Discord-id →
  user mapping, slash commands for status/checkout/checkin, expiry
  pings via notify.
- **Phase 6 — the second game.** Witchspire per the phase 0 decision:
  either the minimal in-tree module (the fourth-game seam test) or
  the option B split. Deliberately last — one working game first.

## Non-goals, confirmed and extended

- No save merging; version history plus a human pick is the whole
  conflict story (unchanged from the brainstorm).
- Discord is orchestration only; bytes never transit it (unchanged).
- No S3/MinIO client in v1 — a storage interface and a filesystem
  implementation; the NAS is a mount decision.
- No auto-provisioned dedicated server on checkout ("optionally
  launches the server process" in the brainstorm): starting servers is
  the console/anvil flow that already exists, and the linked-server
  hand-off (phase 4) is the composition point. Revisit only if a real
  need appears.
- No multi-console/multi-host lock coordination; single console per
  game is an explicit assumption, written where it can be found.
