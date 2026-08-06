import { palKey } from "./paldex";
import { partnerSkill, workProfile } from "./partner";
import type { Pal } from "./api";

/**
 * Base crew planner: the game's work-suitability sheet, transposed to a
 * base. From the pals actually assigned there (the save's baseId join) it
 * builds the 12-row work board, and from the guild's boxes it suggests who
 * would do a job better.
 *
 * A pal's effective level is what the game shows: the species base from the
 * vendored catalog (palWork.json), plus the work books recorded per-pal in
 * the save, plus the condenser's star bonus — which the save does NOT
 * store, so it's modeled from rank here exactly as the game derives it.
 * Jobs the player switched off for a pal are levels it has but work it
 * won't take, and the board refuses to count those as hands.
 */

/** The slice the planner reads; the calculators' SavePal satisfies it. */
export interface CrewPal {
  key: string;
  characterId: string;
  pal: Pal;
  playerUid: string;
  playerName: string;
  where: string;
}

/** The 12 work types, in the order the game's suitability sheet lists
 * them. Ids are the save/catalog codes; labels are the game's names.
 * OilExtraction exists in the catalog's tables but no pal has it — the
 * extractor is machine work — so it earns no row anywhere. */
export const WORK_TYPES: { id: string; label: string }[] = [
  { id: "EmitFlame", label: "Kindling" },
  { id: "Watering", label: "Watering" },
  { id: "Seeding", label: "Planting" },
  { id: "GenerateElectricity", label: "Electricity" },
  { id: "Handcraft", label: "Handiwork" },
  { id: "Collection", label: "Gathering" },
  { id: "Deforest", label: "Lumbering" },
  { id: "Mining", label: "Mining" },
  { id: "ProductMedicine", label: "Medicine" },
  { id: "Cool", label: "Cooling" },
  { id: "Transport", label: "Transporting" },
  { id: "MonsterFarm", label: "Farming" },
];

/** The species' own level for a type — the catalog table, nothing else.
 * Almost every caller wants {@link palWorkLevel} instead. */
export function workLevel(characterId: string, type: string): number {
  return workProfile(characterId)?.w[type] ?? 0;
}

/**
 * The condenser's work bonus per suitability, derived from rank the way
 * the game actually applies it. Stars 1–3 each add +1 to the *next*
 * suitability, best first and cycling round — a mono-suitability pal
 * stacks all three on its one job, which the wikis' "n-th best" phrasing
 * misses. Four stars add +1 to everything the species has, on top of the
 * 3-star allocation, not instead of it. "Best" ranks by species level,
 * ties in sheet order.
 *
 * Calibrated against real 0–4★ ladders on a live save (2026-08-06):
 * Jormuntide Ignis (Kindling 7 alone) reads 7/8/9/10/10, Eidrolon Ignis
 * (6/6) reads 6·6 / 7·6 / 7·7 / 8·7 / 9·8, and Pengullet (four 1s) reads
 * 1111 / 2111 / 2211 / 2221 / 3332 — all with zero work books to confound.
 * The save stores none of this; it's derived from Rank at runtime.
 */
function condenseAdds(characterId: string, stars: number): Map<string, number> {
  const adds = new Map<string, number>();
  if (stars <= 0) return adds;
  const owned = WORK_TYPES.map((w, i) => ({ id: w.id, level: workLevel(characterId, w.id), i }))
    .filter((w) => w.level > 0)
    .sort((a, b) => b.level - a.level || a.i - b.i);
  if (owned.length === 0) return adds;
  for (let k = 0; k < Math.min(stars, 3); k++) {
    const id = owned[k % owned.length].id;
    adds.set(id, (adds.get(id) ?? 0) + 1);
  }
  if (stars >= 4) for (const w of owned) adds.set(w.id, (adds.get(w.id) ?? 0) + 1);
  return adds;
}

export interface WorkBreakdown {
  /** The species' own level. */
  base: number;
  /** Ranks added by work books, recorded per-pal in the save. */
  books: number;
  /** Ranks added by the condenser's stars — up to +3 on a specialist's
   * one suitability (see condenseAdds). */
  condensed: number;
  /** What the game shows: base + books + star bonus, capped at 10. */
  level: number;
  /** The player switched this job off — the level stands, the work stops. */
  off: boolean;
}

export function workBreakdown(pal: Pal, type: string): WorkBreakdown {
  const base = workLevel(pal.characterId, type);
  const books = pal.workAdds?.[type] ?? 0;
  const stars = Math.max(0, Math.min(4, (pal.rank ?? 1) - 1));
  const condensed = condenseAdds(pal.characterId, stars).get(type) ?? 0;
  const level = base + books <= 0 ? 0 : Math.min(10, base + books + condensed);
  return { base, books, condensed, level, off: pal.workOff?.includes(type) ?? false };
}

/** The level the game actually shows for this pal: species base, its work
 * books and its condenser stars together. */
export function palWorkLevel(pal: Pal, type: string): number {
  return workBreakdown(pal, type).level;
}

export function isNocturnal(characterId: string): boolean {
  return workProfile(characterId)?.n === 1;
}

/** Food drain for one pal; 0 for species the catalog doesn't know. */
export function appetite(characterId: string): number {
  return workProfile(characterId)?.f ?? 0;
}

export interface CrewUpgrade<P extends CrewPal = CrewPal> {
  pal: P;
  level: number;
  /** The base's best level today — what the suggestion improves on. */
  over: number;
}

