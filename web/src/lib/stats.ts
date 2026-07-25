import palCombat from "../data/palCombat.json";
import { palKey } from "./paldex";

/**
 * Estimated pal stats from species, level, talents (IVs) and upgrades.
 *
 * Palworld's exact stat maths aren't officially published; this uses the
 * community-accepted formula (base + per-level scaling, talents worth up to
 * +30%, each Soul rank +3%, each condenser star +5%). It lands within a point
 * or two of the in-game numbers, so the UI labels the results "estimated".
 * Base values (hp/attack/defense) are vendored in palCombat.json; "attack"
 * there is the pal's shot attack, which is what the game surfaces as Attack.
 */
// [hp, attack, defense] per palKey.
const combat = palCombat as Record<string, number[]>;

/** Base [hp, attack, defense] for a species, or null when the catalog has no
 * combat stats for it (a handful of event/variant forms). */
export function baseStats(characterId: string): { hp: number; attack: number; defense: number } | null {
  const b = combat[palKey(characterId)];
  return b ? { hp: b[0], attack: b[1], defense: b[2] } : null;
}

export function hasCombatStats(characterId: string): boolean {
  return palKey(characterId) in combat;
}

export interface StatInput {
  characterId: string;
  level: number;
  /** Talents (IVs), 0–100 each. */
  ivHp: number;
  ivAttack: number;
  ivDefense: number;
  /** Soul upgrade rank per stat, 0–10 (+3% each). */
  soulHp?: number;
  soulAttack?: number;
  soulDefense?: number;
  /** Condenser stars, 0–4 (+5% each), applied to all three. */
  condenser?: number;
}

export interface StatResult {
  hp: number;
  attack: number;
  defense: number;
}

const ivMult = (iv: number) => 1 + 0.3 * (clamp(iv, 0, 100) / 100);
const soulMult = (soul: number) => 1 + 0.03 * clamp(soul, 0, 10);
const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));

export function computeStats(input: StatInput): StatResult | null {
  const base = baseStats(input.characterId);
  if (!base) return null;
  const L = Math.max(1, input.level);
  const cond = 1 + 0.05 * clamp(input.condenser ?? 0, 0, 4);

  const hp = Math.floor(
    (500 + 5 * L + base.hp * 0.5 * L * ivMult(input.ivHp)) * soulMult(input.soulHp ?? 0) * cond,
  );
  const attack = Math.floor(
    (100 + base.attack * 0.075 * L * ivMult(input.ivAttack)) * soulMult(input.soulAttack ?? 0) * cond,
  );
  const defense = Math.floor(
    (50 + base.defense * 0.075 * L * ivMult(input.ivDefense)) * soulMult(input.soulDefense ?? 0) * cond,
  );
  return { hp, attack, defense };
}

export interface TalentRating {
  /** Single-letter tier, S being a near-perfect roll. */
  tier: "S" | "A" | "B" | "C" | "D";
  /** Average of the three talents, 0–100. */
  average: number;
}

/** A quick read on a pal's talent roll, averaged across the three stats. */
export function talentRating(ivHp: number, ivAttack: number, ivDefense: number): TalentRating {
  const average = Math.round((clamp(ivHp, 0, 100) + clamp(ivAttack, 0, 100) + clamp(ivDefense, 0, 100)) / 3);
  const tier = average >= 90 ? "S" : average >= 70 ? "A" : average >= 50 ? "B" : average >= 30 ? "C" : "D";
  return { tier, average };
}
