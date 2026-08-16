# Wildskeeper port — real-server verification

The Phase 4 gate: the monorepo-built images must run against the real
Dragonwilds server before the old wildskeeper repo is archived.

## Deploying the candidate

The `Docker (wildskeeper)` workflow publishes on every push to main,
same ghcr names, no `:latest` yet:

    ghcr.io/safwyls/wildskeeper:main
    ghcr.io/safwyls/wkagent:main
    ghcr.io/safwyls/wkagent:main-wine

Repoint the TrueNAS app tags from `:latest` to `:main` (agents to
`:main` / `:main-wine` per server, via the agent-image action).
Rollback is repointing back.

**Provisioning note (the one behavior change):** legacy provisioner-mode
agents are retired — the monorepo console provisions through Ilmari
only (`ILMARI_URL`/`ILMARI_TOKEN`; `PROVISIONER_URL` is no longer
read). Wildskeeper's deployment already runs Ilmari and adoption was
verified 2026-08-15, so this should be a no-op — but confirm the wizard
still works before anything else, and re-adopt if any server was still
registered through the legacy path.

Boot must be a schema no-op on the existing database — a migration
running here is a gate failure.

## Checklist

Login & identity
- [ ] Password login works; existing sessions survive
      (`wildskeeper_session`).
- [ ] Cloudflare Access sign-in works — **new to wildskeeper with the
      port** (core promoted it): configure `CF_ACCESS_*` if you want it,
      or confirm password login alone still behaves with it unset.

Server card & live state
- [ ] Agent health, game state, disk, jobs render.
- [ ] Log-derived roster (dwlog RulesV1) still names players; with the
      Wine build + dwbridge, the bridge roster outranks it.

Config
- [ ] DedicatedServer.ini editor round-trips an edit; per-profile config
      dir is respected (LinuxServer/ vs WindowsServer/).
- [ ] Rotate-admin-password works.

Launch profiles & dwbridge
- [ ] GET launch reports the selected build; PUT switches (persists
      across agent recreation; settings carried to the new build's
      config dir).
- [ ] On the Wine agent: one-click UE4SS kit install; bridge heartbeat
      goes Available; `Save` works end to end; unimplemented commands
      still answer 501 with the honest reason.
- [ ] Native build: commands answer 501 (no mod path), power still works.

World & saves
- [ ] The world page renders SPUD metadata from the newest .sav.
- [ ] Manual snapshot produces a non-empty archive of the save dir.
- [ ] Induced backup failure surfaces `lastFailure` in the UI — **new to
      wildskeeper with the port** (flametender's fix, now core's).

Provisioning (Ilmari)
- [ ] Wizard defaults prefill; the **port pair** proposal strides by two
      and refuses an agent port inside the pair; ownerId is required
      with the in-game pointer.
- [ ] Raise a throwaway server end to end (owner id seeded into the
      ini); join once; destroy (world dir survives).
- [ ] Discover/adopt works; agent recreate keeps ports/volume/env.

Power & scheduling
- [ ] Graceful stop is clean (~2 s, exit 143 is the game's own way);
      scheduled restart fires with the save-outcome notice (Save via
      bridge on Wine; "unsupported" wording on native).

Continuity
- [ ] Persisted desired state (`<install>/.wkagent/desired`) and the
      persisted launch profile (`<install>/.wkagent/profile`) are
      honored across the upgrade.

## After the gate

1. Flip `:latest` on in `.github/workflows/docker-wildskeeper.yml`.
2. Archive `safwyls/wildskeeper` with a pointer commit.
3. Delete the imported `wildskeeper/` tree, its CI job, checkbounds
   legacy rule, and go.work entry.
4. Tick the satisfied §F guards in `docs/drift-ledger.md` (the pair
   rows are already asserted in core's provision_pair tests).
