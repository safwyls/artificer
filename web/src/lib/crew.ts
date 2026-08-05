import { palKey } from "./paldex";
import { partnerSkill, workProfile } from "./partner";
import type { Pal } from "./api";

/**
 * Base crew planner: the game's work-suitability sheet, transposed to a
 * base. From the pals actually assigned there (the save's baseId join) it
 * builds the 12-row work board, and from the guild's boxes it suggests who
 * would do a job better.
 *
 * Levels are species bases from the vendored catalog (palWork.json). The
 * save also records per-pal boosts — work books, and whatever a patch adds
 * next — that the extractor doesn't read yet, so a hand-fed pal can run one
 * level above what this board says. The UI owns that footnote.
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

export function workLevel(characterId: string, type: string): number {
  return workProfile(characterId)?.w[type] ?? 0;
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
 * crew is its own decision, not a suggestion. A candidate is suggested when
 * it beats the base's best level for a type the base covers, or covers a
 * type nobody does; ties prefer the smaller appetite, then the higher
 * combat level (a stronger pal defends the base it works).
 */
export function crewReport<P extends CrewPal>(crew: P[], boxes: P[]): CrewReport<P> {
  const rows: WorkRow<P>[] = WORK_TYPES.map(({ id, label }) => {
    const hands = crew
      .filter((p) => workLevel(p.characterId, id) > 0)
      .sort((a, b) => workLevel(b.characterId, id) - workLevel(a.characterId, id));
    const best = hands.length ? workLevel(hands[0].characterId, id) : 0;

    let upgrade: CrewUpgrade<P> | undefined;
    for (const p of boxes) {
      const lvl = workLevel(p.characterId, id);
      if (lvl <= best) continue;
      if (
        !upgrade ||
        lvl > upgrade.level ||
        (lvl === upgrade.level &&
          (appetite(p.characterId) < appetite(upgrade.pal.characterId) ||
            (appetite(p.characterId) === appetite(upgrade.pal.characterId) && p.pal.level > upgrade.pal.pal.level)))
      ) {
        upgrade = { pal: p, level: lvl, over: best };
      }
    }

    return {
      type: id,
      label,
      best,
      hands,
      night: hands.some((p) => isNocturnal(p.characterId)),
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

/** A pal's suitabilities, best first — the crew list's per-pal line. */
export function topWork(characterId: string, max = 3): { type: string; label: string; level: number }[] {
  return WORK_TYPES.map(({ id, label }) => ({ type: id, label, level: workLevel(characterId, id) }))
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
