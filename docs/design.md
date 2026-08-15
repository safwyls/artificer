# Flamekeeper — design plan

The design source for the Flamekeeper frontend, written before the code
(per the repo rule). The mock-first workflow of the siblings is replaced
by this document plus the implemented theme; treat the tokens here as the
contract.

## Subject, audience, job

An ops console for a private Enshrouded server: one maintainer plus a
handful of friends, glanced at on a second monitor or a phone. The page's
single job: *is the Flame lit* — server up, who's around it — with
power/config one step away. Enshrouded's world is Embervale: a ruined
kingdom drowned under the Shroud (a grey-green fungal fog that owns the
lowlands), survivors holding the high ground, and the sacred Flame as the
thing that holds the fog back. The dashboard adopts that thesis
literally: **the interface is a terrace above the fog; the Flame is the
server.**

## How it must differ from its siblings

Same shell, three consoles, and they will sit in adjacent browser tabs.
Wildskeeper is night-navy + brass structure + rune-cyan live-state +
parchment text + Cinzel (Roman caps) + clip-notch corners and brass
corner brackets. Palcon is Palworld-warm. Flamekeeper must be
recognizable at a squint as neither:

| Axis | Wildskeeper | Flamekeeper |
|---|---|---|
| Ground hue | night navy (blue-cast) | shroud moss (green-grey-cast) |
| Structure | brass (gold) | weathered stone (ash-beige, quiet) |
| Live state | rune cyan (teal) | flame azure (blue-white) |
| Danger | ember orange | spore red (dusty crimson) |
| Text | warm parchment | cool bone |
| Display face | Cinzel (Roman inscription) | Grenze Gotisch (textura blackletter) |
| Signature | brass corner brackets + 45° clipped corners | the fog line + the flame sigil |

## Palette (the `fk.*` literals)

Named, hex, role. The rule — and it is a rule, not a vibe: **stone is
structure and interaction, flame is reserved for live/active state and
focus, spore for danger.** Nothing else gets to glow.

