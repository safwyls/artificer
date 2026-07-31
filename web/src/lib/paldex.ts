import palDeck from "../data/palDeck.json";
import palDex from "../data/palDex.json";
import palStats from "../data/palStats.json";
import passiveSkills from "../data/passiveSkills.json";
import passiveTiers from "../data/passiveTiers.json";
import activeSkills from "../data/activeSkills.json";

/**
 * Lookups that turn a save file's internal ids into what the game actually
 * calls things — "PinkCat" is Cattiva, "PAL_ALLAttack_up1" is Brave.
 *
 * Names and icons are vendored from palworld-server-manager (MIT), which
 * sources the catalogs from palworld-save-pal's English localization; skill
 * and passive descriptions are merged from deafdudecomputers/PalworldSaveTools
 * (MIT). See web/public/pal-icons/README.md. Every lookup falls back to a
 * humanized id, so a pal or skill added by a game update still shows a
 * readable name rather than disappearing or leaking a raw internal id.
 */

interface PalEntry {
  name: string;
  elements: string[];
  rarity: number;
}

/** Skills and passives are stored as {n: name, d: description} to keep the
 * chunk small; `d` is often just the name repeated, which callers drop. */
interface NamedEntry {
  n: string;
  /** Null for entries the catalog has no blurb for. */
  d: string | null;
}

const dex = palDex as Record<string, PalEntry>;
const passives = passiveSkills as Record<string, NamedEntry>;
const tiers = passiveTiers as Record<string, number>;
const actives = activeSkills as Record<string, NamedEntry>;
const stats = palStats as Record<string, { hp: number; stomach: number }>;

// Decorations the game hangs on capture ids: spawn-context prefixes and
// suffixes like BOSS_KingWhale_otomo (a captured raid companion). Shared by
// the dex/icon/stats key and the Paldeck-number lookup so every view folds
// a decorated capture into its species the same way.
const DECOR_PREFIX = /^(boss|predator|raid|summon|gym)_/;
const DECOR_SUFFIX = /_(oilrig|largeoilrig|minioilrig|tower|invader|max|avatar|otomo|servant|quest|enemy|friend|\d+)$/;

const keyCache = new Map<string, string>();

/** Icons, dex entries and stat tables share one key: the id lowercased,
 * stripped of the BOSS_ alpha prefix — and, when that alone misses the
 * catalog, of the other spawn decorations until something matches. An id
 * the catalog simply doesn't know keeps its plain de-bossed form, so icon
 * paths stay stable for genuinely unknown pals. */
export function palKey(characterId: string): string {
  const raw = characterId.toLowerCase().replace(/^boss_/, "");
  const cached = keyCache.get(raw);
  if (cached !== undefined) return cached;
  let key = raw;
  while (!(key in dex)) {
    let next = key.replace(DECOR_PREFIX, "");
    if (next === key) next = key.replace(DECOR_SUFFIX, "");
    if (next === key) {
      key = raw;
      break;
    }
    key = next;
  }
  keyCache.set(raw, key);
  return key;
}

export function palEntry(characterId: string): PalEntry | undefined {
  return dex[palKey(characterId)];
}

export function palName(characterId: string): string {
  return palEntry(characterId)?.name ?? characterId;
}

export function palIconUrl(characterId: string): string {
  // BASE_URL-prefixed so subpath deployments (e.g. the static demo) resolve.
  return `${import.meta.env.BASE_URL}pal-icons/${palKey(characterId)}.webp`;
}

/** Turns a leftover internal id into something readable, for the rare code
 * the catalog has no localized name for — mostly boss-only skills and utility
 * buffs a player never sees on their own pals. "Unique_WorldTreeDragon_BigBang"
 * becomes "World Tree Dragon Big Bang", "AccuracyDecrease" becomes "Accuracy
 * Decrease". */
function humanizeCode(code: string): string {
  return code
    .replace(/^(Unique_|PAL_|EPalWazaID::)/i, "")
    .replace(/[_:]+/g, " ")
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/\s+/g, " ")
    .trim();
}

export function passiveName(code: string): string {
  const n = passives[code]?.n;
  return n && n !== code ? n : humanizeCode(code);
}

/** A passive's tier as the game ranks it: 1–3 (up arrows), −1…−3 (down
 * arrows), 4 for the Rainbow tier (Legend, Lucky, …), 5 for World Tree
 * passives. 0 for codes the tier catalog doesn't cover — gear, gym-boss and
 * test passives a player's own pals don't normally carry. Scraped from
 * game8.co/games/Palworld/archives/439667 (post-Feybreak tier table). */
