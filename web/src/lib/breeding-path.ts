import { SPECIES_IDS, altChildOfPair, childOfPair, isSpecialPair, speciesIndexOf } from "./breeding";

/**
 * Breeding route planner: from the pals actually in the save, find how to
 * breed a target species — plus longer routes, offered only when the extra
 * work raises the talent ceiling.
 *
 * Model: a route is a binary tree whose leaves are owned pals and whose inner
 * nodes are breeding steps. Parents aren't consumed by breeding, so anything
 * hatched joins the pool and can parent any number of later pairs; the cost
 * that's minimized is therefore *generations* (an intermediate's depth), and
 * the egg count quoted to the user is the number of distinct steps after
 * merging identical subtrees. The "ceiling" of a node is its best-case
 * talents — each talent inherits from either parent (or rerolls), so with
 * perfect luck a child gets the per-stat max of its parents, and a route's
 * ceiling is the per-stat max over the owned pals at its leaves.
 *
 * Per species and generation the solver keeps a handful of champion
 * derivations — the best talent sum plus the best of each single stat, ties
 * going to smaller trees. That's a deliberate heuristic rather than a full
 * Pareto frontier: every route it returns is executable and its ceiling
 * honest, but a marginally better route can in principle be missed. Rounds
 * grow the champions generation by generation — earlier generations are
 * final before the next is formed — by pairing species through the forward
 * table (~46k pairs), bounded by the target's minimum generation (from a
 * fast generations-only relaxation) plus EXTRA_GENERATIONS, and skipping
 * species that can't sit inside a within-bound derivation of the target.
 *
 * Gender is deliberately not modeled (except that a pal can't be paired with
 * itself); the UI carries that caveat instead.
 */

/** What the solver needs to know about an owned pal. `SavePal` satisfies it. */
export interface PathPal {
  /** Stable instance id — tells two pals of the same species apart. */
  key: string;
  characterId: string;
  ivHp: number;
  ivAttack: number;
  ivDefense: number;
}

/** Best-case talents as [HP, Attack, Defense]. */
export type Ceiling = readonly [number, number, number];

export type StepParent<P extends PathPal> =
  | { kind: "owned"; pal: P }
  | { kind: "egg"; n: number; speciesId: string };

export interface BreedStep<P extends PathPal> {
  /** 1-based position in breed order; later steps may reuse this egg's pal. */
  n: number;
  childId: string;
  special: boolean;
  ceiling: Ceiling;
  a: StepParent<P>;
  b: StepParent<P>;
}

export interface RouteOption<P extends PathPal> {
  /** Eggs to hatch, after merging reused intermediates. */
  eggs: number;
  ceiling: Ceiling;
  /** In breed order; the last step's child is the target. Empty when eggs=0. */
  steps: BreedStep<P>[];
  /** The already-owned target instance, for the 0-egg option. */
  ownedTarget?: P;
}

export type PlanResult<P extends PathPal> =
  | { status: "ok"; options: RouteOption<P>[] }
  | { status: "notBreedable" }
  | { status: "unreachable" };

/** Generations past the minimum to explore for a better ceiling. */
const EXTRA_GENERATIONS = 2;
/** At most this many route chips: fastest + up to two better-ceiling upgrades. */
const MAX_OPTIONS = 3;

interface Entry<P extends PathPal> {
  /** Generation: 0 for an owned pal, else max(parents) + 1. */
  gen: number;
  iv: Ceiling;
  /** Tree size in breeding steps before subtree merging — the egg-count bias. */
  size: number;
  leaf?: P;
  pair?: { ai: number; a: Entry<P>; bi: number; b: Entry<P>; ci: number };
}

const ivSum = (iv: Ceiling) => iv[0] + iv[1] + iv[2];

/** Champion criteria: talent sum, then each single stat. */
const N_CRITERIA = 4;
function score(e: Entry<PathPal>, k: number): number {
  return k === 0 ? ivSum(e.iv) : e.iv[k - 1];
}
function beats(a: Entry<PathPal>, b: Entry<PathPal>, k: number): boolean {
  const d = score(a, k) - score(b, k);
  return d !== 0 ? d > 0 : a.size < b.size;
}

