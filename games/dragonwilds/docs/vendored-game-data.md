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

Both files live in `web/wildskeeper/src/data/`. (The companion app once
carried byte-for-byte mirrors for its local character page; that page
retired with the relay — `docs/companion.md` — so the frontend copies
are the only ones now.)

| File | Contents | Source |
|------|----------|--------|
| `itemNames.json` | item persistence id → display name (~1,400 entries) | [RSDWArchive/RSDWTools](https://github.com/RSDWArchive/RSDWTools) `data/items` + `data/consumables` — per-asset JSON extracted from the game's paks, each carrying `PersistenceID` and `Name.SourceString`; reduced to the id/name pair only |
| `skillNames.json` | skill persistence id → skill name (12 entries, Agility and Fishing included) | same archive's `data/skills` `SKILL_*.json` assets (`PersistenceID` per skill) |

An earlier revision used the community DWCharacterEditor catalog (792
items, 10 skills); the RSDWTools extract supersedes it — every id the old
catalog knew appears in the new one. Neither upstream publishes a license
file; only the factual id → name pairs are taken — the game's own
identifiers and English display strings — not the projects' code.

## What the ids are

Both id kinds are the game's *persistence ids*: 22-character
base64url-flavoured strings (`P3_Aq0nAXu5dlFuBNGgyaw` is the Abyssal
Whip). They are what character records in the save reference — the
`ItemData` field of an inventory slot and the `Id` field of a skill entry —
so the maps join directly against what `dwsave` extracts.

## Deliberately not vendored

- **Item stats** (weight, stack size, damage): the console shows what a
  character carries, not a build planner. Names suffice.
- **The game's raw XP table**: levels are computed in code
  (`web/wildskeeper/src/lib/xp.ts`, mirrored in the companion page) from
  a formula **calibrated against a real character** (2026-08-20): a
  player's in-game skill panel supplied twelve exact XP→level pairs
  across levels 2–40, and exactly one clean formula matches all twelve —
  the classic RuneScape curve with its divisor changed from 4 to 10
  (level 99 = 5,213,772 XP). The classic-divisor version shipped first
  and read levels visibly low; the observations are pinned as tests so a
  drift shows up as a failure. Remaining caveat: the wiki describes a
  bend above level 93 (linear bridge, then quadratic) that the
  observations cannot reach — recalibrate from a high-level screenshot +
  save pair when one exists. The exact XP always rides on hover.
