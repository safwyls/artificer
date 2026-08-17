# Artificer (monorepo)

Unifies the three game consoles — palcon (Palworld), wildskeeper
(Dragonwilds), flametender (Enshrouded) — and anvil, the host
provisioning service. **`docs/unification-plan.md` is the plan of
record; read it before structural work.** `docs/drift-ledger.md` holds
the per-file reconciliation decisions that Phase 2 executes.

Current state: **Phase 2 in progress**. `core/` is the extracted
console framework (root module `github.com/safwyls/artificer`), taken from
flametender's shared layer with the game-shaped hardcodes replaced by
seams: `game.ConfigCodec` on the Definition, `OfflineConfigWork` in
api/sched (banqueue generalized), `api.ProvisionProfile` for the
wizard/Anvil adapter, neutral `agentctl.Query*` types, per-console
`DefaultID` and session-cookie name. The gate holds: core builds and
passes its full suite with only `game/gametest` registered. The four
old repos remain imported at repo-name prefixes and still build
independently — do not hand-sync code between them; that is what the
drift ledger is for. Phase 2 is complete (core framework + agent kit,
game-blind, green with only the test game). Phase 3 (flametender
rebuilt on core) is code-complete: `games/enshrouded/` (client, codec,
eslog/esquery, banqueue behind the offline-work seam, esapi routes via
seam 5, esagent's agent.Game spec), thin `cmd/flametender` +
`cmd/flameagent`, `web/flametender`, and monorepo image publishing
under the old ghcr names (`:main` channel; `:latest` flips after the
gate). **Phase 3 is complete** (gate passed
2026-08-16 against the live Enshrouded server; legacy tree deleted;
:latest publishes from the monorepo). **Phase 4 (wildskeeper on core)
is code-complete**: `games/dragonwilds/` (client, dwconfig codec,
dwlog/dwsave, dwbridge both halves, dwapi contributed routes, dwagent's
spec with the launch chooser), the kit grew selectable profiles and
health/launch extras, core's wizard grew port-run arity and required
owner ids (seam 4's pair form, asserted in provision_pair tests),
`cmd/wildskeeper` + `cmd/wkagent`, `web/wildskeeper`, `tools/`
(dwbridge + kit + shim), and image publishing on the :main channel.
**Phase 4 is complete** (gate passed 2026-08-16 against the live
Dragonwilds server; legacy tree deleted; :latest publishes from the
monorepo). **Phase 5 (palcon on core) is code-complete**:
`games/palworld/` (REST+RCON client, palconfig codec, palsave reader,
palapi contributed routes — the deep-game surface: pals, guilds,
inventory, storage, achievements — the save-derived Roster, the
advisor prompt, palagent's agent.Game spec), core's wizard grew named
TCP admin transports (seam 4's REST/RCON trio form, asserted in
provision_trio tests) and the runtime identity gained ServerDesc,
`cmd/palcon` (embedding the advisor docs) + `cmd/palagent`,
`web/palcon` (Cloudflare Access SSO ported in), and image publishing
on the :main channel. Gate: docs/palcon-port-verification.md — note
the one behavior change: the legacy provisioner-mode agent is retired;
provisioning is Anvil-only (`PROVISIONER_URL` → `ANVIL_URL`; the doc
has the migration).

Rules already in force (see the plan's "Structural rules"):

- **Core freeze**: no new shared-layer features inside the imported
  console trees. Bug fixes are allowed but must update their file's row
  in `docs/drift-ledger.md` the same day.
- **Dependency rules**, enforced by `scripts/checkbounds.sh` in CI:
  production code never imports `gametest`; anvil never imports
  console or game code. Post-restructure rules (core ✗→ games, games
  ✗→ each other, dockerctl only under anvil/) are listed there and
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
