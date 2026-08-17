# Artificer

The monorepo behind three self-hosted game-server consoles — one shared
framework that forges all three. The host service they're forged on is
`anvil`: the fixed thing on the machine that places every container.

**[Documentation and live demo →](https://safwyls.github.io/artificer/)**

| Console | Game | Agent | Image |
|---|---|---|---|
| palcon | Palworld | `palagent` | `ghcr.io/safwyls/palcon` |
| wildskeeper | RuneScape Dragonwilds | `wkagent` | `ghcr.io/safwyls/wildskeeper` |
| flametender | Enshrouded | `flameagent` | `ghcr.io/safwyls/flametender` |

Plus `anvil` — one per host, the only component holding Docker rights, and
what makes the Raise-a-server wizard work.

## Layout

| Path | What lives there |
|---|---|
| `core/` | The console framework: API, auth, store, backups, scheduler, the sidecar agent kit, the Anvil client. Game-blind. |
| `games/<game>/` | One directory per game: client, config codec, save/log readers, agent spec, contributed routes. |
| `cmd/` | The binaries — each console main is thin wiring over `core`. |
| `web/<console>/` | One React app per console, each themed for its game. |
| `anvil/` | The host provisioning service. Separate module; references no console. |
| `deploy/` | Dockerfiles, one directory per console. |
| `site/` | The public docs site and landing page, published to GitHub Pages. |

Dependency rules — core never imports a game, games never import each other,
an agent never imports its console-side game package, anvil references
nothing above it — are enforced in CI by `scripts/checkbounds.sh`.

## Status

All three consoles are ported onto `core` and verified against real servers
(flametender and wildskeeper 2026-08-16, palcon 2026-08-17). No legacy tree
remains; every image publishes from here.

Next: Phase 6 — Anvil convergence (the guards still open in the drift ledger's
§F) and doc consolidation.

- `docs/unification-plan.md` — the plan of record
- `docs/drift-ledger.md` — per-file reconciliation decisions, and what's left

## Tests

```sh
go build ./... && go vet ./... && go test ./...
./scripts/checkbounds.sh
cd web/<console> && npm ci && npm run build && npm test
```

The save-backed tests need `palworld-save-tools` importable by `python3`; they
skip without it rather than failing.
