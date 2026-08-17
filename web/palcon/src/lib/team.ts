import fights from "../data/bossFights.json";
import { ASTRALYM, PALPAGOS_TOWERS, PANTHALUS, RAID_ROSTER, WORLD_TREE_RUN, bossLabel, isLaboratory } from "./achievements";
import { elementCounters } from "./elements";
import { FIELD_BOSS_POINTS } from "./fieldBosses";
import { palEntry, palKey, palName } from "./paldex";
import { MOUNT_LABELS, partnerSkill, workProfile, type MountKind } from "./partner";
import { palEffectiveStats, powerScore } from "./stats";
import type { Pal } from "./api";

/**
 * Team builder: from the pals actually in the save, pick the five to bring.
 *
 * The scorer is a weighted sum of *named* factors — every point a candidate
 * earns or loses is also a reason the UI can show, so the ranking never says
 * "trust me". Weights are hand-set round numbers on one scale: raw strength
 * tops out around 40, an element edge is 18 a hit, and everything else sits
 * below those two on purpose — a matchup tool that ranked a mount over a
 * counter would be answering the wrong question.
 *
 * Element edges come from the chart in elements.ts, both ways: the target is
 * weak to a pal's element (hits hard), or attacks with an element the pal's
 * own type loses to (takes hits). Partner skills contribute through their
 * vendored tags (see lib/partner.ts) — magnitudes scale with skill rank and
 * are unknowable from a save, so a skill scores by what it does, not size.
 */

/** The slice of the save the engine reads. The calculators' SavePal shape
 * satisfies it structurally; the full record is only tapped for stats. */
export interface TeamPal {
  /** Instance id — identity within a team. */
  key: string;
  characterId: string;
  pal: Pal;
}

export interface TeamTarget {
  id: string;
  label: string;
  /** Where the fight happens, for the option line and the target card. */
  where?: string;
  group: "story" | "raid" | "field" | "element";
  /** For the portrait; element targets have none. */
  characterId?: string;
  elements: string[];
  level?: number;
  levelHard?: number;
}

export interface Reason {
  label: string;
  /** Tints the chip; a reason about no particular element has none. */
  element?: string;
  kind: "power" | "edge" | "partner" | "pair" | "mount" | "coverage";
  value: number;
}

/** Generic over the pal shape so callers get their own richer type back —
 * the calculators pass SavePal and read nicknames off the results. */
export interface ScoredPal<P extends TeamPal = TeamPal> {
  pal: P;
  score: number;
  reasons: Reason[];
  /** Negative matchup facts — shown in warning tone, already in the score. */
  warnings: Reason[];
}

// ---------------------------------------------------------------------------
// Targets
// ---------------------------------------------------------------------------

const ELEMENTS = ["Normal", "Fire", "Water", "Leaf", "Electricity", "Ice", "Earth", "Dark", "Dragon"];

function bossElements(palId: string): string[] {
  return palEntry(palId)?.elements ?? [];
}

/** Every fight worth planning for, in the order the game presents them:
 * story bosses in progression order, raids by level, field bosses by level,
 * and the nine plain elements for "just build me an anti-Fire team".
 * The Forbidden Laboratory is skipped, as everywhere: it's eight pals over
 * four waves, and one element line would be wrong about seven of them. */
