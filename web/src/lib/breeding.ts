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
 * self-pairs (A×A) work.
 */
interface BreedingData {
  pals: string[];
  fwd: number[];
  unique: number[][];
}

const data = breedingData as BreedingData;
const ids = data.pals;
const n = ids.length;
const indexOf = new Map(ids.map((id, i) => [id, i]));

// Upper-triangular pair index for i <= j, matching the generator.
function pairIndex(i: number, j: number): number {
  if (i > j) [i, j] = [j, i];
  return (i * (2 * n - i + 1)) / 2 + (j - i);
}

const specialPairs = new Set(data.unique.map(([a, b]) => pairIndex(a, b)));

export interface BreedablePal {
  id: string;
  name: string;
}

/** Every breedable species, sorted by display name — the picker's catalog. */
export const BREEDABLE: BreedablePal[] = ids
  .map((id) => ({ id, name: palName(id) }))
  .sort((a, b) => a.name.localeCompare(b.name));

export function isBreedable(id: string): boolean {
  return indexOf.has(id);
}

export interface BreedResult {
  childId: string;
  /** True when the pair is one of the game's hand-authored combos rather than
   * the generic power-average formula — worth calling out in the UI. */
  special: boolean;
}

/** The child of breeding two species, or null if either isn't breedable. */
export function breedChild(aId: string, bId: string): BreedResult | null {
  const i = indexOf.get(aId);
  const j = indexOf.get(bId);
  if (i === undefined || j === undefined) return null;
  const pi = pairIndex(i, j);
  const childIdx = data.fwd[pi];
  if (childIdx === undefined || childIdx < 0) return null;
  return { childId: ids[childIdx], special: specialPairs.has(pi) };
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
  const target = indexOf.get(childId);
  if (target === undefined) return [];
  const out: ParentPair[] = [];
  for (let i = 0; i < n; i++) {
    for (let j = i; j < n; j++) {
      const pi = pairIndex(i, j);
      if (data.fwd[pi] === target) {
        out.push({ aId: ids[i], bId: ids[j], special: specialPairs.has(pi) });
      }
    }
  }
  out.sort((a, b) => Number(b.special) - Number(a.special));
  return out;
}