export interface WorkRow<P extends CrewPal = CrewPal> {
  type: string;
  label: string;
  /** Best operational level on the crew — a deployed work buff included;
   * 0 when nobody covers it. */
  best: number;
  /** Everyone covering it, best level first. */
  hands: P[];
  /** Someone on this row works through the night. */
  night: boolean;
  /** The deployed pal whose partner skill gives every *other* hand here a
   * flat +1 — already folded into best and the upgrade comparison. */
  buff?: { pal: P };
  upgrade?: CrewUpgrade<P>;
}

export interface CrewReport<P extends CrewPal = CrewPal> {
  rows: WorkRow<P>[];
  /** Total food drain of the crew. */
  appetite: number;
  /** Crew members that work through the night. */
  nightHands: number;
  /** Sick pals stop working — the loudest problem on a base. */
  sick: P[];
  /** Assigned to the base but suited to no work at all. */
  idle: P[];
  /** Members whose partner skill raises a suitability base-wide. */
  buffs: { pal: P; skill: string; type: string; description: string }[];
  /** Members that produce something at a Ranch. */
  ranchers: P[];
}

/**
 * The board for one base. `boxes` is the upgrade pool — the guild's pals
 * not working at any base; pass only those, since poaching another base's
 * crew is its own decision, not a suggestion. Levels are operational:
 * per-pal effective levels (books and condenser stars included), plus the
 * flat +1 a deployed work-buffing partner skill gives every *other* hand
 * of its type — non-stacking, and a sick buffer isn't working so doesn't
 * count. A pal whose job is switched off is neither a hand for it nor
 * suggested into it. A candidate is suggested when it beats the base's
 * best (buffed like-for-like — it would enjoy the same aura); ties prefer
 * the smaller appetite, then the higher combat level (a stronger pal
 * defends the base it works).
 */
export function crewReport<P extends CrewPal>(crew: P[], boxes: P[]): CrewReport<P> {
  // One buffer per type at most matters: the +1 doesn't stack.
  const buffers = new Map<string, P>();
  for (const p of crew) {
    const type = partnerSkill(p.characterId)?.base;
    if (type && p.pal.sick === "" && !buffers.has(type)) buffers.set(type, p);
  }
  // A hand's operational level: its own sheet, plus the aura when someone
  // *else* provides it and the pal has the suitability at all.
  const operational = (p: P, id: string, work: WorkBreakdown): number => {
    const buffer = buffers.get(id);
    if (work.level <= 0 || !buffer || buffer.key === p.key) return work.level;
    return Math.min(10, work.level + 1);
  };

  const rows: WorkRow<P>[] = WORK_TYPES.map(({ id, label }) => {
    const hands = crew
      .map((p) => ({ p, work: workBreakdown(p.pal, id) }))
      .filter(({ work }) => work.level > 0 && !work.off)
      .map((x) => ({ ...x, level: operational(x.p, id, x.work) }))
      .sort((a, b) => b.level - a.level);
    const best = hands.length ? hands[0].level : 0;

    let upgrade: CrewUpgrade<P> | undefined;
    for (const p of boxes) {
      const work = workBreakdown(p.pal, id);
      if (work.off) continue;
      const level = operational(p, id, work);
      if (level <= best) continue;
      if (
        !upgrade ||
        level > upgrade.level ||
        (level === upgrade.level &&
          (appetite(p.characterId) < appetite(upgrade.pal.characterId) ||
            (appetite(p.characterId) === appetite(upgrade.pal.characterId) && p.pal.level > upgrade.pal.pal.level)))
      ) {
        upgrade = { pal: p, level, over: best };
      }
    }

    const buffer = buffers.get(id);
    return {
      type: id,
      label,
      best,
      hands: hands.map(({ p }) => p),
      night: hands.some(({ p }) => isNocturnal(p.characterId)),
      buff: buffer && hands.some(({ p }) => p.key !== buffer.key) ? { pal: buffer } : undefined,
      upgrade,
    };
  });

  return {
    rows,
    appetite: crew.reduce((s, p) => s + appetite(p.characterId), 0),
    nightHands: crew.filter((p) => isNocturnal(p.characterId)).length,
    sick: crew.filter((p) => p.pal.sick !== ""),
    idle: crew.filter((p) => {
      const w = workProfile(p.characterId)?.w;
      return !w || Object.keys(w).length === 0;
    }),
    buffs: crew.flatMap((p) => {
      const s = partnerSkill(p.characterId);
      return s?.base ? [{ pal: p, skill: s.n, type: s.base, description: s.d }] : [];
    }),
    ranchers: crew.filter((p) => partnerSkill(p.characterId)?.ranch === 1),
  };
}

/** A pal's suitabilities at their effective levels, best first — the crew
 * list's per-pal line. Switched-off jobs stay listed, flagged, so the line
 * explains why the board doesn't count them. */
export function topWork(pal: Pal, max = 3): { type: string; label: string; level: number; off: boolean }[] {
  return WORK_TYPES.map(({ id, label }) => {
    const { level, off } = workBreakdown(pal, id);
    return { type: id, label, level, off };
  })
    .filter((w) => w.level > 0)
    .sort((a, b) => b.level - a.level)
    .slice(0, max);
}

/** One entry per distinct species in a crew — the board's hands stay
 * readable when a base runs six of the same pal. */
export function dedupeSpecies<P extends CrewPal>(pals: P[]): { pal: P; count: number }[] {
  const out = new Map<string, { pal: P; count: number }>();
  for (const p of pals) {
    const k = palKey(p.characterId);
    const seen = out.get(k);
    if (seen) seen.count += 1;
    else out.set(k, { pal: p, count: 1 });
  }
  return [...out.values()];
}
