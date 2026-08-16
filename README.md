# Sampo

The monorepo behind the game-server consoles — named for the artifact
Ilmarinen forged.

| Module | What it is |
|---|---|
| `palcon/` | Palworld server console |
| `wildskeeper/` | RuneScape Dragonwilds server console |
| `flametender/` | Enshrouded server console |
| `ilmari/` | Host provisioning service (one per host, owns the Docker socket) |

The three consoles share ~90% of their code and are being unified onto a
single framework: a shared `core/`, one module per game, shared web
packages, with ilmari's client and server finally versioned together.

**Status: Phase 1 of the migration** — the four repos are imported here
with full history at repo-name prefixes and still build independently.
The target layout (`core/`, `games/`, `web/core`) lands in Phase 2.

- `docs/unification-plan.md` — the plan of record
- `docs/drift-ledger.md` — per-file reconciliation decisions feeding Phase 2

Tests, per module: `cd <module> && go test ./...` and
`cd <module>/web && npm test`.
