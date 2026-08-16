# Sampo (monorepo)

Unifies the three game consoles — palcon (Palworld), wildskeeper
(Dragonwilds), flametender (Enshrouded) — and ilmari, the host
provisioning service. **`docs/unification-plan.md` is the plan of
record; read it before structural work.** `docs/drift-ledger.md` holds
the per-file reconciliation decisions that Phase 2 executes.

Current state: **Phase 2 in progress**. `core/` is the extracted
console framework (root module `github.com/safwyls/sampo`), taken from
flametender's shared layer with the game-shaped hardcodes replaced by
seams: `game.ConfigCodec` on the Definition, `OfflineConfigWork` in
api/sched (banqueue generalized), `api.ProvisionProfile` for the
wizard/Ilmari adapter, neutral `agentctl.Query*` types, per-console
`DefaultID` and session-cookie name. The gate holds: core builds and
passes its full suite with only `game/gametest` registered. The four
old repos remain imported at repo-name prefixes and still build
independently — do not hand-sync code between them; that is what the
drift ledger is for. Phase 2 is complete: 2b restored the advisor and
landed seams 1/6; 2c extracted the agent kit — `core/agent` is the
full sidecar implementation parameterized on an `agent.Game` spec, and
the agent-backed api/agentctl tests run against it with a fake game.
Next: Phase 3, the flametender port (first console rebuilt on core,
gated on a real Enshrouded server).

Rules already in force (see the plan's "Structural rules"):

- **Core freeze**: no new shared-layer features inside the imported
  console trees. Bug fixes are allowed but must update their file's row
  in `docs/drift-ledger.md` the same day.
- **Dependency rules**, enforced by `scripts/checkbounds.sh` in CI:
  production code never imports `gametest`; ilmari never imports
  console or game code. Post-restructure rules (core ✗→ games, games
  ✗→ each other, dockerctl only under ilmari/) are listed there and
  activate in Phase 2.
- The old repos are frozen-then-archived as each console's port is
  verified against a real server (plan, Phases 3–5). Image names, env
  vars, ports, and volume layouts are frozen API — running deployments
  must survive on a tag repoint.

Tests: per module, `go test ./...` and `cd web && npm test`. CI runs
all modules plus the boundary check.

Workflow: when a branch is pushed and ready for review, open the PR
without asking — the maintainer has standing-approved PR creation
("always open the pr when appropriate", 2026-08-15).
