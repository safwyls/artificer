# Flametender — design plan

The design source for the Flametender frontend, written before the code
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
corner brackets. Palcon is Palworld-warm. Flametender must be
recognizable at a squint as neither:

| Axis | Wildskeeper | Flametender |
|---|---|---|
| Ground hue | night navy (blue-cast) | shroud moss (green-grey-cast) |
| Structure | brass (gold) | weathered stone (ash-beige, quiet) |
| Live state | rune cyan (teal) | flame azure (blue-white) |
| Danger | ember orange | spore red (dusty crimson) |
| Text | warm parchment | cool bone |
| Display face | Cinzel (Roman inscription) | Grenze Gotisch (textura blackletter) |
| Signature | brass corner brackets + 45° clipped corners | the fog line + the flame sigil |

## Palette (the `ft.*` literals)

Named, hex, role. The rule — and it is a rule, not a vibe: **stone is
structure and interaction, flame is reserved for live/active state and
focus, spore for danger.** Nothing else gets to glow.

| Token | Hex | Role |
|---|---|---|
| `ft.void` | `#101512` | page ground — moss-black, green-cast (vs the siblings' blue-cast) |
| `ft.fog` | `#182019` | raised/muted surfaces; the fog gradient's body |
| `ft.panel` | `#1e2822` | cards ("terraces") |
| `ft.edge` | `#2d3b32` | borders |
| `ft.stone` | `#98917c` | structure: buttons, section rules, secondary emphasis |
| `ft.stonehi` | `#c6bda4` | structure highlight: panel titles, active nav |
| `ft.flame` | `#7fc3f0` | LIVE: running state, focus ring, links, the lit flame |
| `ft.flamehi` | `#d3ecff` | the white-hot core: numbers mid-glow, sigil core |
| `ft.flamedim` | `#31536b` | flame at rest: selection bg, subdued live accents |
| `ft.spore` | `#c95a4d` | danger: destructive buttons, crashed state, shroud warnings |
| `ft.sporedim` | `#66312a` | danger surfaces/borders |
| `ft.bone` | `#e2ded0` | text — cool bone, deliberately less golden than parchment |
| `ft.lichen` | `#87947e` | muted text — grey-green, carries the fog cast into type |
| `ft.ok` | `#82b378` | success toasts only (live-state is flame's, not green's) |

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
- Panel titles: Karla 600, small size, wide tracking, `ft.stonehi` —
  quiet stone lintels; the blackletter stays reserved for identity
  moments. Labels/eyebrows: 11px Karla, `0.14em` tracking, `ft.lichen`.

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
   viewport — `ft.fog` rising out of `ft.void`, a few hundred px tall,
   opacity low enough to read as atmosphere, not vignette. Panels get a
   1px *top inset highlight* (bone at 5% alpha): stone catching light
   from above while the fog sits below. Static by design (no drift
   animation): a monitoring page must be visually silent when nothing is
   happening.
2. **The flame sigil.** Server status as a brazier glyph: a flame in
   `ft.flame` with a `ft.flamehi` core when running (slow 3–4 s breathing
   flicker, stilled under `prefers-reduced-motion`); an unlit grey wisp
   when stopped; a `ft.spore` guttering glow when crashed. Around it, a
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

## The moderation surface (Phase 2)

Written before the code, same rule. Two editors over
`enshrouded_server.json`: role groups and the ban list.

**Two panels, two homes, because two permissions.** Roles carry join
passwords in the clear, so they live on **Configuration** under the
settings gate that already guards that file. The ban list carries no
credentials, so it lives on **Flameborn** under the moderation gate,
beside the roster it is about. Putting both on one page would mean a
moderator landing on a page whose first panel refuses them.

**Roles: group cards, not a CRUD table.** Each group is one card — name
(inline, Karla 600 bone), password (mono, masked, per-field reveal eye:
the doctrine the settings rows already use), then a row of capability
**chips** and a reserved-slots stepper. Delete is a single ✕ at the
card's right that only takes `ft.spore` on hover.

Chips rather than a checkbox grid because five booleans × N groups is a
thicket, and the question an operator actually has is *who has admin
here* — a badge row answers that at a scan. Lit chips take `ft.flame`
(capability on = active state, which is exactly flame's role); unlit
stay `ft.edge` outline. `Kick/ban` is the chip that means admin, so it
carries more weight than the other four and is the only one allowed
`ft.flamehi`.

**Bans: a short mono list.** One id per row, display name beside it when
the file's element format carries one, `Lift` on the right; an add row on
top taking a SteamID64. Both panels use draft-then-Save with a count on
the button, matching the settings table's rhythm — which also buys a free
undo before anything is committed.

**Ban from the roster.** The roster's Ban button has been dead-with-a-
reason since Phase 1. It becomes live for moderators, relabelled **"Add
to ban list"** so the label itself doesn't overpromise, with a confirm
that says the whole truth: this writes them to the list the game reads at
start; it does not remove them now. Kick stays dead — there is genuinely
no kick outside the in-game player menu.

**The overwrite warning.** While the server is up, the game owns
`bannedAccounts` too — its own ban UI writes it — so a file edit under a
live server can be overwritten when the game next persists. The panel
says so in `ft.spore` whenever the API reports the game running. This is
an unverified behavior stated as a risk, not a fact.

No new visual vocabulary: FtPanel terraces, the stone/flame/spore
three-role rule intact, no icons beyond the lucide glyphs already in the
tree.

### Self-critique of the moderation surface

- *A CRUD table with an Actions column, pencil and trash icons, and a
  modal per row?* The default shape, and rejected: inline cards, no
  modal, one destructive glyph that colors only on hover.
- *A checkbox grid for permissions?* Rejected for the chip row, above.
- *Immediate writes with a toast per action?* Rejected: draft + one
  Save, so a misclicked Lift is recoverable before it reaches the file.
- *Green for "permission enabled"?* Rejected — `ft.ok` is toast-only by
  the palette rule, and "on" is live-state, which is flame's job.
- *The risk taken:* an enabled Ban button that ejects nobody. Bounded by
  the label, the confirm copy and the restart note; if it reads as
  overpromising in use it goes back to disabled-with-reason.
- *The other risk:* passwords rendered in the clear on a page several
  people can reach. Bounded by the settings gate — which is the file's
  own gate, not a new one — and masked-by-default fields.

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

## Phase 2 addendum — presence, readiness, and the real slot count

The Steam query gave the console three facts it never had: how many
people are on *right now*, the server's own configured slot count, and
the build it is running. Plus one the log always had and the UI never
showed — whether the server has finished coming up. This is where they
land, and what was deliberately not built.

**Palette: nothing new.** The theme's three roles already cover this, and
the discipline worth keeping is that flame and spore each mean exactly
one thing. Flame is live-and-joinable; spore is absent-or-wrong. Neither
fits "up but not accepting joins yet" — flame would promise a join that
fails, spore would read as offline. So *starting* is drawn in neutral
stone with a slow pulse: the neutral says "no claim either way", the
motion says "this is temporary", and neither borrows a meaning that is
already spoken for.

**Type: unchanged.** State chips keep the established 11.5px uppercase
with tracking; the build stays mono, because it is an identifier and gets
read character by character when someone is comparing it to a friend's
version-mismatch error.

**Layout: no new regions.** Readiness is one word and belongs on the chip
row a player already scans; the slot count belongs in the sigil and the
Flameborn stat. A dedicated readiness panel would give a transient state
permanent furniture.

**Signature: the sigil stops lying.** `FlameSigil` draws lit-of-total and
its total has been the game's 16-slot hard cap, so a full 4-slot server
rendered as one-quarter lit. Feeding it the queried `maxPlayers` is the
highest-value change on the page — the signature element is the one thing
read at a glance, and it has been reading wrong.

### Self-critique against the defaults

- *A green/amber/red status pill?* That is the default move and it is
  refused: this theme has no traffic-light vocabulary, and importing one
  would flatten stone/flame/spore into a bootstrap alert set.
- *A new "Server health" card?* Refused. Every fact here has an existing
  home; a new card would be a container invented to hold one word.
- *A spinner on "starting"?* No. Nothing is loading — the server is
  working. A pulse says "in progress"; a spinner says "waiting on a
  request", which is a different and wrong claim.
- *Surfacing `transport: "agent+a2s"`?* Tempting, since the backend
  distinguishes them. Refused as jargon on the hero; it earns a mention
  only as a hint under the count, where "is this number trustworthy" is
  the actual question.
- *The risk taken:* a count from the game that can exceed the roster the
  log can name. Rather than hide the gap or invent placeholder rows, the
  roster says how many it can't identify. If that proves confusing in
  use, the fix is naming them via A2S, not suppressing the count.
