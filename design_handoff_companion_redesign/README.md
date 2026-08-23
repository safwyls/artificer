# Handoff: Reliquary Companion — window redesign

## Overview

The Companion is the desktop app that syncs shared world saves from one player's machine.
This redesign reorganises its single window around **custody** — "can I take this world right
now?" — instead of around the installed-games scan that currently dominates the screen.

Five structural changes:

1. **Worlds is the whole page.** The installed-games grid moves to its own tab.
2. **Worlds are grouped by what you can do with them:** checked out to you → free to take → held
   by someone else. Grouping replaces per-row status prose.
3. **One primary action per row**, with a quiet secondary and an overflow menu for the rare and
   destructive verbs. This is the Reliquary one-primary-action rule.
4. **Sync state is reported in exactly one place** — the header (live dot + relative time). The
   scan trail, tried-paths list, Ludusavi note and build hashes leave the chrome entirely and
   live behind a `Diagnostics` link into Settings.
5. **Hold pressure is shown only when it is pressure.** Holds last 48h; a countdown appears on a
   held world only when it is under ~3h. Otherwise the row just names who has it.

### What was wrong with the current window

Recorded so the intent behind each change is clear:

- "Your worlds" — the reason the app exists — was one small card; the installed-games grid took
  roughly 70% of the window, and 11 of 13 tiles read "not linked". The dim tile treatment already
  said that, so the labels were pure noise.
- The world card had four equal-weight buttons (Check out & play / Check out / Edit / Unlink),
  with no way to tell "Check out" from "Check out & play".
- "nobody holds this world" restated the `Free` chip, which was itself stranded in the opposite
  corner from the world's name.
- Sync state was reported three times: header dot + "up to date", the bottom "you're up to date"
  line, and the scan trail.
- The full save path got a prominent well; build hashes sat in the footer. Both are diagnostics.
- Grid titles truncated with an ellipsis ("RuneScape: Dragon…"); the "4 entries hidden / show
  them" control was rendered as a fake game tile inside the grid; the grid had no search.

## About the design files

`Companion Redesign.dc.html` in this bundle is a **design reference written in HTML** — a
prototype showing intended look, copy and behaviour. It is not production code to copy.

The task is to **recreate these screens in the Companion's existing environment**, using its
established component patterns and its Reliquary style layer. The mock renders the Reliquary
tokens as literal hex values because it is standalone; in the app, use the real tokens
(`--gold`, `--mist`, etc. — the mapping is in *Design tokens* below).

The mock has a **state switcher strip above the window frame** (Worlds / Games library / First
run / Offline) and a **fake OS title bar**. Both are scaffolding for reviewing states — neither
ships.

## Fidelity

**High fidelity.** Colours, type, spacing, copy and hover states are final and are taken from the
Reliquary design system. Recreate the UI closely, but source every value from the app's existing
token layer rather than hard-coding the hexes below.

## Screens

### 1. Worlds (default)

The tab a returning user lands on.

**Layout** — vertical stack, window content padded `22px 28px 26px`, `22px` between groups.
Each group is a mono uppercase section label (10px, `0.12em` tracking) above a bordered panel
(`--panel`, 1px `--edge`, 8px radius) containing full-width world rows separated by 1px
`--edge` lines.

**World row** — `display: flex; align-items: center; gap: 16px; padding: 14px 18px`, hover
background `--well`. Left to right:

| Part | Spec |
|---|---|
| Cover | 54×72, 5px radius, 1px `--edge`, `--fill-cover` gradient, game name centred at 9.5px `--mist`. Real cover art replaces this when available; the fallback tile is the design-system pattern. |
| Name + game | Row 1: name 17px bold `--parchment`, then game name 12px `--rune`, baseline-aligned, `gap: 10px`. |
| Custody line | Row 2: custody chip + one short sentence, 12.5px `--mist`. |
| Head meta | Right-aligned mono 11px `--mist`: `v41 · 1.8 GB · 18:02` — version, size, time. |
| Actions | Primary, quiet secondary, then a 3-dot overflow button. |

**Groups, chips and actions**

