# Vendored game data — provenance and refresh chore

The wildskeeper frontend ships two id → display-name snapshots so the
save-derived character view reads as items and skills rather than opaque
persistence ids. Like palcon's equivalents
(`games/palworld/docs/vendored-game-data.md`), they drift with game
patches: re-check after significant Dragonwilds updates, or when a
character's inventory shows raw ids for items that plainly exist.

The code degrades gracefully for unknown ids — the raw id renders in
monospace with a "no name vendored" hint — so drift is cosmetic, never a
crash or a wrong name. Keep it that way when regenerating.

## What's vendored, and from where

Both files live in `web/wildskeeper/src/data/`:

| File | Contents | Source |
|------|----------|--------|
| `itemNames.json` | item persistence id → display name (792 entries) | the community DWCharacterEditor catalog (`data/ItemID.json`, extracted from the game's own data tables via pak extraction), reduced to the id/name pair only |
| `skillNames.json` | skill persistence id → skill name (10 entries) | same project's `data/SkillID.json` |

The upstream project publishes no license file; only the factual id → name
pairs are taken — the game's own identifiers and English display strings —
not the editor's code or the catalog's stats columns.

## What the ids are

Both id kinds are the game's *persistence ids*: 22-character
base64url-flavoured strings (`P3_Aq0nAXu5dlFuBNGgyaw` is the Abyssal
Whip). They are what character records in the save reference — the
`ItemData` field of an inventory slot and the `Id` field of a skill entry —
so the maps join directly against what `dwsave` extracts.

## Deliberately not vendored

- **Item stats** (weight, stack size, damage): the console shows what a
  character carries, not a build planner. Names suffice.
- **The game's own XP → level table**: levels shown in the console are
  derived in code (`web/wildskeeper/src/lib/xp.ts`) from the classic
  RuneScape 1–99 curve — a deliberate assumption (maintainer's call,
  2026-08-19) riding on 99 being the family cap. The wiki documents a
  game-specific tail (94–95 linear bridge, 96–99 quadratic), so a level
  in that band can sit one off the game's own; the exact XP stays on
  hover so the derived number never hides the true one. If the game's
  exact table gets vendored some day, it replaces the formula there and
  this caveat goes away.