export const TEAM_TARGETS: TeamTarget[] = (() => {
  // Raids carry no venue and call their second difficulty "ultra"; Panthalus
  // has no hard mode at all. Everything after normal is optional.
  interface Fight {
    title: string;
    where?: string;
    normal?: number[];
    hard?: number[];
    ultra?: number[];
  }
  const towers = fights.towers as Record<string, Fight>;
  const raids = fights.raids as Record<string, Fight>;
  const out: TeamTarget[] = [];

  for (const boss of [...PALPAGOS_TOWERS, PANTHALUS, ...WORLD_TREE_RUN, ASTRALYM]) {
    if (isLaboratory(boss) || !boss.palId) continue;
    const fight = towers[boss.key];
    out.push({
      id: boss.key,
      label: bossLabel(boss),
      where: fight?.where,
      group: "story",
      characterId: boss.palId,
      elements: bossElements(boss.palId),
      level: fight?.normal?.[0],
      levelHard: fight?.hard?.[0],
    });
  }

  for (const raid of RAID_ROSTER) {
    const fight = raids[raid.key];
    out.push({
      id: raid.key,
      label: palName(raid.palId),
      where: fight?.where,
      group: "raid",
      characterId: raid.palId,
      elements: bossElements(raid.palId),
      level: fight?.normal?.[0],
      levelHard: fight?.hard?.[0] ?? fight?.ultra?.[0],
    });
  }

  // One entry per species, at the highest level it spawns at.
  const fieldBest = new Map<string, TeamTarget>();
  for (const pin of FIELD_BOSS_POINTS) {
    if (!pin.palId) continue;
    const seen = fieldBest.get(pin.palId);
    if (seen && (seen.level ?? 0) >= pin.level) continue;
    fieldBest.set(pin.palId, {
      id: `field:${pin.palId}`,
      label: pin.name,
      group: "field",
      characterId: pin.palId,
      elements: pin.elements ?? [],
      level: pin.level,
    });
  }
  out.push(...[...fieldBest.values()].sort((a, b) => (a.level ?? 0) - (b.level ?? 0) || a.label.localeCompare(b.label)));

  for (const el of ELEMENTS) {
    out.push({ id: `element:${el}`, label: `${el} pals`, group: "element", elements: [el] });
  }
  return out;
})();

export function teamTargetById(id: string): TeamTarget | undefined {
  return TEAM_TARGETS.find((t) => t.id === id);
}

// ---------------------------------------------------------------------------
// Scoring
// ---------------------------------------------------------------------------

/** Weights, one scale. Kept together so rebalancing is one edit. Exported
 * for the "how ranking works" explainer, which prints these very numbers —
 * the write-up can't drift from the model when it *is* the model. */
export const TEAM_WEIGHTS = {
  power: 40, // raw strength ceiling, normalized to the pool's best
  hits: 18, // per element of the pal the target is weak to
  takes: -15, // per element of the pal the target attacks super-effectively
  attackChange: 14, // partner skill arms the player with a counter-element
  buffAlly: 8, // per teammate the element-scoped damage buff reaches (cap 2)
  buffDrops: 3, // loot buff with at least one teammate to land on
  pair: 20, // the species its partner skill names is on the team
  mount: 6, // first mount on the team...
  mountSpeed: 1 / 400, // ...faster ones a touch above slower ones
  coverage: 10, // per element newly answered, when there is no target
};
export type TeamWeights = typeof TEAM_WEIGHTS;
const W = TEAM_WEIGHTS;

const elementNames: Record<string, string> = { Electricity: "Electric" };
/** Chip-length element wording: the catalog's "Electricity" reads as a
 * noun pile in "Electricity attacks" — everywhere else keeps catalog names. */
function elName(el: string): string {
  return elementNames[el] ?? el;
}

function palElements(characterId: string): string[] {
  return palEntry(characterId)?.elements ?? [];
}

/** Elements this pal's own attacks answer super-effectively: its types, plus
 * a partner skill that changes the player's attack element counts for the
 * player's side of the fight. */
function offense(p: TeamPal): string[] {
  const out = [...palElements(p.characterId)];
  const ac = partnerSkill(p.characterId)?.ac;
  if (ac && !out.includes(ac)) out.push(ac);
  return out;
}

/** The defending elements an attacking element set answers. */
function coveredBy(attacking: string[]): Set<string> {
  const out = new Set<string>();
  for (const defender of ELEMENTS) {
    if (elementCounters([defender]).some((e) => attacking.includes(e))) out.add(defender);
  }
  return out;
}

export interface TeamContext {
  target: TeamTarget | null;
  /** Pals already on the team — scoring is marginal against them. */
  team: TeamPal[];
}

