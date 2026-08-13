# Ilmari

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

Ilmari knows containers, images, ports, paths and disk. It does **not** know
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
2. **Host paths are never caller-controlled.** A caller names a slug; Ilmari
   decides the directory beneath its data root. There is deliberately no
   bind-mount field in the spec.
3. **Ownership labels cannot be forged.** They are applied last, and they
   are what every destroy and rebuild checks before touching anything.

Nothing here ever returns a container's environment: it carries tokens and
passwords, and a fleet view is not worth leaking them for.

## API

All routes need `Authorization: Bearer $ILMARI_TOKEN`.

| Verb | Path | Does |
|---|---|---|
| GET | `/v1/health` | version, data root, allowlist, whether Docker answered |
| POST | `/v1/provision` | place a container from a spec |
| POST | `/v1/provision/recreate` | rebuild one on a different image, keeping everything else |
| POST | `/v1/provision/destroy` | remove one, keeping its data directory |
| GET | `/v1/containers` | every container on the host, ours flagged |
| GET | `/v1/ports` | every published host port and what holds it |

`/v1/containers` and `/v1/ports` deliberately report **everything on the
host**, not just Ilmari's. A view that only showed its own would reproduce
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

Everything under `env` is the console's business. Ilmari never reads it.

## Configuration

| Variable | Meaning |
|---|---|
| `ILMARI_TOKEN` | shared bearer token; ≥16 chars, required |
| `ILMARI_DOCKER_HOST` | Docker endpoint (default `unix:///var/run/docker.sock`) |
| `ILMARI_DATA_ROOT` | directory server data directories are created under; required |
| `ILMARI_PUBLIC_HOST` | the address consoles and players reach this machine on |
| `ILMARI_ALLOWED_IMAGE_PREFIXES` | comma-separated allowlist; unset keeps the narrow default |
| `ILMARI_DEFAULT_RUN_AS` | uid:gid suggested to consoles that don't specify |
| `ILMARI_ADDR` | listen address (default `:8820`) |

## Migrating from the built-in provisioners

Containers created by a console's own provisioner carry
`wildskeeper.provisioned` or `palcon.provisioned`. Ilmari recognises both
and treats them as its own, so an existing deployment can adopt it without
relabelling live containers or orphaning running servers. New containers get
the neutral `ilmari.managed` / `ilmari.slug` / `ilmari.owner`.

## Status

**Not yet deployable.** The service builds, tests and publishes, but it is
missing two verbs the consoles depend on (`discover`, `adopt`), serves a
health shape their current client cannot parse, and has never talked to a
real Docker socket — every test so far runs against a fake.

[`docs/migration.md`](docs/migration.md) is the plan from here to replacing
both consoles' provisioners: what to build first, how to prove it against
real Docker with no write path, and how to cut each console over with its
old provisioner still standing.
