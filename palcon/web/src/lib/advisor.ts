import type { Guild } from "./api";
import { palKey, palName, passiveName } from "./paldex";
import { crewReport, dedupeSpecies, topWork, WORK_TYPES, type CrewPal } from "./crew";
import { palEffectiveStats, powerScore } from "./stats";
import { nearestLandmark } from "./pois";

/**
 * Context builder for the pal advisor: the compact JSON summary the browser
 * sends with each question. Built here, not on the server, because every
 * derived number (effective work levels, condenser math, stat estimates)
 * already lives in these client-side calculators — the Go side never learned
 * the vendored catalogs, and the advisor shouldn't be the reason it has to.
 *
 * Built from the same /pals payload the page renders, so player-visibility
 * hides are already applied: what a hidden player keeps off the screen also
 * stays out of the prompt.
 */

/** The slice of the calculators' SavePal the builder reads. */
export type AdvisorPal = CrewPal & {
  nickname: string;
  level: number;
  ivHp: number;
  ivAttack: number;
  ivDefense: number;
  condenser: number;
  souls: { hp: number; attack: number; defense: number };
  passives: string[];
  isAlpha: boolean;
  trust: number;
};

export interface AdvisorContext {
  /** The JSON block the server wraps in <server_data> for the model. */
  json: string;
  /** For the "what the advisor sees" caption. */
  counts: { pals: number; bases: number; players: number };
  /** Questions derived from what the save actually shows — sick crews,
   * available swaps, condenser fodder — so the empty state invites with the
   * player's own problems instead of canned examples. */
  suggestions: string[];
}

/** A pal named the way the advisor should say it back: nickname first,
 * species in parentheses when a nickname hides it. */
function displayName(p: AdvisorPal): string {
  const species = palName(p.characterId);
  return p.nickname && p.nickname !== species ? `${p.nickname} (${species})` : species;
}

function jobLabel(type: string): string {
  return WORK_TYPES.find((w) => w.id === type)?.label ?? type;
}

