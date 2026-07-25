import palCombat from "../data/palCombat.json";
import palPassives from "../data/palPassives.json";
import { palKey, palEntry } from "./paldex";

/**
 * Estimated pal stats from species, level, talents (IVs) and upgrades.
 *
 * Calibrated against in-game numbers: the base + level + talent core is exact
 * (talents worth up to +30%). On top of it:
 *  - Passives of the same stat stack ADDITIVELY (Legend +20% and Burly Body
 *    +20% give +40% defense, not ×1.44).
 *  - Condenser (+5%/star) and souls (+3%/rank) multiply.
 *  - An Alpha (captured field boss) carries an HP-only bonus of +rarity%, which
 *    is why a level-70 alpha Jetragon shows ~15% more HP than its listed base.
 *  - Trust/bond uses the game's per-pal friendship rate, but the exact curve
 *    isn't published, so it's the one approximate term.
 *
 * Base values are [hp, shotAttack, defense, then the three friendship rates].
 * "attack" is shot attack — the value the game surfaces as Attack.
 */
const combat = palCombat as Record<string, number[]>;
const passiveEffects = palPassives as Record<string, number[]>; // code -> [atk%, def%, hp%]

/** Base [hp, attack, defense] for a species, or null when the catalog has no
 * combat stats for it (a handful of event/variant forms). */
export function baseStats(characterId: string): { hp: number; attack: number; defense: number } | null {
  const b = combat[palKey(characterId)];
  return b ? { hp: b[0], attack: b[1], defense: b[2] } : null;
}

export function hasCombatStats(characterId: string): boolean {
  return palKey(characterId) in combat;
}

/** Per-level trust bonus rates [hp%, attack%, defense%] for the species. */
function trustRates(characterId: string): [number, number, number] {
  const b = combat[palKey(characterId)];
  return b && b.length >= 6 ? [b[3], b[4], b[5]] : [0, 0, 0];
}

/** Whether a passive code actually changes the displayed HP/Attack/Defense. */
export function passiveAffectsStats(code: string): boolean {
  return code in passiveEffects;
}

/** A passive's stat effect as [attack%, defense%, hp%], or null when it doesn't
 * touch the displayed stats (element-damage and player-buff passives don't). */
export function passiveStatEffect(code: string): [number, number, number] | null {
  const e = passiveEffects[code];
  return e ? [e[0], e[1], e[2]] : null;
}

export interface StatInput {
  characterId: string;
  level: number;
  /** Talents (IVs), 0–100 each. */
  ivHp: number;
  ivAttack: number;
  ivDefense: number;
  /** Soul upgrade rank per stat, 0–20 (+3% each). */
  soulHp?: number;
  soulAttack?: number;
  soulDefense?: number;
  /** Condenser stars, 0–4 (+5% each), applied to all three. */
  condenser?: number;
  /** Trust/bond level; scaled by the species' friendship rates. Approximate. */
  trust?: number;
  /** Passive skill codes; only stat-affecting ones move the numbers. */
  passives?: string[];
  /** Captured field boss — adds an HP-only bonus of +rarity%. */
  isAlpha?: boolean;
}

export interface StatResult {
  hp: number;
  attack: number;
  defense: number;
}

const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));
const ivMult = (iv: number) => 1 + 0.3 * (clamp(iv, 0, 100) / 100);
const soulMult = (soul: number) => 1 + 0.03 * clamp(soul, 0, 20);

/** Summed passive percentages per stat: [atk%, def%, hp%]. Same-stat passives
 * add rather than compound. */
function passiveSums(passives: string[] | undefined): [number, number, number] {
  const s: [number, number, number] = [0, 0, 0];
  for (const code of passives ?? []) {
    const e = passiveEffects[code];
    if (e) {
      s[0] += e[0];
      s[1] += e[1];
      s[2] += e[2];
    }
  }
  return s;
}

export function computeStats(input: StatInput): StatResult | null {
  const base = baseStats(input.characterId);
  if (!base) return null;
  const L = Math.max(1, input.level);
  const cond = 1 + 0.05 * clamp(input.condenser ?? 0, 0, 4);
  const trust = Math.max(0, input.trust ?? 0);
  const [fHp, fAtk, fDef] = trustRates(input.characterId);
  const [sAtk, sDef, sHp] = passiveSums(input.passives);
  // Alphas carry an HP-only bonus equal to the pal's rarity, in percent.
  const rarity = palEntry(input.characterId)?.rarity ?? 0;
  const baseHp = input.isAlpha ? base.hp * (1 + rarity / 100) : base.hp;

  const hp = Math.floor(
    (500 + 5 * L + baseHp * 0.5 * L * ivMult(input.ivHp)) *
      cond * soulMult(input.soulHp ?? 0) * (1 + (fHp / 100) * trust) * (1 + sHp / 100),
  );
  const attack = Math.floor(
    (100 + base.attack * 0.075 * L * ivMult(input.ivAttack)) *
      cond * soulMult(input.soulAttack ?? 0) * (1 + (fAtk / 100) * trust) * (1 + sAtk / 100),
  );
  const defense = Math.floor(
    (50 + base.defense * 0.075 * L * ivMult(input.ivDefense)) *
      cond * soulMult(input.soulDefense ?? 0) * (1 + (fDef / 100) * trust) * (1 + sDef / 100),
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
