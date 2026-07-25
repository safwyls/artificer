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

Pal icons: `web/public/pal-icons/` — see the README there. Map textures are
**not** vendored (copyrighted; user-supplied, gitignored — see
`web/public/README.md` and the memory note about never deleting them).

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

## How to refresh

1. Pull the current catalogs from the upstream repos listed above (their
   data folders are JSON; the vendored files keep upstream's shape, minus
   fields we don't read).
2. Diff against the current files — additions are safe; renames matter
   (a renamed key silently falls back to the humanized id).
3. Spot-check in the UI with a real save: a newly-added pal should show
   name, icon, elements, and effective stats.
4. Re-verify the constants above against patch notes.
5. Update the attribution footer in `ServerPlayers.tsx` if sources change.

## Related upstream watch

`palworld-save-tools` (Python, powers the backend save extractor — pinned
in the Dockerfile and CI) and `pyooz` have their own pins; upstream PR #215
(native PlM support in palworld-save-tools) would let the pyooz shim retire.
Check those pins on the same cadence.
