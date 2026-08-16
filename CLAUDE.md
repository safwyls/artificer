# Sampo (monorepo)

Unifies the three game consoles — palcon (Palworld), wildskeeper
(Dragonwilds), flametender (Enshrouded) — and ilmari, the host
provisioning service. **`docs/unification-plan.md` is the plan of
record; read it before structural work.** `docs/drift-ledger.md` holds
the per-file reconciliation decisions that Phase 2 executes.

Current state: **Phase 1**. The four old repos are imported with full
history at repo-name prefixes (`palcon/`, `wildskeeper/`,
`flametender/`, `ilmari/`) and are still self-contained modules — each
keeps its own CLAUDE.md, docs, go.mod, and tests. The target layout
(`core/` extracted from flametender's shared layer, `games/{palworld,
dragonwilds,enshrouded}`, `web/core`, single go.mod) lands in Phase 2;
until then do not hand-sync code between the imported trees — that is
what the drift ledger and Phase 2 are for.

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
