import { api, type AdvisorTool, type AdvisorToolCall } from "./api";
import { breedChild, parentPairsFor, BREEDABLE } from "./breeding";
import { eggsForConfidence, expectedEggs, passiveOdds } from "./inheritance";
import { computeStats } from "./stats";
import { palName } from "./paldex";

/**
 * The advisor's tools: the same calculators the Breeding and Stats tabs
 * run, exposed to the model. Definitions and execution live together here —
 * the definitions ride to the server with each question (which forwards
 * them to the model verbatim), the model's calls come back, and this module
 * executes them. The server never learns what a breeding table is.
 *
 * Results are prose, not JSON: the model reads them the way a player would,
 * and a sentence survives species the tables don't know ("unknown species
 * X") better than a null field.
 */

// The model will name species the way players do ("Frostallion Noct"), the
// save names them internally ("IceHorse_Dark"); accept either.
let nameToId: Map<string, string> | null = null;
function resolveSpecies(input: string): string {
  nameToId ??= new Map(BREEDABLE.map((b) => [b.name.toLowerCase(), b.id]));
  return nameToId.get(input.trim().toLowerCase()) ?? input.trim();
}

const str = (v: unknown): string => (typeof v === "string" ? v : String(v ?? ""));
const num = (v: unknown, fallback: number): number => {
  const n = typeof v === "number" ? v : Number(v);
  return Number.isFinite(n) ? n : fallback;
};
const pct = (p: number): string => `${(p * 100).toFixed(1)}%`;

export const ADVISOR_TOOLS: AdvisorTool[] = [
  {
    name: "breed_child",
    description:
      "The exact child species from breeding two Palworld species, from the game's own breeding table. Use in-game species names.",
    inputSchema: {
      type: "object",
      properties: {
        parentA: { type: "string", description: "First parent species name" },
        parentB: { type: "string", description: "Second parent species name" },
      },
      required: ["parentA", "parentB"],
    },
  },
  {
    name: "parents_for",
    description:
      "Every parent pair that breeds into a target species, from the game's own breeding table. Unique (hand-authored) combos are listed first — for many special pals they are the only way.",
    inputSchema: {
      type: "object",
      properties: {
        child: { type: "string", description: "Target species name" },
      },
      required: ["child"],
    },
  },
  {
    name: "inheritance_odds",
    description:
      "Odds that a bred child inherits a desired set of passive skills, given how many distinct passives the two parents carry between them. Also gives expected egg counts.",
    inputSchema: {
      type: "object",
      properties: {
        parentPassiveCount: { type: "number", description: "Distinct passives across both parents (0-8)" },
        desiredCount: { type: "number", description: "How many of those passives the child must have (0-4)" },
      },
      required: ["parentPassiveCount", "desiredCount"],
    },
  },
  {
    name: "search_palcon_docs",
    description:
      "Search palcon's own documentation — how the console works: features, per-view visibility switches, backups, the sidecar agent, provisioning, setup. Use for questions about palcon itself, not about the game.",
    inputSchema: {
      type: "object",
      properties: {
        query: { type: "string", description: "Search terms, e.g. 'hide player from map'" },
      },
      required: ["query"],
    },
  },
  {
    name: "palworld_wiki",
    description:
      "Search the Palworld wiki for game facts beyond the calculators — drops, item locations, boss strategies, mechanics. Prefer this over memory for anything that may have changed in a game update.",
    inputSchema: {
      type: "object",
      properties: {
        query: { type: "string", description: "Search terms, e.g. 'Frostallion Noct location'" },
      },
      required: ["query"],
    },
  },
  {
    name: "estimate_stats",
    description:
      "Estimated HP/Attack/Defense for a species at a given level and investment — the same math as the console's Stats tab. Useful for comparing investment options (condense vs souls vs levels).",
    inputSchema: {
      type: "object",
      properties: {
        species: { type: "string", description: "Species name" },
        level: { type: "number", description: "Pal level (1-65)" },
        ivHp: { type: "number", description: "HP talent 0-100, default 50" },
        ivAttack: { type: "number", description: "Attack talent 0-100, default 50" },
        ivDefense: { type: "number", description: "Defense talent 0-100, default 50" },
        condenserStars: { type: "number", description: "Condenser stars 0-4, default 0" },
        soulRank: { type: "number", description: "Soul rank applied to all three stats 0-20, default 0" },
      },
      required: ["species", "level"],
    },
  },
];

