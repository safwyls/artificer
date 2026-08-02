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
| `structures.json` | building id → build-menu name (`n`), category (`c`, upstream's `type_b`) and icon name (`i`), for the Storage view's container labels, its storage/farm/station grouping and its row icons | palworld-save-pal `buildings.json` joined with `l10n/en/buildings.json` by key; 489 entries trimmed to those three fields |
| `bossFights.json` | boss record key → the fight's in-game title, where it happens, and `[level, HP]` per difficulty, for the Achievements view's fight dialog | [paldb.cc/en/Tower](https://paldb.cc/en/Tower) and [paldb.cc/en/Raid](https://paldb.cc/en/Raid), both datamined — see the note below on why not the guide sites |

Pal icons: `web/public/pal-icons/` — see the README there. Item icons:
`web/public/item-icons/`, and structure icons `web/public/structure-icons/`,
same arrangement and their own READMEs. Map textures
(`web/public/palworld-map.webp`, `palworld-treemap.webp`) are vendored the
same way: © Pocketpair, Inc., credited on-screen in the map view — see
`web/public/README.md` for the fork/redistribution considerations.

Not in `web/src/data/`, but vendored the same way: **`STATUS_NAMES` in
`internal/palsave/extract_pals.py`** maps the Japanese stat names every save
uses — whatever language the server runs in — to English. Taken from
palworld-save-pal's `STATUS_NAME_MAP` (`psp-core/src/domain/player.rs`), which
is the full 18-entry set; the relic-granted "adventure" stats arrived with
Palworld 1.0, so a patch that adds another will show up in the UI as an
untranslated Japanese label until this table is refreshed. Collecting the list
from a sample save is how one got missed before — a stat nobody had spent a
point on simply wasn't there to notice.

The frontend's display order for those stats lives in `ADVENTURE_STATS`
(`web/src/pages/ServerInventory.tsx`) and mirrors the same source, so every
player's build panel reads in one order.

Also not in `web/src/data/`: the roster tables in **`web/src/lib/achievements.ts`**.
No names are vendored there — `palDex.json` already carries them all, under
keys the record data doesn't use. What's hand-maintained is only the *join*:

| Table | Maps | Refresh trigger |
|---|---|---|
| `PALPAGOS_TOWERS` | the eight `BOSS_BATTLE_NAME_<region>Boss` towers → a `gym_*` catalog id | A game update adds a faction tower |
| `PANTHALUS` | the one fight between the towers and the World Tree | — |
| `WORLD_TREE_RUN` | the three `WorldTreeMiddleBoss<n>` dungeon bosses, beatable in any order | A game update adds another gated set |
| `ASTRALYM` | the last fight, gated on all three above | — |
| `RAID_ROSTER` | `PalSummon_<pal>` → a `raid_*` catalog id | A game update adds a summonable raid boss |

`TowerBossDefeatFlag` is one map holding a whole progression, which is why
there are four tables rather than one — the tiers are what the hero draws:

```
Palpagos Islands towers  →  Panthalus  →  World Tree run (any order)  →  Astralym
```

The save backs the split rather than just the labels: every tower carries a
`Tower_<region>` area flag, where Panthalus gets its own `BOSS_KingWhale` one.
`BOSS_CHAIN` flattens all four back into progression order for counting and for
picking the next fight the group can close out.

The Forbidden Laboratory has no catalog portrait — it's a place, and a gauntlet
of modified pals rather than one boss. It borrows Cattiva's outline, flattened
to black under a red rim, the way the game presents those fights. Cattiva and
not Lamball because at portrait size a silhouetted Lamball is a featureless
blob; the ears have to survive. If you swap the stand-in, check the silhouette
at 44px before trusting it.

`BOUNTY_ROSTER` needs no maintenance — the 34 human bounty targets are derived
from every `boss_*` key in the catalog at module load, so refreshing
`palDex.json` refreshes the roster and its denominator together.

Both tables render keys they don't recognise rather than dropping them: an
unknown tower is listed under the run, an unknown raid boss gets its own row.
That's the signal to add a line here.

Every key in all four tables is read off a real save record. What is **not**
read off a save is which `WorldTreeMiddleBoss<n>` is Silvance, which is
Dandilord and which is the Laboratory: the catalog has no `worldtreemiddleboss`
entry, so nothing vendored can say. Those three names are the only unverified
labels on the page, and a save can never settle them — only the game can.

### Why bossFights.json comes from paldb and nowhere else

The guide sites disagree with each other about tower boss levels, and some of
them are simply wrong: one popular list has Lyleen at 25, Orserk at 40 and
Faleris at 45, where the datamined values are 20, 30 and 40. Another prints an
element column that contradicts our own `palDex.json` — Orserk as Ice, Saya &
Selyne as Electric, Auri & Shaolong as Dark — and its own comment section
disputes the last one. [paldb.cc](https://paldb.cc/en/Tower) is datamined
rather than written up, and it agreed exactly with the one guide that published
HP figures, so it is the only source used here. Prefer it on the next refresh.

`bossFights.json` deliberately carries **no elements and no weaknesses**:

- Elements are already in `palDex.json` for every boss form, and they match
  paldb — including the awkward ones, like Saya & Selyne's Dark/Normal and
  Astralym having none at all.
- Weaknesses are computed by `elementCounters` in `web/src/lib/paldex.ts` from
  the element chart. A chart is a rule; copying it into thirteen rows is
  thirteen chances to get it wrong, and it would have to be re-checked every
  time a boss is added.

The element glyphs in `web/src/components/ElementIcon.tsx` are **not** vendored
either. Eight are lucide icons — the icon language the rest of palcon already
speaks — and Dragon is drawn to match, because lucide has no dragon. Nothing of
Pocketpair's is redistributed for them, and they take the element's colour, so
a glyph tints and scales where a lifted PNG would not.

The Laboratory is the one entry whose element the dialog refuses to state. It
borrows Grizzbolt's portrait because the first wave really is a Highly Modified
Grizzbolt, but the fight is eight different pals — so it prints its waves
instead of a matchup that would be wrong about seven of them.

Two joins are deliberately *not* attempted. The bare field-alpha spawner ids
(`81_1_grass_FBOSS_20` — about two thirds of them) name no species anywhere in
the save, so they're counted rather than named; only the ones that carry a
species in the id (`..._FBOSS_FlameBuffalo`) resolve. And
`FastTravelPointUnlockFlag` is a count, never a percentage: it runs higher than
the 141 fast-travel points in `mapPois.json`, so it evidently covers other map
points too, and a denominator would be invented.

### Why structures.json is worth the refresh

The Storage view names the chest an item is sitting in, and a wrong name sends
someone to the wrong chest. These were hand-written from memory first, and the
guesses were wrong in ways the UI could never reveal: `ItemChest_02` is the
**Metal Chest**, not the "Iron Chest"; `BlastFurnace3` is the **Electric
Furnace**, not an "improved" one; `WorkBench_SkillUnlock` is the **Pal Gear
Workbench** and has nothing to do with skill fruit. Prefer the catalog over
judgement here, and let unknown ids fall back to the humanized id.

Upstream covers buildings only. The world's own objects — treasure chests,
ground drops, wild eggs — are named by `WORLD_OBJECTS` in
`web/src/lib/structures.ts`, which is the easy half: nobody finds a treasure
chest by name. Coverage against a real save is 45 catalog + 12 hand-written,
with no humanized fallbacks.

Icons come from the same upstream (`ui/src/lib/assets/img/t_icon_buildobject_*.webp`).
Upstream has 505; only the 159 that can own an item container are vendored, since
a foundation's icon is weight the Storage view can never draw — see
`web/public/structure-icons/README.md` for the selection rule. `i` is recorded in
the catalog only when the file is actually vendored, so a missing icon is a row
without a picture rather than a failed request.

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
