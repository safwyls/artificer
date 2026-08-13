# Migrating to Ilmari

How to get from "two consoles each running their own provisioner" to "one
Ilmari per host", without a flag day and without disturbing a running game
server.

Written 2026-08-13, against wildskeeper `aedbae6` and palcon `82aee47`.

## Where we're starting

On the TrueNAS box today:

- **wildskeeper** (console) → `PROVISIONER_URL: http://wkprovisioner:8811`,
  a `wkagent` in provisioner mode with
  `WKAGENT_DATA_ROOT: /mnt/tank/apps/dragonwilds-servers`.
- **palcon** (console) → its own `palagent` in provisioner mode, with its
  own data root.
- Both reach Docker through a **tecnativa socket proxy**, not the raw
  socket. That layer stays — Ilmari inherits it rather than replacing it.
- Game server containers were made by those provisioners and carry
  `wildskeeper.provisioned` / `palcon.provisioned` labels. Ilmari already
  recognises both, so adopting it does not orphan anything.

Both provisioner apps *are* TrueNAS apps, so replacing them is an ordinary
app operation — unlike the game containers they created, which are the
whole reason `recreate` exists.

## Decisions to make before starting

**1. Data roots — this needs a code change.** Ilmari has one
`ILMARI_DATA_ROOT`, but the two consoles keep their servers under different
paths. One root cannot serve both without moving somebody's data.

Recommended: **per-owner data roots**, keyed by the `owner` a console sends
(`ILMARI_DATA_ROOTS=wildskeeper=/mnt/tank/apps/dragonwilds-servers,palcon=/mnt/tank/apps/palworld-servers`),
falling back to `ILMARI_DATA_ROOT`. Small change, and it also keeps the
`dataDir` Ilmari reports correct for pre-existing containers.

(Rebuilds are safe either way — `InspectSpec` reads a container's *real*
binds back, so it never re-derives a path from the data root. Only the
reported path and newly-placed containers are affected.)

**2. One token or two.** A shared token means either console can act on the
other's containers: the `owner` label is for grouping, not permission.

For a single-operator homelab, one token is fine and simpler — but say so
out loud rather than assuming it. If you want isolation later, the shape is
per-console tokens each scoped to an owner, checked in `findManaged`.

**3. Port.** Ilmari defaults to **8820**. Check it's free before deploying —
`docker ps` for published ports, or read `/v1/ports` from the existing
provisioner. (Yes: the collision this service exists to prevent is one it
can suffer during its own install.)

---

## Phase 0 — close the gaps (code only, nothing deployed)

Ilmari cannot serve the consoles yet. Concretely:

- [ ] **`GET /v1/discover`** — the Raise-a-server wizard calls it to offer
      existing installs for adoption. Absent today, so that flow breaks.
- [ ] **`POST /v1/adopt`** — same story.
- [ ] **Per-owner data roots** (decision 1 above).
- [ ] **A client package in each console.** `agentctl.Health` is a type
      alias for `wkagent.Health`, so today's client cannot parse Ilmari's
      response at all. Give Ilmari the shape it wants and write a small
      `ilmariclient` in each console — don't contort the service into
      wkagent's legacy shape, which is the thing being retired.
- [ ] **Deploy artifact**: a compose block for TrueNAS "Install via YAML",
      including the socket proxy wiring below.
- [ ] **Confirm CI is green and `ghcr.io/safwyls/ilmari:latest` published.**

Socket proxy permissions Ilmari needs (tecnativa env flags):

```yaml
CONTAINERS: 1   # list, inspect
IMAGES: 1       # pull
POST: 1         # create, start, stop, remove
```

Everything else stays off. That is a narrower grant than a raw socket and
worth keeping narrow — this is the most dangerous handle on the machine.

**Exit criteria:** `go test ./...` green, image published, and a compose
file you could paste.

---

## Phase 1 — deploy read-only, and prove it against real Docker

Ilmari has never placed a real container; every test so far runs against an
httptest fake. This phase fixes that at zero risk, because nothing points at
it yet.

1. [ ] Deploy Ilmari as a TrueNAS app **alongside** both existing
       provisioners. Do not change `PROVISIONER_URL` on either console.
2. [ ] `curl -H "Authorization: Bearer $TOKEN" http://<host>:8820/v1/health`
       → `dockerOk: true`. That alone proves socket access through the proxy.