| Token | Hex | Role |
|---|---|---|
| `fk.void` | `#101512` | page ground — moss-black, green-cast (vs the siblings' blue-cast) |
| `fk.fog` | `#182019` | raised/muted surfaces; the fog gradient's body |
| `fk.panel` | `#1e2822` | cards ("terraces") |
| `fk.edge` | `#2d3b32` | borders |
| `fk.stone` | `#98917c` | structure: buttons, section rules, secondary emphasis |
| `fk.stonehi` | `#c6bda4` | structure highlight: panel titles, active nav |
| `fk.flame` | `#7fc3f0` | LIVE: running state, focus ring, links, the lit flame |
| `fk.flamehi` | `#d3ecff` | the white-hot core: numbers mid-glow, sigil core |
| `fk.flamedim` | `#31536b` | flame at rest: selection bg, subdued live accents |
| `fk.spore` | `#c95a4d` | danger: destructive buttons, crashed state, shroud warnings |
| `fk.sporedim` | `#66312a` | danger surfaces/borders |
| `fk.bone` | `#e2ded0` | text — cool bone, deliberately less golden than parchment |
| `fk.lichen` | `#87947e` | muted text — grey-green, carries the fog cast into type |
| `fk.ok` | `#82b378` | success toasts only (live-state is flame's, not green's) |

Semantic mapping onto shadcn vars: background=void, card/popover=panel,
muted=fog, border/input=edge, primary=stone, accent=stonehi,
ring=flame (focus is live-state territory, same doctrine as wildskeeper's
rune ring), destructive=spore, foreground=bone, muted-foreground=lichen.

## Type

- **Display: "Grenze Gotisch"** (Google Fonts, 500/600/700) — a textura
  blackletter built as a display companion to a text family, i.e. a
  medieval-German voice for a medieval-German studio's game. Used with
  hard restraint and **never uppercased** (blackletter caps are
  illegible): the wordmark, the hero server name, page H1s, the big
  numbers on stat tiles. If it appears more than ~5 times on a screen,
  that's a bug.
- **Body: "Karla"** (400/500/700, + italic) — a grotesque with just
  enough quirk to not read as system-default, highly legible at data
  sizes, clearly not the siblings' Alegreya Sans.
- **Utility/mono: "IBM Plex Mono"** (400/500/600) — logs, ids, ports,
  paths.
- Panel titles: Karla 600, small size, wide tracking, `fk.stonehi` —
  quiet stone lintels; the blackletter stays reserved for identity
  moments. Labels/eyebrows: 11px Karla, `0.14em` tracking, `fk.lichen`.

## Layout

Inherited shell (server rail, subnav, content column) — the three
consoles deliberately share bones. What changes is the skin and the
Overview hero: the server name in Grenze Gotisch sits on a horizon —
a 1px stone rule that fades out at both ends — with the flame sigil at
its left and the vitals row beneath. No corner brackets, no clipped
corners: terraces are calm rectangles, radius 4px.

```
┌ rail ┐ ┌────────────────────────────────────────────┐
│ ◆    │ │ (sigil) Emberhold            ~~ horizon ~~ │
│ ◆    │ │ up 3d 4h · 4/16 flameborn · port 15637     │
│      │ │ ┌ power ─────────┐ ┌ vitals ─────────────┐ │
│      │ │ └────────────────┘ └─────────────────────┘ │
│      │ │ ┌ log preview ───────────────────────────┐ │
│      │ │ └────────────────────────────────────────┘ │
└──────┘ └────────────────────────────────────────────┘
         ░░░░░░░░░░░░░ the fog line ░░░░░░░░░░░░░░░░░░░
```

## Signature (two halves of one idea)

1. **The fog line.** A fixed gradient pooled at the bottom of the
   viewport — `fk.fog` rising out of `fk.void`, a few hundred px tall,
   opacity low enough to read as atmosphere, not vignette. Panels get a
   1px *top inset highlight* (bone at 5% alpha): stone catching light
   from above while the fog sits below. Static by design (no drift
   animation): a monitoring page must be visually silent when nothing is
   happening.
2. **The flame sigil.** Server status as a brazier glyph: a flame in
   `fk.flame` with a `fk.flamehi` core when running (slow 3–4 s breathing
   flicker, stilled under `prefers-reduced-motion`); an unlit grey wisp
   when stopped; a `fk.spore` guttering glow when crashed. Around it, a
   ring of up to 16 pips — the player slots, filled pips being Flameborn
   currently *around the fire*. This replaces wildskeeper's six-segment
   rune hex, and 16-around-a-circle is the honest shape of Enshrouded's
   slot model.

## Voice

Present-tense, player's-side words: servers are *raised*, players are
*Flameborn*, the join password is *the word at the gate* — no, restraint:
theme nouns are allowed exactly twice (Flameborn for players, "Raise a
server" inherited); everything else is plain ops English. Buttons say
what they do ("Save changes", "Stop server"). Errors explain and point;
501 reasons come from the backend and are shown verbatim — they already
say where each ability really lives.

## Self-critique against the defaults

- *Near-black + single acid accent?* No — green-cast ground, a
  three-role color system (stone/flame/spore) plus bone, and the accent
  is a reserved state color, not a brand wash.
- *Warm-cream + terracotta serif?* No — dark, and the terracotta-adjacent
  spore red is locked to danger only.
- *Broadsheet hairlines / zero-radius?* No — soft-edged terraces, one
  horizon rule as a deliberate single flourish.
- *Same-as-sibling?* The table above is the checklist; every axis moved.
- *The risk taken:* a blackletter display face on an ops dashboard.
  Bounded by the never-uppercase rule and the ~5-uses budget; if it
  fights the data it retreats to wordmark + hero only.
- *Generic-fantasy risk:* the fog line could read as a mere vignette.
  It earns its place by being literal (the Shroud below, the terrace
  above) and by pairing with the top-light on panels — a physical model,
  not a filter.
