# Design systems

One design system per sub-app, written as data and rendered to standalone
preview pages. All five are done:

| System | Family | What it is |
| --- | --- | --- |
| `reliquary/` | vault | the save-custody service — dark, Georgia, gold |
| `companion/` | vault | the player-side client — **the same palette, verbatim** |
| `palcon/` | console | the Palworld console — the one light theme |
| `wildskeeper/` | console | the Dragonwilds console — brass, rune cyan reserved |
| `flametender/` | console | the Enshrouded console — stone, flame azure reserved |

```
design-system/
  build.mjs              # renders every system to <app>/previews/
  check.mjs              # parity against each app's own stylesheet and Tailwind config
  lib/chrome.mjs         # preview page furniture, token-agnostic
  lib/vault.mjs          # the palette + primitives reliquary and the companion share
  lib/console.mjs        # the shadcn-semantic kit the three consoles share
  <app>/system.mjs       # THE system: tokens, kit, cards — source of truth
  <app>/previews/        # generated, committed
```

```sh
node design-system/build.mjs          # rebuild the previews
node design-system/build.mjs --check  # fail if they are stale
./scripts/checkdesign.sh              # what CI runs: parity + freshness
```

## Two families, not five systems

The five apps are three design languages, and the module layout says so.

**The vault** (`lib/vault.mjs`) is reliquary and the companion. Their two
`index.css` files declare the same eleven tokens word for word, because they
are two halves of one custody flow rather than two products that happen to
match. Keeping the palette and the `vk-*` primitives in one module makes that
shared identity structural instead of a coincidence.

**The consoles** (`lib/console.mjs`) are palcon, wildskeeper and flametender.
All three map a literal palette onto shadcn's semantic tokens, and all three
`index.css` files give the same reason: so shared components need no
per-console work. The `con-*` kit is therefore written entirely against the
semantic tokens — feed it palcon's values and it comes out light and rounded,
feed it flametender's and it comes out mossy and square. Each console's
`system.mjs` adds only its theme and its flourishes.

Two of the three consoles reserve a colour for live state and nothing else —
rune cyan in wildskeeper, flame azure in flametender. That rule is why focus
rings, active server coins and load meters all share one hue, and it is the
single most useful thing to know when adding a component to either.

## What a system is

`<app>/system.mjs` holds:

- **tokens** — `colors` in the exact declaration form the app's `index.css`
  uses (RGB triples for the vault, HSL triples for the consoles, never hex
  strings, so Tailwind's opacity modifiers work), plus `literals` (the
  palette the Tailwind config names, for spots needing an exact hue),
  `scalars` (`--radius`), and `derived` (compounds and the variables the
  shared kit needs a theme to name).
- **chrome** — how the preview furniture reads this app's tokens. The chrome
  knows nothing about token names or whether they are RGB or HSL.
- **kit** — a token-only stylesheet the specimens are written against. Plain
  class names, not the app's Tailwind utilities.
- **cards** — grouped specimens, each carrying the *intent*, the *rules* that
  are easy to break, and the source files it was derived from.
- **source** — the stylesheet, Tailwind config and doc it answers to.

## Why the previews are standalone

Every generated page inlines its own tokens and kit. One file therefore
renders identically in three places that share no infrastructure: a browser
opening it off disk, a Claude Design project card (served under a strict CSP),
and a diff someone is reading as text. The only external request any page
makes is the consoles' Google Fonts stylesheet — the one host a published
Claude Design page may reach — and every face has a real fallback stack, so
the pages stay legible offline.

The cost is that the kits restate in plain CSS what the apps compose in
Tailwind, so the two can drift. Two guards, both in
`scripts/checkdesign.sh` and both in CI:

- `check.mjs` compares every token, scalar and literal hex against the
  stylesheet and Tailwind config the system names, because a stale token is a
  lie that reads like a record.
- `build.mjs --check` rebuilds and diffs, because committed generated files go
  stale silently — the trap `cmd/companion/rsrc_windows_amd64.syso` is already
  held to in the same workflow.

Nothing guards a kit's *class bodies* against the Tailwind they mirror. That
is why every card names its `Implemented by` paths: when you change
`ui/button.tsx`, `button.html` is the page to re-read.

## Pushing a system to Claude Design

The first line of every generated page is the `@dsCard` marker the Design
System pane indexes by, and `previews/_ds_manifest.json` is the same index as
JSON. To push one:

1. From an interactive terminal (`claude` in a real TTY — the design
   authorization flow needs one), run `/design-login`, then `/design-sync`.
2. Point it at `design-system/<app>/previews` as the local directory, and at a
   project of type `PROJECT_TYPE_DESIGN_SYSTEM` — that type is fixed at
   creation, so pushing to a regular project never makes it a design system.
3. Sync incrementally, one card at a time. Never wholesale-replace.

A Claude Code Web session cannot do step 1: `/design-login` has no TTY there,
so `DesignSync` refuses. Building and reviewing the bundles works anywhere;
only the push needs the terminal.

**Project pins.** `.design-sync/config.json` records which Claude Design
project a bundle belongs to, so a later sync updates the same project rather
than creating a second one. Today it pins **reliquary only** — the other four
have no project yet, and pushing one means adding its pin alongside. The
`"shape": "custom"` field is load-bearing: these bundles are hand-authored
from `system.mjs` through `build.mjs`, not produced by the storybook/package
converter, so a sync must upload `previews/` as it stands rather than trying
to re-derive it.

## Adding another app

1. `mkdir design-system/<app>` and write `system.mjs`. If it belongs to an
   existing family, import that family's kit; if it is a third language, add
   `lib/<family>.mjs` beside the other two.
2. Copy the real tokens out of `web/<app>/src/index.css` verbatim —
   `check.mjs` compares declaration strings, so the form matters.
3. Derive each card from a named component file and list it under `sources`.
   A card with no source is a drawing, not a design system.
4. `node design-system/build.mjs && ./scripts/checkdesign.sh`.
