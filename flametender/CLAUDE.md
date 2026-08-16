For any UI/frontend task, always produce a written design plan (palette, type, layout, signature) and self-critique it against generic AI-design defaults before writing code — per the frontend-design skill.

# Flametender (flametender)

**Picking this up mid-flight? Read `docs/state-of-play.md` first** — it is
the handoff: what's done, what is still community-sourced rather than
verified, and what to do next.

A standalone Enshrouded server console built on palcon's reusable base
(sibling repos palcon and wildskeeper; architecture kept structurally
identical on purpose so fixes travel). One game is registered:
`internal/games/enshrouded/` — client derived via the flameagent sidecar,
`esconfig` JSON editor for `enshrouded_server.json`, `eslog` log tracker.
Frontend is Flametender throughout (design plan: `docs/design.md`; theme
tokens are the `fk.*` literals in `web/tailwind.config.js`, mirrored onto
shadcn semantic vars in `web/src/index.css`).

Read `docs/enshrouded-recon.md` before touching parsers or capability
decisions. Its **verification ledger** is the live checklist: much is now
confirmed against a real server (the log vocabulary, readiness marker and
config schema since 2026-08-15), and the rows still marked open are
genuinely open — facts marked [uncertain] are not assumed anywhere in
code. Key facts: Steam app 2278520, Windows-only
binary run under Wine (no Linux build exists), ONE UDP port
(queryPort 15637 = game + Steam A2S), config seeded before first boot
because the game's own generated default is an open server, saves on
shutdown + 10-minute autosave, graceful stop is SIGINT, no RCON/API —
every command 501s with a reason saying where the ability actually lives.

Provisioning is **Ilmari only** (github.com/safwyls/ilmari, the shared
host service): `internal/ilmari` is the client, `api.IlmariProvisioner`
the adapter holding all game-shaped provisioning knowledge (FLAMEAGENT_*
env, single UDP port + agent 8811, image family, `/enshrouded` mount).
This console must never grow Docker rights or a provisioner mode back —
that was deliberately deleted in the transplant from wildskeeper.

Shared-layer tests use the test-only game in `internal/game/gametest`
(a REST-shaped client over httptest fakes) so they don't need a fake
agent and synthetic logs; production code must never import it.

The agent (`cmd/flameagent`) supervises the game directly: SteamCMD
installs the Windows depot, then `wine64 enshrouded_server.exe` with
WINEPREFIX inside the install volume. Before every start it seeds or
enforces `enshrouded_server.json` (name, queryPort, role-group passwords
by *capability*, never by group name). Stop sends SIGINT to the process
group — the game saves the world on the way down — with a 120 s default
grace before SIGKILL.

Auth is username/password by default, plus optional Cloudflare Access SSO
(`internal/cfaccess` verifies the assertion; `api.handleCloudflareLogin`
creates accounts on first sign-in with no permissions).
`docs/cloudflare-access.md` is the contract — in particular why the
audience check is mandatory and why password login stays.

Auth is username/password by default, plus optional Cloudflare Access SSO
(`internal/cfaccess` verifies the assertion; `api.handleCloudflareLogin`
creates accounts on first sign-in with no permissions, and
`CF_ACCESS_ADMIN_EMAILS` is the lockout rescue). `docs/cloudflare-access.md`
is the contract — in particular why the audience check is mandatory and
why password login stays.

Saves are extensionless hex blobs (`3ad85aea`, rolling copies `-1`…`-10`,
plus `-index`/`-info` JSON sidecars) — never match on an extension.
`internal/backup` archives *every* regular file under `savegame/`,
deliberately the same set the agent bundles (`flameagent.listSaveFiles`);
when one changes the other must. Detached work (snapshots) records its
outcome (`Runner.LastFailure`) so a failure isn't invisible to the UI.

Tests: `go test ./...` and `cd web && npm test`. Production build:
`cd web && npm run build` then `go build ./cmd/flametender` (embeds the
bundle).

`bannedAccounts` has two writers — this console and the running game,
whose in-game ban UI maintains the same array and rewrites it on
shutdown. Never write that key while the game is up: queue the edit
(`pending_bans`) and let `internal/banqueue` apply it during the restart,
between the stop and the start. Both restart paths the console drives run
the two halves themselves when work is queued.

Two sources describe a live server and they are not interchangeable.
The Steam query (`esquery`, run **agent-side** via `GET /v1/query`
against `127.0.0.1:queryPort`) owns the present: player count, the real
`slotCount`, the running build, the game's own name. `eslog` owns
identity and history — who those players are, and the `HostOnline`
readiness marker. A silent query falls back to log-derived; it never
blanks the page. Readiness is three-valued on purpose ("" = can't tell,
because the marker scrolls out of the ~80-minute ring), and the query
answering is *not* proof of readiness — game and query share one port.

Roadmap: `docs/roadmap.md` (Phase 2 complete 2026-08-16 — moderation
surface, A2S presence, ready state; the Steam-build-id half of version
surfacing moved to Phase 4's update watcher. Phase 3: save index reader
+ rollback; Phase 5: the 1.0 churn wave, 2026-10-15).

Workflow: when a branch is pushed and ready for review, open the PR
without asking — the maintainer has standing-approved PR creation
("always open the pr when appropriate", 2026-08-15).