/**
 * Score one candidate against the current team and target. Marginal by
 * design: the same pal scores differently once the team already has a mount,
 * covers its element, or contains the species its partner skill boosts.
 *
 * `weights` reweighs the factors — the build variants lean on this to make
 * "play it safe" and "hit hardest" real alternatives rather than reshuffles.
 */
export function scoreCandidate<P extends TeamPal>(
  p: P,
  ctx: TeamContext,
  poolBestPower: number,
  weights: TeamWeights = W,
): ScoredPal<P> {
  const reasons: Reason[] = [];
  const warnings: Reason[] = [];
  const target = ctx.target;
  const team = ctx.team.filter((t) => t.key !== p.key);
  const skill = partnerSkill(p.characterId);
  const elements = palElements(p.characterId);

  const stats = palEffectiveStats(p.pal);
  if (stats && poolBestPower > 0) {
    const v = Math.round((powerScore(stats) / poolBestPower) * weights.power);
    reasons.push({ kind: "power", label: `strength ${v}/${weights.power}`, value: v });
  }

  if (target) {
    const weakTo = elementCounters(target.elements);
    for (const el of elements) {
      // Which of the target's elements this one answers, for the chip text.
      const beaten = target.elements.find((te) => elementCounters([te]).includes(el));
      if (beaten) reasons.push({ kind: "edge", label: `beats ${elName(beaten)}`, element: el, value: weights.hits });
    }
    for (const el of elements) {
      if (elementCounters([el]).some((e) => target.elements.includes(e)))
        warnings.push({ kind: "edge", label: "takes super-effective hits", element: el, value: weights.takes });
    }
    // Not redundant with the pal's own element even when they match (they
    // almost always do): the element scores the pal's damage, this arms the
    // player's. It's why Anubis outranks a plain Earth pal into a tower.
    if (skill?.ac && weakTo.includes(skill.ac)) {
      reasons.push({ kind: "partner", label: `${elName(skill.ac)} attacks for you`, element: skill.ac, value: weights.attackChange });
    }
  } else {
    // No fight picked: reward what this pal newly answers.
    const covered = coveredBy(ctx.team.flatMap((t) => (t.key === p.key ? [] : offense(t))));
    const added = [...coveredBy(offense(p))].filter((el) => !covered.has(el));
    for (const el of added) reasons.push({ kind: "coverage", label: `covers ${elName(el)}`, element: el, value: weights.coverage });
  }

  if (skill?.eb) {
    const [el, kind] = skill.eb;
    const allies = team.filter((t) => palElements(t.characterId).includes(el)).length;
    if (kind === "attack" && allies > 0) {
      reasons.push({ kind: "partner", label: `arms ${elName(el)} allies ×${allies}`, element: el, value: weights.buffAlly * Math.min(allies, 2) });
    } else if (allies > 0) {
      reasons.push({ kind: "partner", label: `boosts ${elName(el)} allies`, element: el, value: weights.buffDrops });
    }
  }

  if (skill?.pb) {
    const [species] = skill.pb;
    if (team.some((t) => palKey(t.characterId) === species))
      reasons.push({ kind: "pair", label: `pairs with ${palName(species)}`, value: weights.pair });
  }
  // The reciprocal read: a teammate's partner skill names this species.
  for (const t of team) {
    const pb = partnerSkill(t.characterId)?.pb;
    if (pb && pb[0] === palKey(p.characterId)) {
      reasons.push({ kind: "pair", label: `boosted by ${palName(t.characterId)}`, value: weights.pair });
      break;
    }
  }

  // weights.mount === 0 turns the whole factor off, speed bonus included —
  // the "hit hardest" build variant doesn't care how anyone gets there.
  if (skill?.m && weights.mount > 0 && !team.some((t) => partnerSkill(t.characterId)?.m)) {
    const speed = workProfile(p.characterId)?.r ?? 0;
    reasons.push({ kind: "mount", label: MOUNT_LABELS[skill.m], value: Math.round(weights.mount + speed * weights.mountSpeed) });
  }

  const score = reasons.reduce((s, r) => s + r.value, 0) + warnings.reduce((s, r) => s + r.value, 0);
  return { pal: p, score, reasons, warnings };
}