function runBreedChild(args: Record<string, unknown>): string {
  const a = resolveSpecies(str(args.parentA));
  const b = resolveSpecies(str(args.parentB));
  const result = breedChild(a, b);
  if (!result) {
    return `${str(args.parentA)} × ${str(args.parentB)}: no breeding result — one of the species is unknown or unbreedable. Check the spelling against in-game names.`;
  }
  const alt = result.altChildId ? ` (or ${palName(result.altChildId)}, depending on which parent is which gender)` : "";
  return `${palName(a)} × ${palName(b)} → ${palName(result.childId)}${alt}${result.special ? " — a unique combo" : ""}.`;
}

function runParentsFor(args: Record<string, unknown>): string {
  const raw = str(args.child);
  const target = resolveSpecies(raw);
  const pairs = parentPairsFor(target);
  if (pairs.length === 0) {
    return `No parent pair breeds ${raw} — the species is unknown, unbreedable, or only obtainable by catching. Check the spelling against in-game names.`;
  }
  const cap = 30;
  const lines = pairs
    .slice(0, cap)
    .map((p) => `- ${palName(p.aId)} × ${palName(p.bId)}${p.special ? " (unique combo)" : ""}`);
  const more = pairs.length > cap ? `\n…and ${pairs.length - cap} more standard pairs.` : "";
  return `${pairs.length} pair(s) breed ${palName(target)}:\n${lines.join("\n")}${more}`;
}

function runInheritanceOdds(args: Record<string, unknown>): string {
  const pool = num(args.parentPassiveCount, -1);
  const desired = num(args.desiredCount, -1);
  const odds = passiveOdds(pool, desired);
  if (pool < 0 || desired < 0 || !odds) {
    return "Invalid inputs: parentPassiveCount must be 0-8 and desiredCount 0-4, with desiredCount ≤ parentPassiveCount.";
  }
  const eggs = expectedEggs(odds.atLeast);
  const confident = eggsForConfidence(odds.atLeast);
  return (
    `With ${pool} distinct passives across the parents, wanting ${desired} of them: ` +
    `${pct(odds.atLeast)} per egg (all ${desired} present, extras allowed), ${pct(odds.exact)} for exactly those and nothing else. ` +
    `Expect about ${Number.isFinite(eggs) ? Math.ceil(eggs) : "∞"} eggs on average, ${Number.isFinite(confident) ? confident : "∞"} for 90% confidence.`
  );
}

function runEstimateStats(args: Record<string, unknown>): string {
  const raw = str(args.species);
  const species = resolveSpecies(raw);
  const stats = computeStats({
    characterId: species,
    level: num(args.level, 1),
    ivHp: num(args.ivHp, 50),
    ivAttack: num(args.ivAttack, 50),
    ivDefense: num(args.ivDefense, 50),
    condenser: num(args.condenserStars, 0),
    soulHp: num(args.soulRank, 0),
    soulAttack: num(args.soulRank, 0),
    soulDefense: num(args.soulRank, 0),
  });
  if (!stats) return `No combat data for ${raw} — check the spelling against in-game names.`;
  return `${palName(species)} at level ${num(args.level, 1)}: about ${stats.hp} HP, ${stats.attack} Attack, ${stats.defense} Defense.`;
}

// The docs change only with the binary, so one fetch serves the session.
let docsPromise: Promise<{ name: string; content: string }[]> | null = null;

/** Heading-scored section search over the embedded project docs. Plain term
 * counting, not embeddings: the corpus is a dozen markdown files, and the
 * model's queries are already well-phrased. */