- **Checked out to you** — panel border is `rgba(201,168,96,0.45)` (gold at 45%) instead of
  `--edge`, so the one world you hold is findable at a glance. Chip: `Held` style (gold border,
  `--fill-held`, `--goldhi`) labelled **Yours**, with a closed-padlock icon. Sub-label on the
  section: "locked to this machine until you check it in". Custody line: "yours since 19:04 —
  47h left on the hold". Primary **Play** (with a play triangle), quiet **Check in**.
- **Free to take · 3** — chip `Free` (green border, `--fill-free`, `--ok`), open padlock. Line:
  "last played by rook". Primary **Check out & play**, quiet **Check out only**. The two labels
  are deliberately explicit: one launches the game, one only takes custody.
- **Held by someone else · 2** — chip `Held` in `--mist` on `--well` (grey, not gold: gold means
  *you* have it). Cover drops to `opacity: 0.75`. Line: "held by rook". When the hold has under
  ~3h left, an ember countdown badge follows it: 1px `rgba(212,115,94,0.45)`, 3px radius,
  `1px 7px`, mono 11px `--ember`, clock icon, text "2h 12m left". Actions are both quiet —
  **Ask for it back**, **Download a copy** — because there is no primary action available.

**Footer of the group stack** — a dashed `--edge` well on `--well`: "13 games installed on this
machine, 1 linked to a world." + link *Open the games library*. This is the only pointer to the
Games tab from Worlds.

