/**
 * Passive-skill inheritance odds.
 *
 * Community-derived model (cross-checked against the published analyses the
 * public breeding calculators implement — these are reverse-engineered
 * rates, not official Pocketpair numbers):
 *
 *  1. The parents' passives form one deduplicated pool.
 *  2. The child inherits k of them, k = 1..4 weighted 40/30/20/10, the
 *     subset chosen uniformly; a roll above the pool size inherits the
 *     whole pool.
 *  3. A second 40/30/20/10 roll adds 0–3 random passives from outside the
 *     pool, filling up to the 4-slot cap — so a child that inherited a
 *     full 4 can't gain strays, which is why a perfect 4-passive copy is
 *     10% while a perfect 2-of-2 is 60% × 40% = 24%.
 *
 * Sanity anchors from that model, used by the checks in the UI copy:
 * exact-set odds with the pool holding only the desired passives are
 * 40% / 24% / 12% / 10% for 1–4 desired.
 */

const INHERIT_WEIGHTS = [0.4, 0.3, 0.2, 0.1]; // k = 1..4
const NO_RANDOM_ADDITION = 0.4; // the r = 0 outcome of the second roll

function choose(n: number, k: number): number {
  if (k < 0 || k > n) return 0;
  let out = 1;
  for (let i = 0; i < k; i++) out = (out * (n - i)) / (i + 1);
  return out;
}

export interface PassiveOdds {
  /** Child ends with precisely the desired passives, nothing else. */
  exact: number;
  /** Child has every desired passive, extras allowed. */
  atLeast: number;
}

/**
 * Odds for a desired subset of size `desired` out of a parent pool of
 * `poolSize` distinct passives. Null when the ask is impossible (more than
 * four, or passives the parents don't have).
 */
export function passiveOdds(poolSize: number, desired: number): PassiveOdds | null {
  if (desired > 4 || desired > poolSize || desired < 0) return null;
  if (poolSize === 0) {
    // Nothing to inherit: "no passives" only survives the random roll.
    return { exact: NO_RANDOM_ADDITION, atLeast: 1 };
  }
  if (desired === 0) {
    // A non-empty pool always contributes at least one inherited passive.
    return { exact: 0, atLeast: 1 };
  }

  let exact = 0;
  let atLeast = 0;
  for (let k = 1; k <= 4; k++) {
    const inherited = Math.min(k, poolSize);
    const w = INHERIT_WEIGHTS[k - 1];
    if (inherited === desired) {
      // Exactly the desired set, then no random stray on top — unless the
      // child is already at the 4-passive cap, where nothing can be added.
      const noStray = desired === 4 ? 1 : NO_RANDOM_ADDITION;
      exact += (w / choose(poolSize, desired)) * noStray;
    }
    if (inherited >= desired) {
      // Hypergeometric: the inherited subset covers all desired ones.
      atLeast += (w * choose(poolSize - desired, inherited - desired)) / choose(poolSize, inherited);
    }
  }
  return { exact, atLeast };
}

/** Chance a specific single passive from the pool reaches the child. */
export function singlePassiveOdds(poolSize: number): number {
  if (poolSize === 0) return 0;
  let p = 0;
  for (let k = 1; k <= 4; k++) {
    p += (INHERIT_WEIGHTS[k - 1] * Math.min(k, poolSize)) / poolSize;
  }
  return p;
}

/** Eggs needed for the given confidence of at least one success. */
export function eggsForConfidence(p: number, confidence = 0.9): number {
  if (p <= 0) return Infinity;
  if (p >= 1) return 1;
  return Math.ceil(Math.log(1 - confidence) / Math.log(1 - p));
}

export function expectedEggs(p: number): number {
  return p > 0 ? 1 / p : Infinity;
}
