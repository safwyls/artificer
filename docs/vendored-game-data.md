# Vendored game data — provenance and refresh chore

The frontend ships snapshots of Palworld game data. They drift with every
game patch, so this is a **recurring chore**, not a one-off: re-check after
each significant Palworld update (the ones that add pals or rebalance
stats), or when players report a missing pal/skill or an off stat estimate.

The code degrades gracefully for unknown ids — humanized names, "no stats
vendored", hidden icons — so drift is cosmetic, never a crash. That's by
design; keep it that way when regenerating.

## What's vendored, and from where

All files live in `web/src/data/`:

| File | Contents | Source |
|------|----------|--------|
| `palDex.json` | id → name, elements, rarity | palworld-server-manager (MIT), which sources palworld-save-pal's English localization |
| `palStats.json` | id → hp, stomach | same |
| `passiveSkills.json`, `activeSkills.json` | code → name/description | merged from deafdudecomputers/PalworldSaveTools (MIT) |
| `passiveTiers.json` | passive code → tier | same |
| `palCombat.json` | id → [hp, shotAttack, defense, 3 friendship rates] | palworld-save-pal data, calibrated (see `web/src/lib/stats.ts` header) |
| `palPassives.json` | passive code → [atk%, def%, hp%] | same |
| `breeding.json` | breeding combination table | palworld-save-pal |
| `palDeck.json` | id → Paldeck label ("94", "94B") | palworld-save-pal `pals.json` (`pal_deck_index`); see `web/public/pal-icons/README.md` for the transform |
| `mapPois.json` | map POI world coordinates by kind; fast travel + watchtower entries carry their English names (`[x, y, name]`) used as "near X" landmarks for bases | palworld-save-pal `fast_travel_points.json` (split on the `UnlockMapPoint` class) joined with `l10n/en/fast_travel_points.json` by GUID, and `map_objects.json` (dungeons, alpha/predator spawns), rounded to whole units |
| `items.json` | item id → name, category, rarity, weight, icon, description, and the gear figures the inventory view shows (max durability, magazine size, attack, defense, built-in passives) | palworld-save-pal `items.json` joined with `l10n/en/items.json`; see `web/public/item-icons/README.md` |

Pal icons: `web/public/pal-icons/` — see the README there. Item icons:
`web/public/item-icons/`, same arrangement and its own README. Map textures
(`web/public/palworld-map.webp`, `palworld-treemap.webp`) are vendored the
same way: © Pocketpair, Inc., credited on-screen in the map view — see
`web/public/README.md` for the fork/redistribution considerations.

## Constants that drift with game patches

These are hand-maintained and must be re-verified against the current game
version whenever the caps change:

- **Level cap** — `max={60}` in `ServerCalculators.tsx` (Level field).
  History: 50 → 55 → 60; Pocketpair raises it in major updates.
- **Soul rank cap** — 20 (+3% each) in `web/src/lib/stats.ts` (`soulMult`)
  and the Calculators soul fields. Went 10 → 20 with Large Pal Souls.
- **Trust/bond rank cap** — 10, and the `FRIENDSHIP_THRESHOLDS` table in
  `stats.ts` (vendored from PalworldSaveTools).
- **Condenser stars** — 4 (+5% each).
- **`TRUST_SCALE = 0.85`** in `stats.ts` — empirical calibration; re-check
  against in-game numbers if a patch touches the bond bonus.
- **Uncatchable deck entries** — `UNCATCHABLE_DECK_LABELS` in
  `web/src/lib/paldex.ts`: Paldeck numbers the game lists but never lets a
  player acquire, excluded from completion so 100% stays reachable and the
  missing-list stays actionable. Currently just #204 Astralym. A patch that
  makes a raid boss catchable (or adds an uncatchable one) has to be
  reflected here by hand — the save gives no catchability flag.
- **Passive inheritance rates** — `web/src/lib/inheritance.ts`: the
  40/30/20/10 inherit-count weights, the matching random-passive roll, and
  the 4-slot cap are community reverse-engineered (the model every public
  breeding calculator implements), not official. Re-verify if a patch
  touches breeding.

## Local touch-ups to preserve on refresh

- `palDex.json`: `plantslime_flower` is renamed to **"Gumoss (Special)"** —
  upstream names it plain "Gumoss", identical to the base pal, which made
  the #12B entry in the Paldex missing-list read as base Gumoss missing.
  It's the only base/variant name collision in the catalog; keep it fixed.
- **Known drift (as of 2026-07)**: the Yakushima creatures
  (`YakushimaMonster001/_Blue/_Pink`, `YakushimaMonster002`,
  `YakushimaBoss001…`) exist in real saves but are absent from
  `palDeck.json`/`palDex.json`. They currently fall out of Paldex and
  species math silently; pick them up on the next catalog refresh.

## How to refresh

1. Pull the current catalogs from the upstream repos listed above (their
   data folders are JSON; the vendored files keep upstream's shape, minus
   fields we don't read).
2. Diff against the current files — additions are safe; renames matter
   (a renamed key silently falls back to the humanized id).
3. Spot-check in the UI with a real save: a newly-added pal should show
   name, icon, elements, and effective stats.
4. Re-verify the constants above against patch notes.
5. Update the attribution footers in `ServerPlayers.tsx` and
   `ServerInventory.tsx` if sources change.

## Related upstream watch

`palworld-save-tools` (Python, powers the backend save extractor — pinned
in the Dockerfile and CI) and `pyooz` have their own pins; upstream PR #215
(native PlM support in palworld-save-tools) would let the pyooz shim retire.
Check those pins on the same cadence.
