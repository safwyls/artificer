# Artificer Companion UI rebuild plan

**Done (2026-08-22).** All three phases landed in one change: the React
frontend lives in `web/companion`, `cmd/companion/server.go` serves
`web/companion`.`Dist()` over the whole non-`/api` surface (the hashed
assets need serving, not just index.html), `cmd/companion/ui/` is gone
(and with it the `COMPANION_LEGACY_UI` fallback, unnecessary once parity
landed in the same change), and the npm stage was added to CI, the
release workflow and the reliquary image — which cross-builds the
companion exe and so needs its frontend too. The tokens and `ui/`
primitives were lifted from `web/reliquary` by copy; no shared
`web/vaultkit` package was extracted, which the plan left optional.
`scripts/checkbounds.sh` now enforces game-blindness on both frontends.
What is **not** done is the phase 3 smoke on a real Windows machine —
tray open, discovery, link, a checkout/check-in round trip against a
reliquary.

The plan of record for replacing `cmd/companion/ui/index.html` — an
811-line vanilla page (inline CSS, innerHTML templating, a 5-second
poll) — with a React frontend structured like the consoles and like
the reliquary rebuild (`docs/reliquary-ui-rebuild.md`; do that one
first or in parallel — the design language and several primitives are
shared). The visual identity is **kept, not redesigned**: the mock at
https://claude.ai/code/artifact/9b18da1f-9659-4f71-85b9-757b0f8b721e
shows the target screens; the reliquary plan's "Design language"
section is normative here too (same vault tokens, Georgia serif, gold
primaries).

Read `docs/companion.md` first — it explains discovery, the four save-
folder sources, path expansion, and the split/leaf model the link form
surfaces. The current `index.html` is the behavioral spec: its inline
comments each record a shipped bug and its fix; every one becomes a
regression test, not folklore.

## Target structure

```
web/companion/
  embed.go            // package web, //go:embed all:dist, Dist() fs.FS
  index.html, package.json, vite.config.ts (proxy /api -> 127.0.0.1:8377),
  tailwind.config.js  // vault palette tokens, as in web/reliquary
  src/
    main.tsx, App.tsx  // single page + modal state; no router needed
    lib/api.ts         // call() wrapper; poll hook (5s) with per-panel isolation
    lib/types.ts       // State, Link, DiscoveredGame, Probe, SplitInfo...
    components/
      HeaderBar.tsx        // identity, connection dot, freshness, Sync now, settings gear
      WorldRow.tsx         // cover thumb, custody chip, state-driven actions
      Shelf.tsx            // tile grid, hidden-entries tile, memoized on shelf signature
      GameTile.tsx         // linked (colour + gold ring + world caption) / unlinked (greyed) / hidden
      ScanTrail.tsx        // collapsible probe list; open-state owned by the user
      LinkGameDialog.tsx   // candidates, folder input, world select, seed checkbox, inline error
      FolderBrowser.tsx    // inline browser; saveish rows highlighted
      SplitExplainer.tsx   // the rune-tinted leaf/root callout (join + create phrasing)
      SettingsDialog.tsx   // service URL + token, Steam folder override
      ui/                  // button, input, select, dialog, badge (share with reliquary if a
                           //   shared package is extracted; otherwise copy)
    test/
```

Go side: `cmd/companion/server.go` serves `web/companion.Dist()`
instead of the embedded `ui/`; the local API (`/api/state`,
`/api/links*`, `/api/browse`, `/api/savepath/*`, `/api/artwork`,
`/api/savehints`, `/api/config`, `/api/hide`, `/api/discover`,
`/api/sync/refresh`) is unchanged. The single-binary,
no-installer shape is frozen: the frontend must embed; no dev-server
dependency at runtime. GitHub releases and the reliquary image bundle
carry the same exe — the release build gains the npm stage.

**Shared design language:** if the reliquary rebuild has landed, lift
its tokens/`ui/` primitives; extracting a shared `web/vaultkit`
package is optional and only worth it once both apps exist — do not
block either rebuild on it. (Dependency rules: `web/*` packages may
share with each other; none of them import game modules.)

## Screens (from the mock)