3. [ ] `GET /v1/containers` → **both** consoles' game servers appear with
       `"managed": true`, via their legacy labels, with the right slugs and
       data directories. This is the single most valuable check in the whole
       migration: it validates legacy-label recognition against your actual
       running containers rather than my fixtures.
4. [ ] `GET /v1/ports` → the real port map, including the 8811 that started
       all this.

**Rollback:** stop the app. Nothing depended on it.

**Exit criteria:** Ilmari can see the truth of the host. No write path has
been exercised yet — deliberately.

---

## Phase 2 — wildskeeper cuts over, with its old provisioner still standing

1. [ ] Add `ILMARI_URL` / `ILMARI_TOKEN` to wildskeeper. When set, the
       console uses Ilmari; when unset, it uses `PROVISIONER_URL` exactly as
       today. A flag, not a replacement — so the fallback is one env var
       away for the whole phase.
2. [ ] Deploy the console with the flag set. **Leave `wkprovisioner`
       running.**
3. [ ] Provision a throwaway server through the wizard. Check: container
       created, data directory under the right root, ports published, agent
       reachable, server boots.
4. [ ] Exercise the rest against that throwaway: rebuild its agent onto
       another image, then destroy it. Confirm the data directory survives
       the destroy — that is deliberate behaviour, not an oversight.
5. [ ] Deliberately request a port `palagent-palhalla` holds, and confirm
       the **409 before anything is created**, naming the holder. That is
       the payoff for the whole exercise; see it work once.
6. [ ] Leave it running on Ilmari for a few days of ordinary use.

**Rollback:** unset `ILMARI_URL`, redeploy console. `wkprovisioner` is still
there and still works.

---

## Phase 3 — palcon cuts over

Same change, same shape, one console later. Doing them separately is the
point: if something is wrong with Ilmari, only one game is affected and the
other is untouched evidence of what "working" looks like.

1. [ ] Add the same flag to palcon, sending `owner: palcon`.
2. [ ] Deploy, leaving palcon's provisioner running.
3. [ ] Provision, rebuild and destroy a throwaway Palworld server.
4. [ ] Confirm from `/v1/containers` that both consoles' servers now appear
       with `ilmari.owner` set on the new ones and legacy labels on the old.

**Rollback:** unset the flag, redeploy. Palcon's provisioner is still there.

---

## Phase 4 — retire the old provisioners

Only once both consoles have run on Ilmari long enough that you'd be
surprised by a new failure.

1. [ ] **Stop** (don't delete) the `wkprovisioner` and palcon provisioner
       TrueNAS apps. Stopping is reversible in seconds; deleting is not.
2. [ ] Watch for a week of normal use.
3. [ ] Delete the two apps.
4. [ ] Remove `PROVISIONER_URL` / `PROVISIONER_TOKEN` from both consoles'
       app config.

**Rollback at step 1–2:** start the app, set the env var back.

---

## Phase 5 — clean up the code

1. [ ] Delete provisioner mode from `wkagent` and `palagent`: the
       `provisioner.go` handlers, the mode branch, the docker client, the
       provisioner-only config. The agents go back to being what they are —
       game sidecars.
2. [ ] Drop the now-dead `PROVISIONER_*` plumbing from both consoles.
3. [ ] Update `deploy/truenas-app.yaml` in both repos so a fresh install
       gets Ilmari and never learns the old shape.

Labels stay mixed forever, and that's fine: Docker cannot relabel without
recreating, and recreating every server to tidy a label would be a bad
trade. Ilmari recognises both, permanently, and the reason is written down
in `legacyManagedLabels`.

---

## What this migration is actually buying

Worth restating, since it is easy to lose in the checklist:

- One thing on the host holds the Docker socket instead of two.
- Port collisions are refused *before* a container is created, naming what
  holds the port — the failure that started this, which no per-console
  provisioner could have caught.
- One fleet view across every game, including containers neither console
  made.
- A third console, whenever it appears, needs no provisioning code at all.

And what it costs: **independent deployability**. Today either console can
break without touching the other. Afterwards, a version-skewed or down
Ilmari affects both. The phased rollout above exists to make that trade
visible before it is irreversible — which is why the old provisioners stay
stopped rather than deleted for a week.