export function planRoutes<P extends PathPal>(owned: P[], targetId: string): PlanResult<P> {
  const n = SPECIES_IDS.length;
  const target = speciesIndexOf(targetId);
  if (target === undefined) return { status: "notBreedable" };

  // byGen[species][generation] = champion derivations, one per criterion.
  const byGen: Entry<P>[][][] = Array.from({ length: n }, () => []);

  const insert = (species: number, cand: Entry<P>) => {
    const bucket = (byGen[species][cand.gen] ??= []);
    let champion = bucket.length < N_CRITERIA;
    if (!champion) {
      for (let k = 0; k < N_CRITERIA && !champion; k++) {
        let best: Entry<P> = bucket[0];
        for (let m = 1; m < bucket.length; m++) if (beats(bucket[m], best, k)) best = bucket[m];
        champion = beats(cand, best, k);
      }
      if (!champion) return;
    }
    bucket.push(cand);
    if (bucket.length > N_CRITERIA) {
      // Keep exactly the champions; anything not best at some criterion goes.
      const keep = new Set<Entry<P>>();
      for (let k = 0; k < N_CRITERIA; k++) {
        let best = bucket[0];
        for (let m = 1; m < bucket.length; m++) if (beats(bucket[m], best, k)) best = bucket[m];
        keep.add(best);
      }
      byGen[species][cand.gen] = [...keep];
    }
  };

  // Seed best-first so ties resolve toward higher talent sums.
  const sortedOwned = [...owned].sort(
    (a, b) => b.ivHp + b.ivAttack + b.ivDefense - (a.ivHp + a.ivAttack + a.ivDefense),
  );
  for (const pal of sortedOwned) {
    const i = speciesIndexOf(pal.characterId);
    if (i === undefined) continue;
    insert(i, { gen: 0, iv: [pal.ivHp, pal.ivAttack, pal.ivDefense], size: 0, leaf: pal });
  }

  // Earliest generation each species can exist at:
  // dist[c] = min over pairs of max(dist[a], dist[b]) + 1.
  const INF = 0x3fffffff;
  const dist = new Int32Array(n).fill(INF);
  for (let i = 0; i < n; i++) if (byGen[i][0]?.length) dist[i] = 0;
  for (let changed = true; changed; ) {
    changed = false;
    for (let i = 0; i < n; i++) {
      if (dist[i] >= INF) continue;
      for (let j = i; j < n; j++) {
        if (dist[j] >= INF) continue;
        const d = Math.max(dist[i], dist[j]) + 1;
        const c1 = childOfPair(i, j);
        if (c1 < 0) continue;
        if (d < dist[c1]) {
          dist[c1] = d;
          changed = true;
        }
        const c2 = altChildOfPair(i, j);
        if (c2 >= 0 && d < dist[c2]) {
          dist[c2] = d;
          changed = true;
        }
      }
    }
  }
  if (dist[target] >= INF) return { status: "unreachable" };

  const cap = dist[target] + EXTRA_GENERATIONS;

  // Fewest generations from a species to the target, treating any reachable
  // species as an eligible partner regardless of timing — a lower bound, so
  // pruning on dist[s] + up[s] > cap never cuts a real route.
  const up = new Int32Array(n).fill(INF);
  up[target] = 0;
  for (let changed = true; changed; ) {
    changed = false;
    for (let i = 0; i < n; i++) {
      if (dist[i] >= INF) continue;
      for (let j = i; j < n; j++) {
        if (dist[j] >= INF) continue;
        const c1 = childOfPair(i, j);
        if (c1 < 0) continue;
        const c2 = altChildOfPair(i, j);
        const through = Math.min(up[c1], c2 >= 0 ? up[c2] : INF) + 1;
        if (through < up[i]) {
          up[i] = through;
          changed = true;
        }
        if (through < up[j]) {
          up[j] = through;
          changed = true;
        }
      }
    }
  }

  const useful = (s: number) => dist[s] + up[s] <= cap;

  for (let r = 1; r <= cap; r++) {
    for (let i = 0; i < n; i++) {
      if (dist[i] >= r || !useful(i)) continue; // nothing below generation r, or off-route
      for (let j = i; j < n; j++) {
        if (dist[j] >= r || !useful(j)) continue;
        const c1 = childOfPair(i, j);
        if (c1 < 0) continue;
        const c2 = altChildOfPair(i, j);
        // A child born this round still has to reach the target in budget.
        const want1 = r + up[c1] <= cap;
        const want2 = c2 >= 0 && r + up[c2] <= cap;
        if (!want1 && !want2) continue;
        // A generation-r child needs one parent at exactly r-1 and the other
        // anywhere at or below it. Within one species the pair is unordered,
        // so only walk one orientation (and one triangle at ga === gb).
        const combine = (listA: Entry<P>[] | undefined, listB: Entry<P>[] | undefined) => {
          if (!listA?.length || !listB?.length) return;
          const triangle = listA === listB;
          for (let x = 0; x < listA.length; x++) {
            const ea = listA[x];
            for (let y = triangle ? x : 0; y < listB.length; y++) {
              const eb = listB[y];
              // A pal can't breed with itself; two pals of one species can.
              if (ea.leaf && eb.leaf && ea.leaf.key === eb.leaf.key) continue;
              const iv: Ceiling = [
                Math.max(ea.iv[0], eb.iv[0]),
                Math.max(ea.iv[1], eb.iv[1]),
                Math.max(ea.iv[2], eb.iv[2]),
              ];
              const size = ea.size + eb.size + 1;
              if (want1) {
                insert(c1, { gen: r, iv, size, pair: { ai: i, a: ea, bi: j, b: eb, ci: c1 } });
              }
              if (want2) {
                insert(c2, { gen: r, iv, size, pair: { ai: i, a: ea, bi: j, b: eb, ci: c2 } });
              }
            }
          }
        };
        for (let g = 0; g <= r - 1; g++) {
          combine(byGen[i][g], byGen[j][r - 1]);
        }
        if (i !== j) {
          for (let g = 0; g <= r - 2; g++) {
            combine(byGen[i][r - 1], byGen[j][g]);
          }
        }
      }
    }
  }

  // One candidate per generation (best ceiling, then smallest tree),
  // reconstructed — which merges reused subtrees, shrinking the egg count —
  // then filtered to: fastest first, then only strict ceiling upgrades.
  const candidates: Entry<P>[] = [];
  for (const bucket of byGen[target]) {
    if (!bucket?.length) continue;
    let best = bucket[0];
    for (const e of bucket) {
      if (ivSum(e.iv) > ivSum(best.iv) || (ivSum(e.iv) === ivSum(best.iv) && e.size < best.size)) best = e;
    }
    candidates.push(best);
  }
  const recon = candidates.map(reconstruct);
  recon.sort((a, b) => a.eggs - b.eggs || ivSum(b.ceiling) - ivSum(a.ceiling));

  const options: RouteOption<P>[] = [];
  for (const opt of recon) {
    const prev = options[options.length - 1];
    if (prev && ivSum(opt.ceiling) <= ivSum(prev.ceiling)) continue;
    options.push(opt);
    if (options.length >= MAX_OPTIONS) break;
  }
  return { status: "ok", options };
}

