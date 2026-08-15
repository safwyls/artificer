For any UI/frontend task, always produce a written design plan (palette, type, layout, signature) and self-critique it against generic AI-design defaults before writing code — per the frontend-design skill.

# Flamekeeper (flamekeeper)

**Picking this up mid-flight? Read `docs/state-of-play.md` first** — it is
the handoff: what's done, what is still community-sourced rather than
verified, and what to do next.

A standalone Enshrouded server console built on palcon's reusable base
(sibling repos palcon and wildskeeper; architecture kept structurally
identical on purpose so fixes travel). One game is registered:
`internal/games/enshrouded/` — client derived via the flameagent sidecar,
`esconfig` JSON editor for `enshrouded_server.json`, `eslog` log tracker.
Frontend is Flamekeeper throughout (design plan: `docs/design.md`; theme
tokens are the `fk.*` literals in `web/tailwind.config.js`, mirrored onto
shadcn semantic vars in `web/src/index.css`).

Read `docs/enshrouded-recon.md` before touching parsers or capability
decisions. **Nothing in it is verified against a real server yet** — its
verification ledger is the checklist, and facts marked [uncertain] are
not assumed anywhere in code. Key facts: Steam app 2278520, Windows-only
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

Tests: `go test ./...` and `cd web && npm test`. Production build:
`cd web && npm run build` then `go build ./cmd/flamekeeper` (embeds the
bundle).

Roadmap: `docs/roadmap.md` (Phase 2: A2S presence + names; Phase 3: save
index reader + rollback; Phase 5: the 1.0 churn wave, 2026-10-15).

Workflow: when a branch is pushed and ready for review, open the PR
without asking — the maintainer has standing-approved PR creation
("always open the pr when appropriate", 2026-08-15).