/** The pool's best strength, the yardstick every candidate is measured by. */
export function bestPower(pool: TeamPal[]): number {
  let best = 0;
  for (const p of pool) {
    const s = palEffectiveStats(p.pal);
    if (s) best = Math.max(best, powerScore(s));
  }
  return best;
}

/** Every candidate scored against the current team, best first. */
export function rankCandidates<P extends TeamPal>(pool: P[], ctx: TeamContext): ScoredPal<P>[] {
  const yardstick = bestPower(pool);
  const inTeam = new Set(ctx.team.map((t) => t.key));
  return pool
    .filter((p) => !inTeam.has(p.key))
    .map((p) => scoreCandidate(p, ctx, yardstick))
    .sort((a, b) => b.score - a.score);
}

/**
 * Fill the team's empty slots, greedily: each pick is the best marginal
 * candidate against the team as it stands, so a second Fire pal is worth
 * less once the first covers that edge, and the mount bonus is spent once.
 * One pal per species — a hand-picked duplicate is respected, the filler
 * just never creates one. Locked picks always stay.
 */
export function fillTeam<P extends TeamPal>(
  pool: P[],
  locked: P[],
  target: TeamTarget | null,
  size = 5,
  weights: TeamWeights = W,
): P[] {
  const team = [...locked];
  const yardstick = bestPower(pool);
  while (team.length < size) {
    const used = new Set(team.map((t) => t.key));
    const species = new Set(team.map((t) => palKey(t.characterId)));
    let best: ScoredPal<P> | null = null;
    for (const p of pool) {
      if (used.has(p.key) || species.has(palKey(p.characterId))) continue;
      const scored = scoreCandidate(p, { target, team }, yardstick, weights);
      if (!best || scored.score > best.score) best = scored;
    }
    if (!best) break;
    team.push(best.pal);
  }
  return team;
}

// ---------------------------------------------------------------------------
// Build proposals
// ---------------------------------------------------------------------------

/** What a proposed party trades away and what it gets — the cost/benefit
 * line under each build card, counted rather than adjectived. */
export interface BuildSummary {
  /** Members whose own elements answer the target. */
  counters: number;
  /** Members whose partner skill adds offense — arming the player or allies. */
  armed: number;
  /** Members the target's own attacks hit super-effectively. */
  exposed: number;
  synergies: number;
  mount?: MountKind;
  avgLevel: number;
}

export interface BuildOption<P extends TeamPal = TeamPal> {
  id: string;
  label: string;
  /** The lean this build takes, in one sentence. */
  blurb: string;
  team: P[];
  summary: BuildSummary;
}

/** The three leans. Only the deltas from the default weights are listed —
 * everything else scores identically, so the builds differ exactly where
 * the blurbs say they do. */
const BUILD_VARIANTS: { id: string; label: string; blurb: string; weights: Partial<TeamWeights> }[] = [
  { id: "best", label: "Best overall", blurb: "Counters, strength and utility weighed together.", weights: {} },
  {
    id: "safe",
    label: "Play it safe",
    blurb: "Avoids anything the fight hits super-effectively, even at some strength cost.",
    weights: { takes: -60 },
  },
  {
    id: "power",
    label: "Hit hardest",
    blurb: "Raw damage — the strongest counters, mounts and loot ignored.",
    weights: { power: 60, mount: 0, buffDrops: 0 },
  },
];

export function buildSummary(team: TeamPal[], target: TeamTarget | null): BuildSummary {
  const analysis = analyzeTeam(team, target);
  const weakTo = target ? elementCounters(target.elements) : [];
  const targetEls = target?.elements ?? [];
  return {
    counters: team.filter((p) => palElements(p.characterId).some((el) => weakTo.includes(el))).length,
    armed: team.filter((p) => {
      const s = partnerSkill(p.characterId);
      if (s?.ac && weakTo.includes(s.ac)) return true;
      return s?.eb?.[1] === "attack" && team.some((t) => t.key !== p.key && palElements(t.characterId).includes(s.eb![0]));
    }).length,
    exposed: team.filter((p) =>
      palElements(p.characterId).some((el) => elementCounters([el]).some((e) => targetEls.includes(e))),
    ).length,
    synergies: analysis.synergies.length,
    mount: analysis.mount,
    avgLevel: team.length ? Math.round(team.reduce((s, p) => s + p.pal.level, 0) / team.length) : 0,
  };
}

