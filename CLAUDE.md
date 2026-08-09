For any UI/frontend task, always produce a written design plan (palette, type, layout, signature) and self-critique it against generic AI-design defaults before writing code — per the frontend-design skill.

# Wildskeeper (dwcon)

**Picking this up mid-flight? Read `docs/state-of-play.md` first** — it is
the handoff: what's done, what's verified against a real server, what is
still guessed, and what to do next.

A standalone Dragonwilds server console built on palcon's reusable base
(sibling repo; architecture kept structurally identical on purpose). One
game is registered: `internal/games/dragonwilds/` — client derived via the
palagent sidecar, `dwconfig` ini editor, `dwlog` log tracker, `dwsave`
world-save reader (SPUD header metadata, served at `/servers/{id}/world`). Frontend is
Wildskeeper throughout (design source: `mocks/dragonwilds-dashboard.html`;
theme tokens are the `wk.*` literals in `web/tailwind.config.js`, mirrored
onto shadcn semantic vars in `web/src/index.css`).

Read `docs/dragonwilds-recon.md` before touching parsers or capability
decisions. Its "Empirical findings" section is measured on a real server
and outranks the web-sourced sections above it — notably: the save format
is **SPUD, not GVAS**; the server does **not** save on shutdown (clean stop
~2 s, exit 143); `OwnerId` is not format-validated. A real client joined on
2026-08-09, so the join/leave log lines (`dwlog` RulesV1), player id shape,
and ban location (ini `KnownPlayerList`) are now verified — see the recon
doc's "Closed 2026-08-09" section. Steam app id 4019830 (dedicated server
tool), native Linux build, no RCON/REST/query — commands reach the game
through the **dwbridge** UE4SS mod (`tools/dwbridge`, Phase 4): `Save` works
end to end, the rest return `game.UnsupportedError` (HTTP 501) until the mod
implements them. See the recon doc's "Command surface" for the mapped game
functions.

Shared-layer tests use the test-only game in `internal/game/gametest`
(a REST-shaped client over httptest fakes) so they don't need a fake agent
and synthetic logs; production code must never import it.

The agent (`cmd/palagent`, name kept from palcon) supervises the game
directly: `./RSDragonwildsServer.sh -log -Port=7777`, publishing the
7777/7778 UDP pair the game binds. `PALAGENT_OWNER_ID` is effectively
required — the game refuses to start without an owner, so the agent seeds
`DedicatedServer.ini` with it when an install has none. Provisioning
(`internal/palagent/provisioner.go` + `internal/api/provision.go`) makes
that whole stack from the Raise-a-server wizard.

Tests: `go test ./...` and `cd web && npm test`. Production build:
`cd web && npm run build` then `go build ./cmd/dwcon` (embeds the bundle).