/** Unfold an entry into breed-order steps, merging identical subtrees — a pal
 * hatched once can parent any number of later pairs. */
function reconstruct<P extends PathPal>(entry: Entry<P>): RouteOption<P> {
  if (entry.leaf) return { eggs: 0, ceiling: entry.iv, steps: [], ownedTarget: entry.leaf };

  const steps: BreedStep<P>[] = [];
  const seen = new Map<string, number>();
  const build = (e: Entry<P>): { ref: StepParent<P>; sig: string } => {
    if (e.leaf) return { ref: { kind: "owned", pal: e.leaf }, sig: `L${e.leaf.key}` };
    const { ai, a, bi, b, ci } = e.pair!;
    const ra = build(a);
    let rb = build(b);
    if (ra.sig === rb.sig && ra.ref.kind === "egg") {
      // Both slots resolved to the same hatched pal, which can't breed with
      // itself — run its parents' pairing once more for a second copy.
      const orig = steps[ra.ref.n - 1];
      const copySig = `${ra.sig}#2`;
      let copyN = seen.get(copySig);
      if (copyN === undefined) {
        copyN = steps.length + 1;
        steps.push({ ...orig, n: copyN });
        seen.set(copySig, copyN);
      }
      rb = { ref: { kind: "egg", n: copyN, speciesId: orig.childId }, sig: copySig };
    }
    const speciesId = SPECIES_IDS[ci];
    const sig = `B${ci}(${ra.sig}|${rb.sig})`;
    let num = seen.get(sig);
    if (num === undefined) {
      num = steps.length + 1;
      steps.push({
        n: num,
        childId: speciesId,
        special: isSpecialPair(ai, bi),
        ceiling: e.iv,
        a: ra.ref,
        b: rb.ref,
      });
      seen.set(sig, num);
    }
    return { ref: { kind: "egg", n: num, speciesId }, sig };
  };
  build(entry);
  return { eggs: steps.length, ceiling: entry.iv, steps };
}