/**
 * Up to three full-party proposals with different leans, each summarized so
 * the choice is a real cost/benefit read rather than three reshuffles. A
 * variant that lands on the same five as an earlier one is dropped — when
 * the pool is small or one answer dominates, one card is the honest output.
 * Locked picks appear in every build; excluded pals are the caller's cut,
 * made by filtering the pool.
 */
export function suggestBuilds<P extends TeamPal>(pool: P[], locked: P[], target: TeamTarget | null): BuildOption<P>[] {
  const out: BuildOption<P>[] = [];
  const seen = new Set<string>();
  for (const v of BUILD_VARIANTS) {
    const team = fillTeam(pool, locked, target, 5, { ...W, ...v.weights });
    if (team.length === 0) continue;
    const signature = team
      .map((t) => t.key)
      .sort()
      .join("|");
    if (seen.has(signature)) continue;
    seen.add(signature);
    out.push({ id: v.id, label: v.label, blurb: v.blurb, team, summary: buildSummary(team, target) });
  }
  return out;
}

// ---------------------------------------------------------------------------
// Team analysis
// ---------------------------------------------------------------------------

export interface TeamAnalysis {
  /** Which of the nine elements this team's attacks answer. */
  coverage: Set<string>;
  /** Synergies actually active in this lineup. */
  synergies: Reason[];
  /** What's missing or risky — empty for a clean sheet. */
  gaps: Reason[];
  mount?: MountKind;
}

export function analyzeTeam(team: TeamPal[], target: TeamTarget | null): TeamAnalysis {
  const coverage = coveredBy(team.flatMap(offense));
  const synergies: Reason[] = [];
  const gaps: Reason[] = [];
  let mount: MountKind | undefined;

  for (const p of team) {
    const skill = partnerSkill(p.characterId);
    if (!skill) continue;
    if (skill.m && !mount) mount = skill.m;
    if (skill.eb) {
      const [el, kind] = skill.eb;
      const allies = team.filter((t) => t.key !== p.key && palElements(t.characterId).includes(el)).length;
      if (allies > 0)
        synergies.push({
          kind: "partner",
          label: `${palName(p.characterId)} ${kind === "attack" ? "arms" : "boosts"} ${allies} ${elName(el)} ${allies === 1 ? "ally" : "allies"}`,
          element: el,
          value: 0,
        });
    }
    if (skill.pb && team.some((t) => palKey(t.characterId) === skill.pb![0]))
      synergies.push({ kind: "pair", label: `${palName(p.characterId)} boosts ${palName(skill.pb[0])}`, value: 0 });
    if (skill.ac && target && elementCounters(target.elements).includes(skill.ac))
      synergies.push({ kind: "partner", label: `${palName(p.characterId)} arms you with ${elName(skill.ac)}`, element: skill.ac, value: 0 });
  }

  if (team.length > 0) {
    if (target && target.elements.length > 0) {
      const weakTo = elementCounters(target.elements);
      if (!team.some((p) => offense(p).some((el) => weakTo.includes(el))))
        gaps.push({ kind: "edge", label: `nobody hits ${target.label.replace(/ pals$/, "")} hard`, value: 0 });
      const exposed = team.filter((p) =>
        palElements(p.characterId).some((el) => elementCounters([el]).some((e) => target.elements.includes(e))),
      ).length;
      if (exposed > 0)
        gaps.push({ kind: "edge", label: `${exposed} take${exposed === 1 ? "s" : ""} super-effective hits`, value: 0 });
    }
    if (!mount) gaps.push({ kind: "mount", label: "no mount", value: 0 });
  }

  return { coverage, synergies, gaps, mount };
}

export { ELEMENTS as TEAM_ELEMENTS };