export function passiveTier(code: string): number {
  return tiers[code] ?? 0;
}

/** Description, or "" when the catalog just repeats the name (many do). */
export function passiveDescription(code: string): string {
  const entry = passives[code];
  if (!entry?.d || entry.d === entry.n) return "";
  return entry.d;
}

export function skillName(code: string): string {
  const n = actives[code]?.n;
  return n && n !== code ? n : humanizeCode(code);
}

export function skillDescription(code: string): string {
  const entry = actives[code];
  if (!entry?.d || entry.d === entry.n) return "";
  return entry.d;
}

/** Base max HP and stomach for the species, used to show a pal's current
 * values as a proportion. Undefined for anything not in the catalog. */
export function palBaseStats(characterId: string): { hp: number; stomach: number } | undefined {
  return stats[palKey(characterId)];
}

/** Paldeck numbers as the game labels them — "94" for a base pal, "94B" for
 * its subspecies — vendored from palworld-save-pal's pal_deck_index (see
 * web/public/pal-icons/README.md). Lookup strips the same capture decorations
 * breeding does ("PREDATOR_", "SUMMON_…_MAX"), so a captured variant shows
 * its species' number. Null for NPCs and unreleased pals. */
const deck = palDeck as Record<string, string>;

export function palDeckNo(characterId: string): string | null {
  let key = palKey(characterId);
  for (;;) {
    const hit = deck[key];
    if (hit) return hit;
    let next = key.replace(DECOR_PREFIX, "");
    if (next === key) next = key.replace(DECOR_SUFFIX, "");
    if (next === key) return null;
    key = next;
  }
}

/** Orders deck labels naturally: 94 before 94B before 95; unnumbered last. */
export function palDeckSortValue(characterId: string): number {
  const label = palDeckNo(characterId);
  if (!label) return Number.MAX_SAFE_INTEGER;
  const suffix = label.replace(/^\d+/, "");
  return parseInt(label, 10) * 8 + (suffix ? suffix.charCodeAt(0) - 65 : -1) + 1;
}

function deckLabelSort(a: string, b: string): number {
  const na = parseInt(a, 10) - parseInt(b, 10);
  if (na !== 0) return na;
  return a.localeCompare(b); // "94" before "94B"
}

/** The full Paldeck: every catchable entry ("1"…"204" plus B variants),
 * each with a representative character id for icon/name display. Decorated
 * spawn ids (SUMMON_…, …_oilrig) share their species' label; the shortest
 * id is the plain species. This is the universe Paldex completion is
 * measured against. */
export const DECK_ENTRIES: { label: string; characterId: string }[] = (() => {
  const byLabel = new Map<string, string>();
  for (const [id, label] of Object.entries(deck)) {
    const existing = byLabel.get(label);
    if (!existing || id.length < existing.length) byLabel.set(label, id);
  }
  return [...byLabel.entries()]
    .map(([label, characterId]) => ({ label, characterId }))
    .sort((a, b) => deckLabelSort(a.label, b.label));
})();

// Completion percentages track the numbered entries, like the game's own
// counter — B-subspecies sit under the same number in-game.
export const DECK_BASE_ENTRIES = DECK_ENTRIES.filter((e) => /^\d+$/.test(e.label));
export const DECK_VARIANT_ENTRIES = DECK_ENTRIES.filter((e) => !/^\d+$/.test(e.label));

/** Rarity 8+ is the game's own threshold for a rare (blue-tier) pal, 12+ for
 * legendary — used only to tint the icon frame. */
export function rarityTier(rarity: number): "legendary" | "rare" | "common" {
  if (rarity >= 12) return "legendary";
  if (rarity >= 8) return "rare";
  return "common";
}

export const ELEMENT_COLORS: Record<string, string> = {
  Normal: "#9C9186",
  Fire: "#E8491D",
  Water: "#5B9BD5",
  Leaf: "#4A9D7C",
  Electricity: "#F2A93B",
  Ice: "#7FC8E8",
  Earth: "#A9773F",
  Dark: "#6B4A7E",
  Dragon: "#8B3A9E",
};

export function elementColor(element: string): string {
  return ELEMENT_COLORS[element] ?? "#9C9186";
}
