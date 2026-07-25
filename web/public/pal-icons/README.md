# Pal icons

Sprite icons for every pal, shown on the Player details view.

**These are Palworld game assets — copyright Pocketpair, Inc.**, not part of
this project. They're vendored here so a self-hosted deployment works without
extra setup, and the Player details view credits Pocketpair on screen. A fork
that redistributes Palcon should make that call deliberately rather than
inherit it.

## Source

Vendored from [palworld-server-manager](https://github.com/amantu-qbit/palworld-server-manager)
(MIT), path `assets/pal-icons`. The accompanying lookup tables in
`web/src/data/` come from the same repo:

| File | Upstream source |
| --- | --- |
| `web/src/data/palDex.json` | `src/data/palDex.json` — display name, elements, rarity |
| `web/src/data/passiveSkills.json` | `bridge/data/passive_skills.json` — passive id → English name |
| `web/src/data/activeSkills.json` | `bridge/data/active_skills.json` — active skill id → English name |

Those catalogs originate from [palworld-save-pal](https://github.com/oMaN-Rod/palworld-save-pal)'s
English localization data, which in turn derives from
[palworld-save-tools](https://github.com/cheahjs/palworld-save-tools). Both are
static id → display-name lookups; no code was copied from either project.

## Skill & passive descriptions

Active-skill descriptions (the `d` field in `activeSkills.json`) come with the
palworld-server-manager catalog above. Its passive catalog ships **names
only**, so the passive descriptions in `passiveSkills.json` are merged in from
[PalworldSaveTools](https://github.com/deafdudecomputers/PalworldSaveTools)
(MIT), path `resources/game_data/skills.json` — a static id → description
table. The merge keeps the localized names and only fills the `d` field,
matching on the internal skill id (`asset`); in-game `\r\n` line breaks are
collapsed to single spaces. See the commit that added them for the transform.

Descriptions that stayed blank are internal entries with no player-facing
blurb: passive work flags (`CollectItem_*`), Pal Sphere modifiers
(`SphereModule_*`), and boss-only active skills (`Unique_*Boss*`, `*_GYM_Act`,
`Unique_WorldTreeDragon_*`) that never appear on a player's own pals. The
lookup layer humanizes any such leftover id (`Unique_WorldTreeDragon_BigBang`
→ "World Tree Dragon Big Bang") so the UI never shows a raw internal name.

## Breeding & combat data (calculators)

The Calculators tab is backed by two more files derived from
[PalworldSaveTools](https://github.com/deafdudecomputers/PalworldSaveTools)
(MIT):

| File | Derived from | Contents |
| --- | --- | --- |
| `web/src/data/breeding.json` | `resources/game_data/breedingdata.json` | Every parent-pair → child outcome, as a dense upper-triangular table, plus the set of hand-authored "special" combos |
| `web/src/data/palCombat.json` | `resources/game_data/characters.json` | `[hp, shotAttack, defense, then the three per-level friendship/trust rates]` per species |
| `web/src/data/palPassives.json` | `resources/game_data/skills.json` | Passive code → `[attack%, defense%, hp%]`, only the effects the game applies `ToSelf` |

The breeding table is inverted from the upstream `child_to_parents_*` maps and
validated to match its `parent_to_children_formula` exactly, so the outcomes
are the game's, not a reimplemented formula.

`web/src/lib/stats.ts` was **calibrated against in-game numbers** (see the
commit that added it). Findings baked into the data:
- Attack uses **shot attack** — the game's displayed Attack ignores melee.
- Passives affecting the same stat stack **additively** (Legend +20% and Burly
  Body +20% give +40% defense), and only `ToSelf` `ShotAttack`/`Defense`/`MaxHP`
  effects count; `ElementBoost` and `ToTrainer` passives don't move the shown
  numbers, so they're excluded from `palPassives.json`.
- An Alpha (captured field boss, the save's `isBoss`) gets an **HP-only** bonus
  of `+rarity%` — a level-70 alpha Jetragon (rarity 20) shows +20% HP over its
  listed base, which is why the numbers looked off before this was modelled.
- Talents +30% max, condenser +5%/star, souls +3%/rank, trust via the vendored
  friendship rate (the one approximate term — the exact curve isn't published).
- palCombat keys are aliased to the palDex ids so a pal picked from the save
  resolves whether its id carries a `Predator_`-style prefix or not.

Verified exact on a spread of real pals across levels 1–70; the only estimate
left is the trust/bond bonus, which the UI calls out.

## Naming

A file is named for the pal's internal id, lowercased, with the `BOSS_` prefix
(which marks an alpha variant) stripped — so `BOSS_Anubis` and `Anubis` both
resolve to `anubis.webp`. `web/src/lib/paldex.ts` does that mapping and falls
back to the raw id whenever a lookup misses, so a pal added by a game update
still renders, just without art or a localized name.

## Refreshing

```sh
curl -sL https://github.com/amantu-qbit/palworld-server-manager/archive/refs/heads/main.tar.gz | \
  tar xz --wildcards '*/assets/pal-icons/*' --strip-components=3 -C web/public/pal-icons
```

The lookup tables are trimmed to the fields the UI renders (descriptions and
unused columns are dropped) to keep them out of the JS bundle's way; see the
commit that added them for the exact transform.