**Overflow menu contents** (per row, using the design system's menu): World history, Open save
folder, Copy save path, Rename, separator, Unlink (danger). The raw save path lives here — it is
a debug fact, not a daily one.

### 2. Games

Setup, visited rarely. Same window chrome, own tab.

- **Toolbar row**: search field (340px max, magnifier icon inset 11px left, placeholder "Search
  installed games"); a segmented filter in a 1px `--edge` box with 2px padding —
  `All 13` (active: `--panel` fill, `--goldhi` text) / `Linked 1` / `Unlinked 12`; right-aligned
  **Rescan** and **Link a folder by hand…**, both quiet.
- **Linked · 1** section first, in gold mono label. Tile border `rgba(201,168,96,0.5)`, body on
  `--panel`, second line "1 world · FGI" in `--goldhi`. Putting linked games in their own section
  fixes the old grid, where the one linked tile was lost among twelve dimmed ones.
- **Not linked · 12** with sub-label "Reliquary knows a save location for 7 of these." Tiles at
  `opacity: 0.72`, hover to `opacity: 1` + `rgba(201,168,96,0.5)` border. Tile body: name 13px
  `--parchment` **wrapping to two lines** (no ellipsis), note 11.5px `--mist` — either "save
  folder known" or "pick the folder yourself" — and a `Link` affordance in `--goldhi` on the
  right, so clickability is stated on the tile rather than in a section subtitle.
- Grid: `repeat(6, 1fr)`, `gap: 14px`, cover area 132px tall, tile radius 6px.
- Hidden entries are a line of text below the grid, not a tile: "4 entries were hidden as
  duplicates or launchers. *Show them*".

### 3. First run

Shown when the machine has no linked worlds.

Centred 620px column, 60px top padding. Reliquary diamond mark (34px, `--gold`, 1.1 stroke),
title "No worlds on this machine yet" (21px `--gold`, `0.05em`), then one explanatory sentence in
14px `--mist` at `max-width: 46ch`: "A world is one save folder the vault holds for your group.
Link a game's save folder and Reliquary starts keeping its history."

Below it a three-step panel, one row per step, 1px `--edge` dividers. Each row: a 22px circular
step marker, a 14.5px `--parchment` title, a 12.5px sub-line, and an optional action.

1. ✓ green marker — "Signed in as safwyl@pm.me" / "4 people in this vault"
2. ✓ green marker — "Found 13 installed games" / "Across 4 libraries. Save locations known for 7
   of them." + quiet **Review** → Games tab
3. gold `3` marker, row background `--well` to mark it as current — "Link a game to make your
   first world" / "Pick the game and Reliquary suggests the save folder. You confirm it." +
   primary **Choose a game** → Games tab

Closing line, centred 12.5px: "Someone in your group already made worlds? *Join one instead*".

### 4. Offline

The service is unreachable. Confirmed behaviour: **the user keeps playing locally and the sync
queues.**

- **Banner** — 1px `rgba(212,115,94,0.5)` on `--panel`, struck-through wifi icon in `--ember`:
  "Working offline — the vault is unreachable" / "Keep playing the world you already hold.
  Everything you do is written locally and sent up when the vault answers again. Retrying in
  30s." + quiet **Retry now**.
- **Checked out to you — playable offline** — the held world, unchanged except the custody line
  ("the hold stands while you are offline — 41h left") and a second mono meta line "2 saves not
  yet sent". Primary **Play** stays enabled: this is the whole point of the offline state.
- **Queued to send · 3** — a compact row list (13px, `11px 18px` padding): mono time, what
  happened, mono size right-aligned. E.g. `20:14 · Save written to FGI · 18 MB`.
- **The other worlds are hidden**, with a dashed well explaining why: "custody can't be
  confirmed, so checking one out could collide with someone else." + *Show them read-only*.
  This was a judgement call — if the team prefers all six worlds visible and disabled, replace
  the well with the normal groups and disable every primary.

## Interactions and behaviour

- **Tab switching** — Worlds / Games / Activity / Conflicts / Settings. Active tab: `--goldhi`
  text + 2px `--gold` bottom border sitting on the header's 1px `--edge` rule (`margin-bottom:
  -1px`). Inactive: `--mist`, hover `--parchment`. Offline is a variant of Worlds, so the Worlds
  tab stays active while offline.
- **Conflicts tab** carries a count badge when non-zero: mono 10px `--ember`, 1px
  `rgba(212,115,94,0.5)` border, 3px radius. No badge at zero.
- **Row hover** — background to `--well`. Rows are not links; they contain buttons. The name and
  cover open the world's page.
- **Button hovers** (from the design system, `0.12s ease` on colour and border-color):
  primary `--gold` → `--goldhi` text and border; quiet `--edge` border → `rgba(201,168,96,0.6)`
  and `--mist` → `--parchment` text.
- **Focus** — the single Reliquary gold ring: `box-shadow: 0 0 0 2px <page ground>, 0 0 0 4px
  rgba(201,168,96,0.6)`, no outline. Applies to every button, tab and input.
- **Chip and primary are computed from one custody call.** They must not be able to disagree —
  derive both from a single `custodyOf(world)` result. This is the design system's rule and the
  main invariant to preserve in the implementation.
- **Sync now** shows a spinner in place of the dot and "syncing…" in the header; on completion,
  the relative time resets. Success and failure both raise a toast (design system `rq-toast`) —
  never silence.
- **Destructive verbs** (Unlink, Discard local changes) go through the confirm dialog, which
  states what the action costs rather than that it is irreversible.

## State

| State | Values | Notes |
|---|---|---|
| `view` | `worlds` \| `games` \| `firstRun` \| `offline` | `firstRun` is derived — no linked worlds — not user-selected. `offline` is derived from connectivity. |
| `worlds[]` | `{id, name, game, coverUrl, head: {version, size, savedAt}, custody}` | `custody` is `{state: 'free'\|'yours'\|'held'\|'expired', holder, expiresAt}` |
| `games[]` | `{id, name, coverUrl, linked, worldId, saveLocationKnown, hidden}` | `hidden` drives the "4 entries hidden" line |
| `sync` | `{status: 'idle'\|'syncing'\|'offline'\|'error', lastSyncAt, queue[]}` | `queue[]` is `{time, what, size}` — only rendered offline |
| `filter` | `all` \| `linked` \| `unlinked` + search string | Games tab only |

Derived, not stored: group membership (from `custody.state`), whether a countdown shows
(`expiresAt - now < 3h`), the footer counts, the Conflicts badge.

## Design tokens

All from the Reliquary design system — do not introduce new values.

| Token | Hex | Used for |
|---|---|---|
| `--ink` | `#100d17` | window ground, input fills |
| `--well` | `#14101d` | title bar, footer bar, row hover, current step row |
| `--panel` | `#1a1524` | every panel and card |
| `--edge` | `#2f2740` | every border and divider |
| `--parchment` | `#e8e0cf` | body and world names |
| `--mist` | `#948da3` | secondary text, labels, mono head meta, quiet buttons |
| `--gold` | `#c9a860` | headings, primary button, focus ring |
| `--goldhi` | `#e3c67f` | links, hover accent, held-by-you chip text |
| `--ok` | `#7fc46a` | free custody, live dot, done steps |
| `--ember` | `#d4735e` | offline banner, hold countdown, conflict badge, danger |
| `--rune` | `#9d7fc4` | game name tags |
| `--fill-free` | `#14200f` | Free chip fill |
| `--fill-held` | `#23180c` | Yours chip fill |
| `--fill-cover` | `linear-gradient(to bottom right, #221b2e, #14101d)` | cover fallback tile |
| `--primary-fill` | `linear-gradient(to bottom, #2a2416, #1e1a10)` | primary button fill |

Partial-alpha uses, all of `--gold`/`--ember`: `rgba(201,168,96,0.45)` gold panel border,
`rgba(201,168,96,0.5)` linked-tile border, `rgba(201,168,96,0.6)` quiet hover border and focus
ring, `rgba(212,115,94,0.45–0.5)` countdown border, conflict badge, offline banner.

**Type** — Georgia (`Georgia, Gelasio, "Times New Roman", serif`) throughout; system mono
(`ui-monospace, SFMono-Regular, Menlo, monospace`) for machine facts only: versions, sizes,
timestamps, counts, section labels, IDs. Sizes in use: 22 (wordmark) / 21 (empty-state title) /
17 (world name, bold) / 14.5 (tabs, step titles) / 13.5–13 (buttons, tile names) / 12.5
(secondary prose) / 12 (chips) / 11 (mono meta) / 10 (mono section labels, uppercase, `0.12em`).

**Geometry** — window 1440px wide, 10px radius; panels and wells 8px; tiles 6px; covers 5px;
buttons, chips-as-badges and inputs 4px; chips 999px. Padding: window content `22px 28px 26px`,
panel rows `14–15px 18px`, buttons `7px 14–16px`. Gaps: 22 between groups, 16 in a row, 14 in the
tile grid, 8–10 between inline items.

## Assets

**None shipped.** Every cover in the mock is the design system's fallback tile (`--fill-cover`
gradient + game name), because no cover art was available. The real app should use its existing
cover-art source and keep the fallback tile for misses. Icons are inline SVG in a Lucide-like
1.4–2px stroke style — substitute the app's existing icon set: play, refresh, gear, search,
padlock (open/closed), clock, wifi-off, 3-dot overflow, Reliquary diamond mark.

## Implementation checklist

Roughly in dependency order:

1. Add tab navigation to the Companion window; move the existing installed-games grid to the
   Games tab unchanged as a first step.
2. Introduce `custodyOf(world)` returning `{state, holder, expiresAt}`, and derive both the chip
   and the primary action from it. Nothing else should decide either.
3. Build the world row (cover, name + game, chip + line, mono head meta, actions) and group the
   Worlds tab by custody state with counts in the section labels.
4. Reduce each row to one primary + one quiet + overflow. Move Edit, Unlink, the save path and
   the folder actions into the overflow menu.
5. Delete the duplicate sync reporting: keep the header dot + relative time, remove the bottom
   status line and move the scan trail, tried paths and build hashes into Settings › Diagnostics.
6. Add the countdown badge with the under-3h threshold.
7. Rework the Games tab: search, three filters, linked/unlinked sections, two-line titles, the
   per-tile `Link` affordance, and hidden entries as a text line.
8. Add the first-run state, keyed on "no linked worlds".
9. Add the offline state: banner, queue list, hold-stands copy, hidden non-holdable worlds.
10. Confirm the focus ring is present on every interactive element, and that the mono/serif split
    matches the table above.

## Files

- `Companion Redesign.dc.html` — the full prototype, all four states. Open it directly in a
  browser; the strip above the window frame switches states.
- Design system: the Reliquary system attached to this project — `index.html` for the palette and
  intent, `world-card.html`, `app-shell.html`, `custody-chip.html`, `button.html`,
  `overflow-menu.html`, `cover-art.html`, `feedback.html`, `dialog.html` for the components this
  redesign composes.
