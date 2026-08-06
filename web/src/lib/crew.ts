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
 * The suitabilities a condensed pal's star bonus reaches, derived from rank
 * the way the game does: one star boosts the best suitability by +1, two
 * stars the second-best as well, three the third; four stars boost every
 * suitability the species has. "Best" ranks by species level, ties in
 * sheet order. Community-verified (palworld.wiki.gg, Pal Condensation) —
 * the save stores no trace of it, so it has to be modeled.
 */
function condensedTypes(characterId: string, stars: number): string[] {
  if (stars <= 0) return [];
  const owned = WORK_TYPES.map((w, i) => ({ id: w.id, level: workLevel(characterId, w.id), i })).filter(
    (w) => w.level > 0,
  );
  if (stars >= 4) return owned.map((w) => w.id);
  return owned
    .sort((a, b) => b.level - a.level || a.i - b.i)
    .slice(0, stars)
    .map((w) => w.id);
}

export interface WorkBreakdown {
  /** The species' own level. */
  base: number;
  /** Ranks added by work books, recorded per-pal in the save. */
  books: number;
  /** +1 when the condenser's star bonus reaches this type. */
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
  const condensed = base > 0 && condensedTypes(pal.characterId, stars).includes(type) ? 1 : 0;
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
  /** Best suitability level on the crew; 0 when nobody covers it. */
  best: number;
  /** Everyone covering it, best level first. */
  hands: P[];
  /** Someone on this row works through the night. */
  night: boolean;
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
  buffs: { pal: P; skill: string; description: string }[];
  /** Members that produce something at a Ranch. */
  ranchers: P[];
}

/**
 * The board for one base. `boxes` is the upgrade pool — the guild's pals
 * not working at any base; pass only those, since poaching another base's
 * crew is its own decision, not a suggestion. Levels are per-pal effective
 * levels (books and condenser stars included), and a pal whose job is
 * switched off is neither a hand for it nor suggested into it. A candidate
 * is suggested when it beats the base's best level for a type the base
 * covers, or covers a type nobody does; ties prefer the smaller appetite,
 * then the higher combat level (a stronger pal defends the base it works).
 */
export function crewReport<P extends CrewPal>(crew: P[], boxes: P[]): CrewReport<P> {
  const rows: WorkRow<P>[] = WORK_TYPES.map(({ id, label }) => {
    const hands = crew
      .map((p) => ({ p, work: workBreakdown(p.pal, id) }))
      .filter(({ work }) => work.level > 0 && !work.off)
      .sort((a, b) => b.work.level - a.work.level);
    const best = hands.length ? hands[0].work.level : 0;

    let upgrade: CrewUpgrade<P> | undefined;
    for (const p of boxes) {
      const work = workBreakdown(p.pal, id);
      if (work.off || work.level <= best) continue;
      if (
        !upgrade ||
        work.level > upgrade.level ||
        (work.level === upgrade.level &&
          (appetite(p.characterId) < appetite(upgrade.pal.characterId) ||
            (appetite(p.characterId) === appetite(upgrade.pal.characterId) && p.pal.level > upgrade.pal.pal.level)))
      ) {
        upgrade = { pal: p, level: work.level, over: best };
      }
    }

    return {
      type: id,
      label,
      best,
      hands: hands.map(({ p }) => p),
      night: hands.some(({ p }) => isNocturnal(p.characterId)),
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
      return s?.base ? [{ pal: p, skill: s.n, description: s.d }] : [];
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