1. **Header bar** replaces the Connection panel: connection dot,
   "Connected as", freshness ("up to date" / "synced 40s ago"),
   Sync now, and a settings gear opening SettingsDialog (service
   URL + token + Steam folder). Version pair (companion · service)
   moves to a slim footer with the last sync action.
2. **Your worlds**: rows with cover thumb, custody chip (You hold /
   Free / Held by X / expired), the linked folder path, and
   state-driven actions exactly as `linkedRow()` encodes them.
3. **Shelf**: the signature stays — linked tiles in colour with a
   gold ring and the world name as caption, unlinked greyscale,
   hover un-greys; hidden entries collapse into a dashed
   "N hidden — show them" tile. Rescan + "Link a folder by hand" sit
   in the section header; the hint-note and collapsible scan trail
   sit under the grid.
4. **Link dialog**: candidate folders labelled with their source,
   inline FolderBrowser, world select (join vs create-new with name +
   seed checkbox), SplitExplainer, and errors rendered inside the
   dialog.
5. **First-run screen**: when not connected, the page shows the
   connect card and the game-finding card side by side with the scan
   trail visible — setup is a deliberate state, not an empty page
   with forms at the bottom.

## Behavior that must survive (the port checklist)

Each bullet traces to a comment in the current file; each gets a test.

- **Shelf memoization**: rebuild tiles only when the shelf signature
  (games, links, art keys, selection, show-hidden) changes — naive
  re-render made every cover flicker on each 5 s poll. React mostly
  solves this, but `<img>` remounts still flicker: key tiles by
  `gameKey` and never remount on poll.
- **Scan trail open-state is the player's**: it must survive polls;
  a fresh scan that found nothing opens itself; first render defaults
  closed-on-success.
- **Modal survives the poll**: form state must never be clobbered by
  a background refresh (the old design guaranteed this by keeping
  forms only in the modal; controlled React inputs must not be reset
  from poll data).
- **Errors render where the player looks**: link failures inside the
  dialog (`linkError`), never only the shared status line. Missing
  save folder is caught client-side before the round trip.
- **The split/leaf model**: joining a world with a recorded
  `savePath` resolves root+leaf via `/api/savepath/resolve`
  (create on submit only); creating a world shows the split from
  `/api/savepath/split` and records the leaf. The explainer's two
  phrasings (join: "will use/create"; create: "recorded as the
  folder…") both survive.
- **Art and hints fetch on game-set change, never on the timer** —
  the boot-time-only fetch bug made good IGDB credentials look dead.
  Covers/hints failing is silent (decoration / improvement).
- **One cover renderer** for tile and thumb (`coverHTML`), with the
  broken-image fallback to a name tile.
- **Per-panel failure isolation**: one panel throwing must not blank
  the others (error boundaries per section, with the panel named).
- **Dirty-input guard**: URL/steam-folder inputs are not overwritten
  by poll data while focused or edited.
- **Whole identity key** (`app:<id>` else `name:<lower>`) shared by
  art, hide-list and server — keep one `gameKey` in `lib/types.ts`.
- **Attribute-escaping class of bug** (`jsattr`) disappears with JSX —
  add a regression test anyway for paths with quotes/backslashes
  reaching the browser and link APIs intact.
- Confirm texts and button labels carry semantics ("kept and flagged,
  not lost"; "Nothing is deleted") — port them verbatim.

## Phases

Same gates as the consoles: `go build ./... && go vet ./... && go test
./...`, `./scripts/checkbounds.sh`, `cd web/companion && npm test`.

- **Phase 1 — scaffold.** `web/companion` toolchain, HeaderBar +
  footer, read-only worlds list and shelf from `/api/state`, first-run
  screen. Serve embedded from `cmd/companion` behind
  `COMPANION_LEGACY_UI=1` fallback to the old page.
- **Phase 2 — parity.** Link/unlink dialogs, folder browser, split
  explainer, settings dialog, hide/show, rescan, sync-now, art +
  hints, scan trail. Port checklist lands as tests here.
- **Phase 3 — cutover.** Remove `cmd/companion/ui/` and the flag;
  release workflow and the reliquary image build the frontend; smoke
  on a real Windows machine (tray open, discovery, link, checkout/
  check-in round trip against a reliquary).

Frozen API: the local port (8377), config file location, tray
behavior, and every `/api` route stay as they are — this rebuild
changes rendering only.
