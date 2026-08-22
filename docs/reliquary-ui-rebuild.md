# Reliquary UI rebuild plan

**Done (2026-08-21).** All three phases landed in one change: the React
frontend lives in `web/reliquary`, `cmd/reliquary` embeds it through
`web/reliquary`.`Dist()`, `cmd/reliquary/ui/` is gone (and with it the
`RELIQUARY_LEGACY_UI` escape hatch, which was never needed once parity
was reached in the same change), CI builds and tests it, and
`deploy/reliquary/Dockerfile` grew the node stage. What is **not** done
is the phase 3 smoke-check against a real deployment — login including
the SSO hints, a checkout/check-in round trip, the admin panels — which
needs the live host. `scripts/checkbounds.sh` now enforces the
game-blindness rule on `web/reliquary/src` as well.

The plan of record for replacing `cmd/reliquary/ui/index.html` — today a
single 702-line vanilla page (inline CSS, innerHTML string templating,
one global refresh) — with a React frontend structured like the three
consoles. The visual identity is **kept, not redesigned**: the mock at
https://claude.ai/code/artifact/9fb1287b-8f27-4bdc-a4f7-7d1ca49e5876
shows the target screens, and the "Design language" section below is
normative.

Read `docs/save-sync-architecture.md` and `docs/companion.md` first;
the UI's concepts (custody, claims, conflicts, companion tokens) come
from there. The current `index.html` is the behavioral spec — every
feature it has must survive the port; its inline comments record
hard-won behavior (the `jsattr` escaping bug, whole-record user
updates, art-fetch discipline, SSO fall-through hints). Port the
lessons, not the string templating.

## Target structure

Mirror the consoles exactly (`web/wildskeeper` is the reference):

```
web/reliquary/
  embed.go            // package web, //go:embed all:dist, Dist() fs.FS
  index.html
  package.json        // vite + react + typescript + tailwind, npm (package-lock)
  vite.config.ts      // dev proxy: /api -> http://localhost:<reliquary port>
  tailwind.config.js  // vault palette as tokens (below)
  src/
    main.tsx, App.tsx // router: /login, /, /worlds/:id, /companion, /admin/*
    index.css         // css vars + tailwind layers
    lib/api.ts        // fetch wrapper (the j() helper, typed), SSE hook
    lib/types.ts      // World, WorldStatus, Holder, Version, User, Me...
    components/
      AppShell.tsx        // sidebar nav, role-gated items, user + live dot + version footer
      WorldCard.tsx       // cover, custody chip, state-driven actions
      CustodyChip.tsx     // Free / Held / Hold expired variants
      CoverArt.tsx        // img with fallback tile (name in a gradient box)
      VersionRow.tsx      // v-id, HEAD/CONFLICT/CHECKPOINT badges, actions
      ConfirmDialog.tsx   // replaces window.confirm
      ui/                 // button, input, label, badge, dialog, table primitives
    pages/
      Login.tsx           // password box + SSO hint states (404/401/other)
      Worlds.tsx          // world card list + new-world + companion strip
      WorldDetail.tsx     // header + tabs: History / Settings / Server link
      Companion.tsx       // token mint/copy/revoke, download, bundled version
      AdminUsers.tsx      // table + add form (custody/role/disable/delete)
      AdminArtwork.tsx    // IGDB credentials + status/test
      AdminCatalogue.tsx  // savedb status + refresh
    test/               // vitest + testing-library, like the consoles
```

Go side: `cmd/reliquary/main.go` swaps `//go:embed ui` for importing
`web/reliquary`'s `Dist()` (the consoles' pattern); `VaultRoutes`
keeps serving the FS it is handed — no API changes. Delete
`cmd/reliquary/ui/` only in the same change that embeds the new build.
CI: add `web/reliquary` to the monorepo's npm build/test matrix; the
Dockerfile in `deploy/reliquary` gains the same node build stage the
console images have.

Dependencies: copy a console's stack (react, react-router-dom,
@tanstack/react-query, tailwind, clsx/tailwind-merge, radix dialog +
label + tooltip, lucide-react). No new libraries without cause.

## Design language (normative)

One deliberate dark look — no light theme, no theme toggle. Tokens, as
CSS variables consumed by Tailwind (`colors: { ink: "var(--ink)" ... }`):