async function runDocsSearch(args: Record<string, unknown>): Promise<string> {
  const query = str(args.query).toLowerCase();
  const terms = query.split(/\W+/).filter((t) => t.length > 2);
  if (terms.length === 0) return "Give search terms, e.g. 'hide player from map'.";
  docsPromise ??= api.docs().then((r) => r.docs);
  let docs;
  try {
    docs = await docsPromise;
  } catch {
    docsPromise = null;
    return "The palcon docs couldn't be loaded right now.";
  }
  const sections: { title: string; body: string; score: number }[] = [];
  for (const doc of docs) {
    for (const section of doc.content.split(/\n(?=#{1,3} )/)) {
      const lower = section.toLowerCase();
      let score = 0;
      for (const term of terms) score += lower.split(term).length - 1;
      if (score > 0) {
        const heading = section.split("\n", 1)[0].replace(/^#+\s*/, "");
        sections.push({ title: `${doc.name} › ${heading}`, body: section.slice(0, 1500), score });
      }
    }
  }
  if (sections.length === 0) return `Nothing in the palcon docs matches "${str(args.query)}".`;
  sections.sort((a, b) => b.score - a.score);
  return sections
    .slice(0, 3)
    .map((s) => `--- ${s.title} ---\n${s.body}`)
    .join("\n\n");
}

/** Palworld wiki lookup, straight from the browser — the Fandom MediaWiki
 * API allows anonymous cross-origin reads via origin=*, so the Go server
 * never proxies (and never grows an outbound-fetch surface). */
async function runWikiSearch(args: Record<string, unknown>): Promise<string> {
  const query = str(args.query).trim();
  if (!query) return "Give search terms, e.g. 'Frostallion Noct location'.";
  const base = "https://palworld.fandom.com/api.php";
  try {
    const searchRes = await fetch(
      `${base}?action=query&list=search&srsearch=${encodeURIComponent(query)}&srlimit=4&format=json&origin=*`,
    );
    const search = (await searchRes.json()) as { query?: { search?: { title: string }[] } };
    const hits = search.query?.search ?? [];
    if (hits.length === 0) return `The Palworld wiki has no page matching "${query}".`;
    const title = hits[0].title;
    const pageRes = await fetch(
      `${base}?action=query&prop=extracts&explaintext=1&titles=${encodeURIComponent(title)}&format=json&origin=*`,
    );
    const page = (await pageRes.json()) as { query?: { pages?: Record<string, { extract?: string }> } };
    const extract = Object.values(page.query?.pages ?? {})[0]?.extract ?? "";
    const others = hits
      .slice(1)
      .map((h) => h.title)
      .join(", ");
    const body = extract ? extract.slice(0, 4000) : "(The page has no plain-text summary.)";
    return `Wiki page "${title}":\n${body}${others ? `\n\nOther matching pages: ${others}.` : ""}`;
  } catch {
    return "The Palworld wiki couldn't be reached right now.";
  }
}

const RUNNERS: Record<string, (args: Record<string, unknown>) => string | Promise<string>> = {
  breed_child: runBreedChild,
  parents_for: runParentsFor,
  inheritance_odds: runInheritanceOdds,
  estimate_stats: runEstimateStats,
  search_palcon_docs: runDocsSearch,
  palworld_wiki: runWikiSearch,
};

/** Executes one model-requested call. Errors come back as text — the model
 * can read "unknown species" and correct itself; a thrown exception can't. */
export async function runAdvisorTool(name: string, args: Record<string, unknown>): Promise<string> {
  const runner = RUNNERS[name];
  if (!runner) return `Unknown tool ${name}.`;
  try {
    return await runner(args ?? {});
  } catch (e) {
    return `The ${name} tool failed: ${e instanceof Error ? e.message : String(e)}`;
  }
}

/** The transcript's one-line account of a call, in player terms. */
export function describeToolCall(call: AdvisorToolCall): string {
  const args = call.args ?? {};
  switch (call.name) {
    case "breed_child":
      return `checked ${str(args.parentA)} × ${str(args.parentB)}`;
    case "parents_for":
      return `looked up pairs that breed ${str(args.child)}`;
    case "inheritance_odds":
      return "worked out passive-inheritance odds";
    case "estimate_stats":
      return `estimated ${str(args.species)}'s stats`;
    case "search_palcon_docs":
      return `searched the palcon docs for "${str(args.query)}"`;
    case "palworld_wiki":
      return `checked the wiki for "${str(args.query)}"`;
    default:
      return `used ${call.name}`;
  }
}
