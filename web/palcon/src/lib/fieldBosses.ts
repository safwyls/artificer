import fieldBosses from "../data/fieldBosses.json";

/**
 * Field boss spawn points: which pal stands where, and what a save's flag keys
 * mean.
 *
 * Its own module rather than part of `achievements.ts` for one reason worth
 * keeping: the live map draws these, and the map is an eagerly-loaded route.
 * Reaching into achievements.ts for them pulled `palDex.json` and the boss
 * catalogs into the main bundle and put 230 KB on every first paint. Names are
 * baked into the data at generation time and the icon path is derived, so
 * nothing here needs the pal catalog at all.
 *
 * See docs/vendored-game-data.md for where the table comes from and how to
 * regenerate it.
 */
interface FieldBossSpawn {
  /** Pal id — also the icon filename and the palDex key. */
  p: string;
  /** Display name and elements, baked in so this module needs no catalog. */
  n: string;
  e: string[];
}

interface FieldBossPoint extends FieldBossSpawn {
  l: number;
  x: number;
  y: number;
}

interface BountyPoint {
  /** Spawn key — also this target's NormalBossDefeatFlag key. */
  k: string;
  n: string;
  l: number;
  x: number;
  y: number;
}

/**
 * Two lists, not one, because they don't line up one-to-one. `spawns` maps a
 * save's flag key to its occupant — 89 keys, 89 pals. `points` has 90, because
 * one flag key (`remainsIsland_1_GrassGolem_FBOSS`) covers two Dualith spawns
 * at different places and levels; keying pins by flag would lose one.
 *
 * Every spawn here is one the location data can place. Nine keys the name
 * source also lists were dropped: no save read has ever set one, and the
 * location data has none of them either, which together say they are spawn
 * points a game update removed or renamed rather than content anyone can
 * reach. Listing them made the roster claim nine field bosses that cannot be
 * found. If one ever does turn up in a save it counts through
 * `unknownFieldBossCount` and says so, which is the signal to put it back.
 */
const data = fieldBosses as {
  spawns: Record<string, FieldBossSpawn>;
  points: FieldBossPoint[];
  bounties: BountyPoint[];
};
const spawns = data.spawns;

/** The pal's portrait. The id is already the icon's filename for every entry
 * in this table, so this doesn't need palIconUrl's catalog-backed key lookup —
 * which is the whole reason the map can stay cheap. */
export function fieldBossIconUrl(palId: string): string {
  return `${import.meta.env.BASE_URL}pal-icons/${palId}.webp`;
}

/** Every field boss on the map, once each, alphabetical. Each pal has exactly
 * one spawn point, so the table's values are the roster — which is what makes
 * "what's left" answerable rather than just "how many". */
export const FIELD_BOSS_ROSTER: { palId: string; name: string }[] = [
  ...new Map(Object.values(spawns).map((b) => [b.p, b.n])),
]
  .map(([palId, name]) => ({ palId, name }))
  .sort((a, b) => a.name.localeCompare(b.name));

/**
 * A boss the map can place. Field bosses carry a portrait and elements; bounty
 * targets are people, so they have neither — which is why both are optional
 * rather than there being two pin types the map has to branch on twice.
 */
export interface MapBossPin {
  name: string;
  level: number;
  x: number;
  y: number;
  /** Pal id for the portrait — field bosses only. */
  palId?: string;
  /** Field bosses only; a human has no element. */
  elements?: string[];
}

export const FIELD_BOSS_POINTS: MapBossPin[] = data.points.map((b) => ({
  palId: b.p,
  name: b.n,
  elements: b.e,
  level: b.l,
  x: b.x,
  y: b.y,
}));

/**
 * Where the human bounty targets stand. 66 pins for 33 targets: most spawn in
 * more than one camp, and the save records one flag for the target rather than
 * one per camp, so beating it anywhere clears them all.
 *
 * Elder is the one target with no pin. Its id
 * (`boss_hunter_fat_gatlinggun_quest_strongoldman`) marks it as quest-spawned,
 * so there is no fixed world position to record — not a gap in the data.
 *
 * The three `REGION_Oilrig_*` entries the source files alongside these are
 * left out: they have no catalog name and no bounty flag, and the save counts
 * them separately in OilrigClearCount.
 */
export const BOUNTY_POINTS: MapBossPin[] = data.bounties.map((b) => ({
  name: b.n,
  level: b.l,
  x: b.x,
  y: b.y,
}));

/** Which of the roster a player has beaten, keyed by pal. */
export function beatenFieldBossPals(keys: string[]): Set<string> {
  return new Set(keys.map((k) => spawns[k]?.p).filter(Boolean));
}

/** Spawn points in a save the table doesn't know — a new one from a game
 * update. Counted so the figures still add up, and surfaced so it's visible
 * rather than quietly missing from the roster. */
export function unknownFieldBossCount(keys: string[]): number {
  return keys.filter((k) => !(k in spawns)).length;
}