- `--ink #100d17` page ground · `--panel #1a1524` cards · `--edge
  #2f2740` borders · `--parchment #e8e0cf` text · `--mist #948da3`
  secondary text · `--gold #c9a860` / `--goldhi #e3c67f` accent &
  headings · `--ok #7fc46a` · `--ember #d4735e` danger/conflict ·
  `--rune #9d7fc4` game tags / info.
- Type: Georgia serif body and headings; `ui-monospace` for ids,
  sizes, timestamps, tokens. Panel headings uppercase, letterspaced,
  gold.
- Buttons: primary = gold outline on a dark gold gradient; quiet =
  mist text, edge border; danger = quiet that turns ember. One primary
  per custody state, never several.
- Icons: lucide (stroke style matches the mock's inline SVGs). No emoji.

## What changes structurally (from the mock)

1. **App shell** replaces the single scrolling stack: left sidebar —
   Worlds / Companion / Users / Cover art / Save catalogue — with
   admin items rendered only for admins and the read-only
   ("no custody") notice shown as a banner on Worlds. User identity,
   live-SSE dot, and version move to the sidebar footer.
2. **World cards** get a custody chip (Free green / Held gold /
   Hold expired ember) driving one primary action; quiet actions
   trimmed to Download head + History + an overflow menu for the
   admin/rare verbs (host on server, force release, import, delete).
3. **World detail page** (`/worlds/:id`) absorbs what today piles into
   the card: tabs for History (version rows with HEAD / CONFLICT /
   CHECKPOINT badges, Make head), Settings (the admin form), Server
   link (agent URL/token, give/take). Retention + storage line under
   the history.
4. **Companion page** takes the token panel and download; the Worlds
   page keeps only a one-line pointer strip.
5. **Login** keeps its exact SSO behavior: try `/me`, try
   `/login/cloudflare`, and show the three distinguishable hints
   (not configured / no assertion / other error) — that logic ports
   verbatim.

## Behavior that must survive (the port checklist)

- All action verbs and their visibility rules in `worldActions()` —
  they encode the permission model (canSync, mine, claimable,
  serverHeld, requestedKind, admin) and their confirm texts.
- User updates send the **whole record** (role + permissions +
  disabled together) — the API replaces, a partial write clears fields
  (see the comment block above `saveUser`).
- Cover art: fetch only when the **set of worlds changes**, never on
  the poll; failures are silent (covers are decoration). Key by
  `app:<appId>` else `name:<lower(name)>`.
- SSE `custody` events trigger a refetch; 20 s poll stays as the
  fallback; `live` / `reconnecting…` indicator; re-open a dead
  EventSource after 5 s. With react-query this is
  `invalidateQueries` on event.
- Uploads are raw `.tar` POSTs (`checkin`, `import`) with the
  conflict-flag toast on `version.conflict`.
- Logout must follow `ssoLogoutURL` when the session began at Access.
- Version string on both login and app footer.
- Read-only accounts see worlds and can download; every mutating
  affordance hides (not disables) without the `savesync` grant.

## Phases (each lands green: `go build ./... && go vet ./... && go test ./...`, `./scripts/checkbounds.sh`, `cd web/reliquary && npm test`)

- **Phase 1 — scaffold.** `web/reliquary` with the console toolchain,
  AppShell, router, api/types libs, Login, and a Worlds page that
  renders the world list read-only. Embed + serve it from
  `cmd/reliquary`; keep the old `ui/` untouched and behind a
  `RELIQUARY_LEGACY_UI=1` escape hatch until phase 3.
- **Phase 2 — parity.** World actions, uploads, world detail
  (history/settings/server link), Companion page, the three admin
  pages, SSE + poll. Port the checklist above; tests per component in
  the consoles' style (the existing UI's comment-documented bugs each
  get a regression test: attribute escaping, whole-record user saves,
  art-fetch memoization).
- **Phase 3 — cutover.** Delete `cmd/reliquary/ui/` and the legacy
  flag; CI + Dockerfile build the new frontend; smoke-check against a
  real deployment (login incl. SSO hints, checkout/check-in round
  trip, admin panels). Update `docs/state-of-play.md`.

Structural rules apply: reliquary stays game-blind (no game module
imports in `web/reliquary` — it renders whatever metadata the API
reports), and the frozen-API rule means **no route, env var, or image
name changes** ride along with this rebuild.
