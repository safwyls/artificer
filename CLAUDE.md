# Artificer (monorepo)

Unifies the three game consoles — palcon (Palworld), wildskeeper
(Dragonwilds), flametender (Enshrouded) — and anvil, the host
provisioning service. **`docs/unification-plan.md` is the plan of
record; read it before structural work.** `docs/drift-ledger.md` holds
the per-file reconciliation decisions the port phases executed.

Current state: **the unification is done.** All three consoles are built
on `core/` and verified against real servers — flametender and
wildskeeper 2026-08-16, palcon 2026-08-17. No legacy tree remains; every
image publishes from here, and CI builds the monorepo plus anvil.

`core/` is the console framework, taken from flametender's shared layer
with every game-shaped hardcode replaced by a seam: `game.ConfigCodec`
and `game.SaveLayout` on the Definition, `OfflineConfigWork` in
api/sched (banqueue generalized), `api.ProvisionProfile` for the
wizard/Anvil adapter, `api.RosterSource`, game-contributed routes,
neutral `agentctl.Query*` types, the `agent.Game` spec parameterizing
the sidecar kit, and per-console `DefaultID` + session-cookie name. The
gate that keeps it honest: core builds and passes its full suite with
only `game/gametest` registered.

What each game module carries:

- `games/palworld` — REST+RCON client, palconfig, palsave, palapi (the
  pals/guilds/inventory/storage/achievements surface), the save-derived
  Roster, the advisor prompt, palagent's spec. Seam 4's REST/RCON trio.
- `games/dragonwilds` — client, dwconfig, dwlog/dwsave, both halves of
  dwbridge, dwapi, dwagent's spec with the launch chooser (native vs
  Wine). Seam 4's UDP pair + required owner id.
- `games/enshrouded` — client, esconfig, eslog/esquery, banqueue behind
  the offline-work seam, esapi, esagent's spec. Seam 4's single port.

Provisioning is Anvil-only across all three; the legacy
provisioner-mode agent is retired (`PROVISIONER_URL` → `ANVIL_URL` —
`docs/palcon-port-verification.md` has the migration).

**Next: Phase 6** — Anvil convergence (the four §F guards still open in
the drift ledger: the missing-vs-refused invariant, a destroy test for
the keep-the-volume promise, InspectSpec edge cases on recreate, and
`Containers()`/`Ports()` + an APIVersion assertion on the client), plus
doc consolidation. One known gap outside that: the GitHub repo is still
named `sampo`. Anvil's image now builds here (`docker-anvil.yml`),
publishing `anvil` plus `ilmari` as a migration channel for hosts pinned
to the old name.

Rules already in force (see the plan's "Structural rules"):

- **Dependency rules**, enforced by `scripts/checkbounds.sh` in CI:
  core never imports a game, games never import each other, an agent
  never imports its console-side game package, production code never
  imports `gametest`, and anvil references nothing above it.
- **Frozen API**: image names, env vars, ports and volume layouts are
  what running deployments depend on. A rename is a migration with a
  documented path, not an edit (see the ilmari→anvil label compat in
  `anvil/internal/host/client.go`).
- A game that cannot support a feature answers with a reason — a 501
  naming where the ability actually lives — rather than hiding it.

Tests: `go build ./... && go vet ./... && go test ./...`,
`./scripts/checkbounds.sh`, and `cd web/<console> && npm test`. The
anvil module has its own suite. Save-backed palworld tests need
`palworld-save-tools` importable by python3; they skip without it.

Workflow: when a branch is pushed and ready for review, open the PR
without asking — the maintainer has standing-approved PR creation
("always open the pr when appropriate", 2026-08-15).