export function buildAdvisorContext(pals: AdvisorPal[], guilds: Guild[]): AdvisorContext {
  // Signals the suggestion builder turns into questions afterwards — the
  // loud problems (sickness, open upgrades, uncovered nights, idle hands)
  // noted here as the boards are computed.
  const sickBases: string[] = [];
  const openUpgrades: { name: string; job: string; base: string }[] = [];
  const nightlessBases: string[] = [];
  const idleNames: string[] = [];

  // Bases, labeled the way the Crew tab labels them so the advisor and the
  // board the player is looking at use the same names.
  const baseDefs: { id: string; label: string; guildName: string; memberUids: Set<string> | null }[] = [];
  const known = new Set<string>();
  for (const g of guilds) {
    const uids = new Set(g.members.map((m) => m.uid));
    g.bases.forEach((b, i) => {
      known.add(b.id);
      const mark = nearestLandmark(b.x, b.y);
      baseDefs.push({
        id: b.id,
        label: `${g.name || "Unnamed guild"} · Base ${i + 1}` + (mark ? ` · near ${mark.name}` : ""),
        guildName: g.name || "Unnamed guild",
        memberUids: uids,
      });
    });
  }
  for (const id of new Set(pals.map((p) => p.pal.baseId).filter((b) => b && !known.has(b)))) {
    baseDefs.push({ id, label: "Unlisted base", guildName: "", memberUids: null });
  }

  const bases = baseDefs.map((def) => {
    const crew = pals.filter((p) => p.pal.baseId === def.id);
    const boxes = pals.filter(
      (p) => p.pal.baseId === "" && (def.memberUids === null || def.memberUids.has(p.playerUid)),
    );
    const report = crewReport(crew, boxes);

    const baseShort = def.label.split(" · near ")[0];
    if (report.sick.length > 0) sickBases.push(baseShort);
    const bestUpgrade = report.rows
      .filter((r) => r.upgrade)
      .sort((a, b) => b.upgrade!.level - b.upgrade!.over - (a.upgrade!.level - a.upgrade!.over))[0];
    if (bestUpgrade) {
      openUpgrades.push({
        name: displayName(bestUpgrade.upgrade!.pal as AdvisorPal),
        job: bestUpgrade.label,
        base: baseShort,
      });
    }
    if (crew.length > 0 && report.nightHands === 0) nightlessBases.push(baseShort);
    for (const p of report.idle) idleNames.push(displayName(p as AdvisorPal));

    return {
      base: def.label,
      guild: def.guildName || undefined,
      crewSize: crew.length,
      appetite: report.appetite,
      nightWorkers: report.nightHands,
      sick: report.sick.map((p) => `${displayName(p as AdvisorPal)} (${(p as AdvisorPal).pal.sick})`),
      idle: report.idle.map((p) => displayName(p as AdvisorPal)),
      ranchers: report.ranchers.map((p) => displayName(p as AdvisorPal)),
      buffs: report.buffs.map((b) => `${displayName(b.pal as AdvisorPal)} gives +1 ${jobLabel(b.type)} base-wide`),
      board: report.rows
        .filter((r) => r.best > 0 || r.upgrade)
        .map((r) => ({
          job: r.label,
          best: r.best,
          night: r.night || undefined,
          hands: dedupeSpecies(r.hands)
            .slice(0, 6)
            .map(({ pal, count }) => (count > 1 ? `${palName(pal.characterId)} x${count}` : palName(pal.characterId))),
          upgrade: r.upgrade
            ? {
                swapIn: displayName(r.upgrade.pal as AdvisorPal),
                owner: r.upgrade.pal.playerName,
                from: r.upgrade.pal.where,
                level: r.upgrade.level,
                currentBest: r.upgrade.over,
              }
            : undefined,
        })),
      crew: crew.map((p) => ({
        name: displayName(p),
        owner: p.playerName,
        level: p.level,
        stars: p.condenser || undefined,
        sick: p.pal.sick || undefined,
        work: topWork(p.pal).map((w) => `${w.label} ${w.level}${w.off ? " (switched off)" : ""}`),
      })),
    };
  });

  // Per-player rollup with their strongest pals — soul/condense/breeding
  // questions are about individuals, so the strongest carry full detail.
  const byPlayer = new Map<string, AdvisorPal[]>();
  for (const p of pals) {
    const list = byPlayer.get(p.playerUid) ?? [];
    list.push(p);
    byPlayer.set(p.playerUid, list);
  }
  const players = [...byPlayer.values()].map((list) => {
    const notable = list
      .map((p) => ({ p, stats: palEffectiveStats(p.pal) }))
      .sort((a, b) => (b.stats ? powerScore(b.stats) : 0) - (a.stats ? powerScore(a.stats) : 0))
      .slice(0, 10)
      .map(({ p, stats }) => ({
        name: displayName(p),
        where: p.where,
        level: p.level,
        talents: `${p.ivHp}/${p.ivAttack}/${p.ivDefense}`,
        stars: p.condenser,
        souls: p.souls.hp + p.souls.attack + p.souls.defense > 0 ? p.souls : undefined,
        trust: p.trust,
        passives: p.passives.map(passiveName),
        alpha: p.isAlpha || undefined,
        power: stats ? Math.round(powerScore(stats)) : undefined,
      }));
    return {
      player: list[0].playerName,
      pals: list.length,
      atBases: list.filter((p) => p.pal.baseId !== "").length,
      strongest: notable,
    };
  });

  // Spare same-species copies are condenser fodder; party and base pals are
  // in use, so only boxed copies count as spare.
  const species = new Map<string, { total: number; spares: number }>();
  for (const p of pals) {
    const k = palKey(p.characterId);
    const entry = species.get(k) ?? { total: 0, spares: 0 };
    entry.total += 1;
    if (p.pal.baseId === "" && p.where !== "Party") entry.spares += 1;
    species.set(k, entry);
  }
  const duplicates = [...species.entries()]
    .filter(([, v]) => v.total > 1 && v.spares > 0)
    .sort((a, b) => b[1].spares - a[1].spares)
    .slice(0, 40)
    .map(([k, v]) => ({ species: palName(k), total: v.total, spareInBoxes: v.spares }));

  // Suggestions: one guaranteed chip for the loudest problem (a sick crew),
  // then a random draw across distinct categories so the invitations read
  // like different kinds of questions, not four spellings of one. Random on
  // purpose — the pool re-rolls per page load, which keeps the chips fresh
  // for someone who opens the chat every day.
  const pick = <T,>(arr: T[]): T => arr[Math.floor(Math.random() * arr.length)];
  const suggestions: string[] = [];
  if (sickBases.length > 0) suggestions.push(`Why have pals stopped working at ${pick(sickBases)}?`);

  const pools: string[][] = [];
  if (openUpgrades.length > 0) {
    const up = pick(openUpgrades);
    pools.push([
      `Should I swap ${up.name} into ${up.job} at ${up.base}?`,
      `Who should cover ${up.job} at ${up.base}?`,
    ]);
  }
  if (nightlessBases.length > 0) {
    pools.push([`Nobody works nights at ${pick(nightlessBases)} — who should I add?`]);
  }
  if (idleNames.length > 0) {
    pools.push([`${pick(idleNames)} isn't suited to any work — what should I do with them?`]);
  }
  if (duplicates.length > 0) {
    const dup = duplicates[0];
    pools.push(
      dup.spareInBoxes >= 4
        ? [`What should I condense first?`, `Are my ${dup.spareInBoxes} spare ${dup.species} worth condensing?`]
        : [`Is ${dup.species} worth collecting more of to condense?`],
    );
    pools.push([`What does ${dup.species} drop?`, `Where do wild ${dup.species} spawn?`]);
  }
  pools.push(["Which pals deserve my Pal Souls?", "Where should my soul upgrades go?"]);
  pools.push(["What's worth breeding next?", "How do I breed stronger workers?"]);
  pools.push(["What should I focus on next?", "How do I level my pals faster?"]);
  pools.push(["What can palcon's calculators do?", "How does the breeding path finder work?"]);

  // Shuffle the categories, then take one question from each until four
  // chips stand — every draw a different kind of ask.
  pools.sort(() => Math.random() - 0.5);
  for (const pool of pools) {
    if (suggestions.length >= 4) break;
    suggestions.push(pick(pool));
  }

  return {
    json: JSON.stringify({ bases, players, duplicates }),
    counts: { pals: pals.length, bases: bases.length, players: players.length },
    suggestions,
  };
}
