# Palcon port — real-server verification

The Phase 5 gate: the monorepo-built images must run against the real
Palworld server before the old palcon repo is archived.

## Deploying the candidate

The `Docker (palcon)` workflow publishes on every push to main, same
ghcr names, no `:latest` yet:

    ghcr.io/safwyls/palcon:main
    ghcr.io/safwyls/palagent:main

Repoint the deployment's tags from `:latest` to `:main` (console and
agents). Rollback is repointing back.

**Set the image tag to `main` in the wizard too.** The Raise/Provision
dialog prefills the agent image tag with `latest`, and for palagent
`:latest` is still the *legacy* image built from the old repo — this
workflow deliberately does not publish `:latest` until the gate passes.
Leaving the default means provisioning a monorepo console against a
legacy agent, which is not what this gate is testing. Set it under
Advanced, per server, until the flip.

**Provisioning migration (the one behavior change):** the legacy
provisioner-mode agent is retired — the monorepo console provisions
through Ilmari only. Concretely:

- `PROVISIONER_URL` / `PROVISIONER_TOKEN` are **no longer read** by the
  console; set `ILMARI_URL` / `ILMARI_TOKEN` instead (Ilmari already
  runs on the shared host for flametender and wildskeeper — palcon's
  allowlist needs `ghcr.io/safwyls/palagent` in
  `ILMARI_ALLOWED_IMAGE_PREFIXES` if it isn't there yet).
- The palagent image no longer reads `PALAGENT_DOCKER_HOST`,
  `PALAGENT_DATA_ROOT`, `PALAGENT_PUBLIC_HOST`,
  `PALAGENT_DEFAULT_IMAGE_TAG` or `PALAGENT_DEFAULT_RUN_AS`. A
  container that only ever acted as a per-server sidecar is unaffected;
  a container that was *also* the provisioner simply loses that role to
  Ilmari.
- Existing game stacks keep working untouched (their `PALAGENT_*`
  runtime env is unchanged). Re-home them into the wizard via
  Discover/Adopt once Ilmari can see them.

`DOCKER_HOST` power control is unchanged — deployments that drive
compose-stack containers directly keep doing so.

Boot must be a schema no-op on the existing database — a migration
running here is a gate failure.

## Checklist

Login & identity
- [ ] Password login works; existing sessions survive
      (`palcon_session`).
- [ ] Cloudflare Access sign-in works — **new to palcon with the port**
      (core promoted it): configure `CF_ACCESS_*` if you want it, or
      confirm password login alone still behaves with it unset.

Server card & live state
- [ ] REST path: metrics, player list, server info render.
- [ ] RCON fallback: with REST disabled on a server, the card still
      works through RCON.

The pals surface (the reason this console exists)
- [ ] /pals, /guilds, /inventory, /storage, /achievements all serve
      from the live save; Paldex and the calculators load.
- [ ] Storage honours its switches: world loot and private chests stay
      server-side filtered.
- [ ] Visibility switches: hiding a view 403s members, admins bypass;
      per-player hides hold across every view.
- [ ] The save refresher keeps the parse warm across an autosave (page
      load right after an autosave is a cache hit, not a multi-second
      parse).

Pal advisor
- [ ] Advisor chat answers with whichever key is configured (env or
      UI-saved); docs-search still finds the embedded docs.

Config
- [ ] PalWorldSettings.ini editor round-trips an edit.

Moderation & power
- [ ] Kick/ban/unban; broadcast; in-game shutdown with countdown.
- [ ] Graceful stop/restart via the agent; scheduled restart fires with
      warnings.

Backups
- [ ] Manual snapshot produces a non-empty archive of .sav files only;
      Level.sav magic is verified (a mid-write save is refused, not
      archived).
- [ ] Induced backup failure surfaces `lastFailure` in the UI — **new
      to palcon with the port** (flametender's fix, now core's).

Provisioning (Ilmari)
- [ ] Wizard defaults prefill; the **admin-port trio** proposal moves
      game + REST + RCON ports together and refuses any collision among
      the four (incl. the agent port).
- [ ] A newly raised server installs and starts: the agent's own boot
      runs SteamCMD, then autostarts. A 502 on Start carries the agent's
      message verbatim (the console maps any agent 500 that way), so
      read it — `game is not installed under /palworld` means the
      install never finished, and `docker logs palagent-<slug>` says
      why. Check `runAs` against the image: unlike wkagent, the palagent
      image has no baked-in unprivileged user.
- [ ] Raise a throwaway server end to end: ini seeded from the game's
      defaults with ServerName/ServerDescription applied once,
      AdminPassword + RCONEnabled + RESTAPIEnabled enforced, REST and
      RCON reachable on the published ports, server row wired for both.
- [ ] Discover/adopt works and only lists palagent-* containers; agent
      recreate keeps ports/volume/env.

Continuity
- [ ] Persisted desired state (`<install>/.palagent/desired`) is
      honored across the upgrade.
- [ ] The save-extractor still runs in the new console image (python3 +
      palworld-save-tools + pyooz baked in).

## After the gate

1. Flip `:latest` on in `.github/workflows/docker-palcon.yml`.
2. Archive `safwyls/palcon` with a pointer commit.
3. Delete the imported `palcon/` tree, its CI job, checkbounds legacy
   rule, and go.work entry.
4. Tick the satisfied §F guards in `docs/drift-ledger.md` (the trio
   rows are asserted in core's provision_trio tests).
