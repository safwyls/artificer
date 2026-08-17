# Anvil

The host provisioning service for [wildskeeper](https://github.com/safwyls/wildskeeper),
[palcon](https://github.com/safwyls/palcon) and anything else that needs a
game server placed on a machine. One per host.

Named for the smith of the Kalevala, who made things that worked.

## Why it exists

Provisioning started inside each game console's sidecar agent and was
copied into the second console for the second game. Two processes then held
the same Docker socket on the same machine, created containers the same
way, and **could not see each other**. That is not theoretical: one console
proposed a host port the other already held, the create succeeded, the
start failed, and the leftover container had to be removed by hand.

A host has one Docker socket. It should have one thing holding it.

## What it knows, and what it refuses to know

Anvil knows containers, images, ports, paths and disk. It does **not** know
what a world name is, which game a container runs, or what its settings
mean. All of that arrives as data in a provisioning spec and is passed
through untouched.

That is the whole design. A console owns its game's knowledge and sends it;
a game's changes never become a deploy of this service; and adding a third
console needs no code here at all.

## Security posture

The consoles deliberately hold no Docker rights. This service is the only
component that does, so it exposes a fixed set of shaped verbs rather than
anything resembling a Docker proxy. Three constraints keep "place a
container for me" from meaning "run anything you like on my NAS", and each
has a test:

1. **Images are allowlisted** by prefix, defaulting to the operator's own
   registry namespace. A leaked token deploys a newer agent, not a payload.
2. **Host paths are never caller-controlled.** A caller names a slug; Anvil
   decides the directory beneath its data root. There is deliberately no
   bind-mount field in the spec.
3. **Ownership labels cannot be forged.** They are applied last, and they
   are what every destroy and rebuild checks before touching anything.

The fleet views never return a container's environment: it carries tokens
and passwords. The one deliberate exception is `adopt`, which exists to
recover exactly those secrets — the console's own provisioner injected
them, so handing them back to that console's token stays inside the
original trust boundary — and it is filtered to the caller's registered
env namespace, so no console can read another's.

## Consoles are registered, not shared

Each game dashboard is a **registered client** with its own token, its own
data root, and optionally its own image allowlist:

```json
[
  {"id": "wildskeeper", "token": "…", "dataRoot": "/mnt/tank/apps/dragonwilds-servers"},
  {"id": "palcon",      "token": "…", "dataRoot": "/mnt/tank/apps/palworld-servers",
   "imagePrefixes": ["ghcr.io/safwyls/palagent"]}
]
```

(`ANVIL_CLIENTS` inline, or `ANVIL_CLIENTS_FILE` — the file wins, and is
the better habit for secrets.)

The contracts are deliberately **similar but separate**: same verbs, same
spec, same shapes, but separate credentials and separate entitlements. The
token says who is asking; ownership is enforced from it, never from the
request. Concretely:

- a wildskeeper token **cannot destroy or rebuild** a Palworld server
  (403), including servers that predate Anvil — the legacy
  `palcon.provisioned` label names their owner;
- each console's containers land under **its own data root**;
- each console can be held to **its own image allowlist**;
- the fleet views share exactly what collision-avoidance needs — every
  container's name, image, state and ports — while slug and data directory
  appear only on rows the caller owns.

Two clients sharing a token is a startup refusal, not a silent
last-one-wins.

## API

All routes need `Authorization: Bearer <that console's token>`.

| Verb | Path | Does |
|---|---|---|
| GET | `/v1/health` | version, data root, allowlist, whether Docker answered |
| POST | `/v1/provision` | place a container from a spec |
| POST | `/v1/provision/recreate` | rebuild one on a different image, keeping everything else |
| POST | `/v1/provision/destroy` | remove one, keeping its data directory |
| GET | `/v1/discover` | containers this console could adopt: its own, plus unlabelled ones under its image allowlist |
| POST | `/v1/adopt` | recover one for re-registration, env filtered to the caller's `envPrefix` |
| GET | `/v1/containers` | every container on the host, ours flagged |
| GET | `/v1/ports` | every published host port and what holds it |

`/v1/containers` and `/v1/ports` deliberately report **everything on the
host**, not just Anvil's. A view that only showed its own would reproduce
exactly the blindness this service replaces.

### Provisioning spec

```json
{
  "name": "wkagent-ashenfall",
  "slug": "ashenfall",
  "owner": "wildskeeper",
  "image": "ghcr.io/safwyls/wkagent:latest-wine",
  "user": "568:568",
  "env": { "WKAGENT_MODE": "supervisor", "WKAGENT_TOKEN": "..." },
  "ports": [
    { "host": 7777, "container": 7777, "proto": "udp" },
    { "host": 8811, "container": 8811, "proto": "tcp" }
  ],
  "dataMount": "/dragonwilds"
}
```

Everything under `env` is the console's business. Anvil never reads it.

## Configuration

| Variable | Meaning |
|---|---|
| `ANVIL_CLIENTS` | JSON array of client registrations (see above); required unless the file is set |
| `ANVIL_CLIENTS_FILE` | path to the same JSON; wins over the inline form |
| `ANVIL_DOCKER_HOST` | Docker endpoint (default `unix:///var/run/docker.sock`) |
| `ANVIL_PUBLIC_HOST` | the address consoles and players reach this machine on |
| `ANVIL_ALLOWED_IMAGE_PREFIXES` | fallback allowlist for clients that don't bring their own |
| `ANVIL_DEFAULT_RUN_AS` | uid:gid suggested to consoles that don't specify |
| `ANVIL_ADDR` | listen address (default `:8820`) |

## Migrating from the built-in provisioners

Containers created by a console's own provisioner carry
`wildskeeper.provisioned` or `palcon.provisioned`. Anvil recognises both
and treats them as its own, so an existing deployment can adopt it without
relabelling live containers or orphaning running servers. New containers get
the neutral `anvil.managed` / `anvil.slug` / `anvil.owner`.

## Status

**Ready for Phase 1: a read-only deploy alongside the existing
provisioners.** The API is complete (place, rebuild, destroy, discover,
adopt, fleet views), per-console tokens enforce ownership, the image is
published, and `deploy/truenas-app.yaml` is ready to paste.

Two honest limits remain. Nothing here has talked to a real Docker socket —
every test runs against a fake — which is exactly what Phase 1 exists to
prove, at zero risk, because nothing points at Anvil on day one. And the
consoles cannot *use* it yet: each needs a small `anvilclient` package
before its cut-over, which is Phase 2's first step.

[`docs/migration.md`](docs/migration.md) is the full plan from here to
retiring both consoles' built-in provisioners.
