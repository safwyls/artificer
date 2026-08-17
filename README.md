# Sampo

The monorepo behind three self-hosted game-server consoles — named for the
artifact Ilmarinen forged, which is also why the host service is `ilmari`.

**[Documentation and live demo →](https://safwyls.github.io/sampo/)**

| Console | Game | Agent | Image |
|---|---|---|---|
| palcon | Palworld | `palagent` | `ghcr.io/safwyls/palcon` |
| wildskeeper | RuneScape Dragonwilds | `wkagent` | `ghcr.io/safwyls/wildskeeper` |
| flametender | Enshrouded | `flameagent` | `ghcr.io/safwyls/flametender` |

Plus `ilmari` — one per host, the only component holding Docker rights, and
what makes the Raise-a-server wizard work.

## Layout

| Path | What lives there |
|---|---|
| `core/` | The console framework: API, auth, store, backups, scheduler, the sidecar agent kit, the Ilmari client. Game-blind. |
| `games/<game>/` | One directory per game: client, config codec, save/log readers, agent spec, contributed routes. |
| `cmd/` | The binaries — each console main is thin wiring over `core`. |
| `web/<console>/` | One React app per console, each themed for its game. |
| `ilmari/` | The host provisioning service. Separate module; references no console. |
| `deploy/` | Dockerfiles, one directory per console. |
| `site/` | The public docs site and landing page, published to GitHub Pages. |

Dependency rules — core never imports a game, games never import each other,
an agent never imports its console-side game package, ilmari references
nothing above it — are enforced in CI by `scripts/checkbounds.sh`.

## Status

Flametender and wildskeeper are fully ported onto `core` and verified against
real servers. Palcon's port is code-complete and in its verification gate; the
legacy `palcon/` tree stays until that passes.

- `docs/unification-plan.md` — the plan of record
- `docs/drift-ledger.md` — per-file reconciliation decisions
- `docs/palcon-port-verification.md` — the open gate

## Tests

```sh
go build ./... && go vet ./... && go test ./...
./scripts/checkbounds.sh
cd web/<console> && npm ci && npm run build && npm test
```

The save-backed tests need `palworld-save-tools` importable by `python3`; they
skip without it rather than failing.
