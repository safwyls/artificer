# Flametender port — real-server verification

The Phase 3 gate (unification plan): the monorepo-built images must run
against the real Enshrouded server before the old flametender repo is
archived. This is the checklist for that session.

## Deploying the candidate

The `Docker (flametender)` workflow publishes on every push to main,
under the **same ghcr names** the old repo used, but **without
`:latest`** — the branch tag is the verification channel:

    ghcr.io/safwyls/flametender:main
    ghcr.io/safwyls/flameagent:main

Repoint the TrueNAS app's image tags from `:latest` to `:main` and
restart the apps. Rollback at any point is repointing back to `:latest`
(still the old repo's last build).

The console runs against the existing database — every migration in the
monorepo is filename-identical to one already applied, so boot must be a
schema no-op. If the console runs a migration on a production DB here,
stop and treat it as a gate failure.

## Checklist

Login & identity
- [ ] Password login works; existing sessions survive (the cookie is
      still `flametender_session`).
- [ ] Cloudflare Access sign-in works; `CF_ACCESS_ADMIN_EMAILS` still
      grants admin.

Server card & live state
- [ ] Agent health, game state, disk, and job status render.
- [ ] A2S presence: player count/slots from the query while the game is
      up; log-derived fallback when it isn't answering.
- [ ] Readiness three-state still behaves (booting ≠ ready).

Config & moderation
- [ ] Settings editor round-trips an edit through the agent.
- [ ] Rotate-admin-password works.
- [ ] Roles editor reads and writes groups (restart-required notice).
- [ ] **Ban queue**: ban a test id while the game is running — it must
      queue, not write; restart must apply it between stop and start;
      the panel shows pending → applied.

Power & scheduling
- [ ] Stop is graceful (SIGINT; world saved — check the game log for
      the save on the way down; clean exit, not 143).
- [ ] Scheduled restart fires with the save-outcome Discord notice.

Backups & saves
- [ ] Manual snapshot produces a non-empty archive containing the hex
      world blobs and `-index`/`-info` sidecars.
- [ ] An induced failure (briefly misconfigure the save path) surfaces
      `lastFailure` in the UI instead of failing silently.

Provisioning (Anvil)
- [ ] Wizard defaults prefill from Anvil; raise a throwaway server
      end to end; join it once; destroy it (world dir must survive the
      destroy, per the DestroyResult contract).
- [ ] Discover/adopt still lists and adopts.
- [ ] Agent image recreate (`:main` ↔ another tag) keeps ports,
      volume, and env.

Continuity details
- [ ] The persisted desired-state file (`<install>/.flameagent/desired`)
      is honored — a server stopped before the upgrade stays stopped.

## After the gate

1. Flip `:latest` on in `.github/workflows/docker-flametender.yml`
   (the `enable=false` line) and repoint the apps back to `:latest`.
2. Archive `safwyls/flametender` with a pointer commit to this repo.
3. Delete the imported `flametender/` tree from the monorepo and drop
   its job from CI (the code lives in `core/`, `games/enshrouded/`,
   `cmd/`, and `web/flametender/` now).
4. Tick the satisfied §F guards in `docs/drift-ledger.md`.
