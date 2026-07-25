import breedingData from "../data/breeding.json";
import { palName } from "./paldex";

/**
 * Palworld breeding lookups, driven by a precomputed forward table.
 *
 * Rather than reimplement the game's combi-rank formula (whose tie-breaking is
 * fiddly and easy to get subtly wrong), the child of every parent pair is
 * baked into a dense table vendored from PalworldSaveTools — see
 * web/public/pal-icons/README.md for provenance. `fwd` is indexed by an
 * upper-triangular pair index so breeding is symmetric (A×B == B×A) and
 * self-pairs (A×A) work. Hand-authored combos in `unique` take precedence
 * over `fwd`, which for 61 of them still holds the formula child.
 */
interface BreedingData {
  pals: string[];
  fwd: number[];
  /** Hand-authored combos as [parentA, parentB, child] index triples. Most are
   * also baked into fwd, but 61 of them aren't — their fwd slot still holds
   * the power-average child — so they must override fwd on lookup. One pair
   * (Katress × Wixen) appears twice with different children: the game's only
   * gender-dependent combo, kept as the pair's alternate child. */
  unique: number[][];
}

const data = breedingData as BreedingData;
const ids = data.pals;
const n = ids.length;
const indexOf = new Map(ids.map((id, i) => [id, i]));
// Save files hand out ids the table doesn't spell the same way. The table
// itself mixes cases ("Blueplatypus" vs "BluePlatypus_Fire"), and captured
// pals carry decorations: alphas ("BOSS_Penking"), predator pals
// ("PREDATOR_MonochromeQueen"), raid-summon captures ("SUMMON_DarkAlien_MAX"),
// and variant spawns ("PlantSlime_Flower") — all of which breed as their base
// species. Strip decorations until the id matches a table row; ids that never
// match (humans, gym/raid/tower bosses) are genuinely unbreedable.
const canonIndexOf = new Map(ids.map((id, i) => [id.toLowerCase(), i]));
const DECOR_PREFIX = /^(boss|predator|raid|summon|gym)_/;
const DECOR_SUFFIX = /_(flower|oilrig|largeoilrig|minioilrig|tower|invader|max|avatar|otomo|servant|quest|enemy|friend|\d+)$/;

/** Table row for a characterId in any of the save's spellings, or undefined
 * when the species isn't breedable at all. */
export function speciesIndexOf(characterId: string): number | undefined {
  const exact = indexOf.get(characterId);
  if (exact !== undefined) return exact;
  let key = characterId.toLowerCase();
  for (;;) {
    const hit = canonIndexOf.get(key);
    if (hit !== undefined) return hit;
    let next = key.replace(DECOR_PREFIX, "");
    if (next === key) next = key.replace(DECOR_SUFFIX, "");
    if (next === key) return undefined;
    key = next;
  }
}

// Upper-triangular pair index for i <= j, matching the generator.
function pairIndex(i: number, j: number): number {
  if (i > j) [i, j] = [j, i];
  return (i * (2 * n - i + 1)) / 2 + (j - i);
}

const specialPairs = new Set(data.unique.map(([a, b]) => pairIndex(a, b)));
const overrideApplied = new Set<number>();

// Resolve overrides once into flat tables: the planner hits these lookups tens
// of thousands of times per plan, so they're plain array reads. On a unique
// pair the authored children replace the fwd slot outright (its formula child
// is unobtainable in game); a second distinct authored child becomes the alt.
const resolved = new Int16Array(data.fwd.length);
const resolvedAlt = new Int16Array(data.fwd.length).fill(-1);
for (let k = 0; k < data.fwd.length; k++) resolved[k] = data.fwd[k] ?? -1;
for (const [a, b, c] of data.unique) {
  const pi = pairIndex(a, b);
  if (!overrideApplied.has(pi)) {
    overrideApplied.add(pi);
    resolved[pi] = c;
  } else if (resolved[pi] !== c) {
    resolvedAlt[pi] = c;
  }
}

// Low-level table access for the path planner (breeding-path.ts), which works
// in table indices to avoid re-canonicalizing ids in its inner loops.
export const SPECIES_IDS: readonly string[] = ids;

/** Child species index for a pair of table rows, or -1 when the pair can't breed. */
export function childOfPair(i: number, j: number): number {
  return resolved[pairIndex(i, j)];
}

/** Second possible child for the game's one gender-dependent pair, else -1. */
export function altChildOfPair(i: number, j: number): number {
  return resolvedAlt[pairIndex(i, j)];
}

export function isSpecialPair(i: number, j: number): boolean {
  return specialPairs.has(pairIndex(i, j));
}

export interface BreedablePal {
  id: string;
  name: string;
}

/** Every breedable species, sorted by display name — the picker's catalog. */
export const BREEDABLE: BreedablePal[] = ids
  .map((id) => ({ id, name: palName(id) }))
  .sort((a, b) => a.name.localeCompare(b.name));

export function isBreedable(id: string): boolean {
  return speciesIndexOf(id) !== undefined;
}

export interface BreedResult {
  childId: string;
  /** True when the pair is one of the game's hand-authored combos rather than
   * the generic power-average formula — worth calling out in the UI. */
  special: boolean;
  /** The other possible child for the game's one gender-dependent pair
   * (Katress × Wixen); which hatches depends on which parent is which gender. */
  altChildId?: string;
}

/** The child of breeding two species, or null if either isn't breedable. */
export function breedChild(aId: string, bId: string): BreedResult | null {
  const i = speciesIndexOf(aId);
  const j = speciesIndexOf(bId);
  if (i === undefined || j === undefined) return null;
  const childIdx = childOfPair(i, j);
  if (childIdx < 0) return null;
  const alt = altChildOfPair(i, j);
  return {
    childId: ids[childIdx],
    special: isSpecialPair(i, j),
    ...(alt >= 0 ? { altChildId: ids[alt] } : {}),
  };
}

export interface ParentPair {
  aId: string;
  bId: string;
  special: boolean;
}

/**
 * All parent pairs that breed into a target species. Scans the forward table
 * (~46k pairs) once — cheap, and avoids shipping a second reverse table.
 * Results are sorted so special combos surface first.
 */
export function parentPairsFor(childId: string): ParentPair[] {
  const target = speciesIndexOf(childId);
  if (target === undefined) return [];
  const out: ParentPair[] = [];
  for (let i = 0; i < n; i++) {
    for (let j = i; j < n; j++) {
      if (childOfPair(i, j) === target || altChildOfPair(i, j) === target) {
        out.push({ aId: ids[i], bId: ids[j], special: specialPairs.has(pairIndex(i, j)) });
      }
    }
  }
  out.sort((a, b) => Number(b.special) - Number(a.special));
  return out;
}
