import bossFights from "../data/bossFights.json";
import palDex from "../data/palDex.json";
import { palName } from "./paldex";
import type { PlayerRecords } from "./api";

/**
 * Turns a player save's RecordData keys into the names the game uses.
 *
 * The names themselves are already vendored — palDex.json carries the boss
 * duos (`gym_elecpanda` is "Zoe & Grizzbolt"), the raid bosses (`raid_nightlady`
 * is "Bellanoir") and all 34 human bounty targets (`boss_hunter_rifle` is
 * "Hawk"). What's new here is only the join: which record key names which
 * catalog entry. See docs/vendored-game-data.md for the refresh chore.
 *
 * Every lookup falls back to a readable form of the raw id, and every roster
 * renders keys it doesn't recognise, so a tower or raid boss added by a game
 * update shows up unnamed rather than vanishing.
 */

interface DexEntry {
  name: string;
  elements: string[];
  rarity: number;
}

const dex = palDex as Record<string, DexEntry>;

/** The catalog's own name for a record id.
 *
 * Not palName(): that strips a leading `BOSS_` before looking up, because for
 * pals `BOSS_Anubis` is just an alpha Anubis. The human bounty targets are
 * catalogued *with* the prefix — `boss_hunter_rifle` is Hawk, while
 * `hunter_rifle` is nothing — so the undecorated key gets first refusal.
 */
export function recordName(id: string): string {
  return dex[id.toLowerCase()]?.name ?? palName(id);
}

/**
 * TowerBossDefeatFlag is one map holding a whole progression, and the shape of
 * that progression is the thing worth drawing — so it's modelled here as the
 * four tiers it actually is rather than as one flat list:
 *
 *     the Palpagos Islands towers        eight, in the order you meet them
 *              ↓
 *          Panthalus                     one fight, between the two halves
 *              ↓
 *     the World Tree run                 three, in any order
 *              ↓
 *       Zenara & Astralym                the last fight, gated on all three
 *
 * Every key is read off a real save record.
 */
export type Boss = { key: string; palId?: string; name?: string; region?: string };

/**
 * The Palpagos Islands towers, in the order the game presents them — which is
 * also the order they get harder, so the run reads as a difficulty ladder
 * rather than an arbitrary list.
 *
 * Regions are the save's own words: FindAreaFlagMap carries Tower_Grass,
 * Tower_Forest, Tower_Volcano, Tower_Desert, Tower_Snowy and Tower_Sakurajima,
 * which is also what settles the one genuinely ambiguous pair — the third
 * tower's record key says "Electric" but its area flag and its quest
 * (Main_DefeatVolcanoBoss) both say volcano, so ElectricBoss is Axel & Orserk
 * and DesertBoss is Marcus & Faleris, not the other way round.
 *
 * Deliberately no faction names ("Rayne Syndicate Tower"): the catalog doesn't
 * carry them, the newer three aren't ones this codebase can source, and the duo
 * is what players actually say anyway — "have you done Axel yet".
 */
export const PALPAGOS_TOWERS: Boss[] = [
  { key: "BOSS_BATTLE_NAME_GrassBoss", palId: "gym_elecpanda", region: "Grass" },
  { key: "BOSS_BATTLE_NAME_ForestBoss", palId: "gym_lilyqueen", region: "Forest" },
  { key: "BOSS_BATTLE_NAME_ElectricBoss", palId: "gym_thunderdragonman", region: "Volcano" },
  { key: "BOSS_BATTLE_NAME_DesertBoss", palId: "gym_horus", region: "Desert" },
  { key: "BOSS_BATTLE_NAME_SnowBoss", palId: "gym_blackgriffon", region: "Snowy" },
  { key: "BOSS_BATTLE_NAME_SakurajimaBoss", palId: "gym_moonqueen", region: "Sakurajima" },
  { key: "BOSS_BATTLE_NAME_VikingBoss", palId: "gym_snowtigerbeastman", region: "Feybreak" },
  { key: "BOSS_BATTLE_NAME_SorajimaBoss", palId: "gym_blueskydragon", region: "Sky Island" },
];

/** The fight between the towers and the World Tree. Not a tower: the save
 * gives it a BOSS_KingWhale area flag where every tower gets Tower_<region>. */
export const PANTHALUS: Boss = {
  key: "BOSS_BATTLE_NAME_KingWhaleBoss",
  palId: "kingwhale",
  region: "Ocean",
};

/**
 * The World Tree run — three dungeon bosses in any order, all of which have to
 * fall before the Astralym fight opens.
 *
 * No catalog names these: there is no `worldtreemiddleboss` entry anywhere, and
 * because the three can be taken in any order the save's numbering implies
 * nothing on its own. They were pinned from the saves instead, using two
 * order-preserving facts — a flag map records keys in the order they were first
 * set, and each of these bosses drops a distinctly named item that lands in the
 * next free inventory slot:
 *
 *     Gigi     flags MB1,MB3,MB2   drops Dandilord 44, Lab 45, Silvance 47
 *     Bobachu  flags MB1,MB3,MB2   drops Dandilord 41, Lab 42, Silvance 43
 *     AzarA    flags MB2,MB3,MB1   drops Silvance 36, Lab 37, Dandilord 38
 *     safwyl   flags MB2,MB1,MB3   drops Silvance 53, Dandilord 54, Lab 55
 *
 * Four players, three different orders, one consistent assignment: 1 is
 * Dandilord, 2 is Silvance, 3 is the Laboratory. There are six ways to pair
 * these up and four independent readings agree on one, so this is inference
 * rather than a lookup — but it is not a guess. The drop names are the anchor
 * (`BossDefeatReward_Mothman` is "Silvance's Plume").
 */
export const WORLD_TREE_RUN: Boss[] = [
  { key: "BOSS_BATTLE_NAME_WorldTreeMiddleBoss1", palId: "flowerprince", name: "Dandilord" },
  { key: "BOSS_BATTLE_NAME_WorldTreeMiddleBoss2", palId: "mothman", name: "Silvance" },
  // Not one boss but eight modified tower boss pals over four waves, so no
  // single catalog portrait fits. It draws Grizzbolt blacked out under a red
  // rim — which is not a stand-in: the first wave *is* a Highly Modified
  // Grizzbolt, catalogued by paldb under that name, and the silhouette is how
  // the game itself presents those fights. The horns survive being flattened
  // to one colour, which a rounder pal's outline doesn't; see BossPortrait for
  // the inset that stops the circle clipping it back into a disc.
  { key: "BOSS_BATTLE_NAME_WorldTreeMiddleBoss3", palId: "elecpanda", name: "Forbidden Laboratory" },
];

/** The last fight, gated on the whole World Tree run. */
export const ASTRALYM: Boss = {
  key: "BOSS_BATTLE_NAME_WorldTreeBoss",
  palId: "gym_worldtreedragon",
  region: "World Tree",
};

/** Is this the Forbidden Laboratory, which draws as a silhouette rather than
 * as the pal whose outline it borrows? */
export function isLaboratory(boss: Boss): boolean {
  return boss.key === "BOSS_BATTLE_NAME_WorldTreeMiddleBoss3";
}

/** Every boss battle the page knows, in progression order — for counting a
 * player's progress and for finding the next thing the group can close out. */
export const BOSS_CHAIN: Boss[] = [...PALPAGOS_TOWERS, PANTHALUS, ...WORLD_TREE_RUN, ASTRALYM];

/** What to call a boss: its own name when it has one, else the catalog's name
 * for the pal it's drawn as. */
export function bossLabel(boss: Boss): string {
  return boss.name ?? (boss.palId ? recordName(boss.palId) : boss.key);
}

/** What a fight is like, for the detail dialog. Levels and HP are vendored
 * (see docs/vendored-game-data.md); elements come from palDex.json, which
 * already carries them for every boss form, and the counters are computed off
 * those rather than stored, so they can't drift from the element chart. */
export interface FightStats {
  title: string;
  where?: string;
  /** [level, hp] at each difficulty the game offers. */
  normal?: [number, number];
  hard?: [number, number];
  ultra?: [number, number];
  /** The game renames the hard version of the Astralym fight. */
  hardTitle?: string;
  /** For the Laboratory, which is a level band rather than one fight. */
  levelRange?: [number, number];
  waves?: string[][];
  note?: string;
}

// Double cast because TypeScript infers a JSON `[10, 12900]` as number[], not
// as the pair FightStats declares.
const fights = bossFights as unknown as {
  towers: Record<string, FightStats>;
  raids: Record<string, FightStats>;
};


export function towerFight(key: string): FightStats | undefined {
  return fights.towers[key];
}

export function raidFight(key: string): FightStats | undefined {
  return fights.raids[key];
}

/**
 * The thirteen effigy kinds, in the order the game numbers their items.
 *
 * The save counts effigies by the bonus they feed (`EPalRelicType::JumpPower`)
 * while players know them by the pal on the statue ("Rooby Effigy"), so this
 * is the join between the two. Every pairing is from paldb's effigy list, and
 * the sets line up exactly — thirteen kinds, thirteen enum values, no leftovers
 * on either side, which is the check that the mapping is right.
 *
 * `item` is the catalog id, so the view can draw the effigy's own icon rather
 * than the pal's portrait — a Rooby Effigy is a statue, not a Rooby.
 */
export const EFFIGY_KINDS: { type: string; item: string; pal: string }[] = [
  { type: "CapturePower", item: "Relic", pal: "Lifmunk" },
  { type: "HungerReduction", item: "Relic_01", pal: "Lamball" },
  { type: "SwimSpeed", item: "Relic_02", pal: "Pengullet" },
  { type: "FoodDecayReduction", item: "Relic_03", pal: "Munchill" },
  { type: "JumpPower", item: "Relic_04", pal: "Rooby" },
  { type: "GliderSpeed", item: "Relic_05", pal: "Herbil" },
  { type: "ClimbSpeed", item: "Relic_06", pal: "Tanzee" },
  { type: "StatusAilmentResist", item: "Relic_07", pal: "Depresso" },
  { type: "StaminaReduction", item: "Relic_08", pal: "Cattiva" },
  { type: "SphereHoming", item: "Relic_09", pal: "Lunaris" },
  { type: "ExpBonus", item: "Relic_10", pal: "Relaxaurus" },
  { type: "RainbowPassiveRate", item: "Relic_11", pal: "Yakumo" },
  { type: "MoveSpeed", item: "Relic_12", pal: "Mimog" },
];

/**
 * The effigy's own icon, built from its id rather than looked up.
 *
 * items.json knows the icon name, but importing it here would pull a 532 KB
 * catalog into this route to resolve thirteen strings — and the icon name is
 * simply the id lowercased (`Relic_04` → `relic_04`) for every effigy. If that
 * convention ever breaks, the icon 404s and the chip loses its picture; it
 * can't show the wrong one.
 */
export function effigyIconUrl(item: string): string {
  return `${import.meta.env.BASE_URL}item-icons/${item.toLowerCase()}.webp`;
}

/** Effigies found per kind, biggest first, skipping kinds with none — a row
 * reading "Yakumo 0" is a slower way of saying nothing. A kind the table
 * doesn't know still shows, under its raw enum name, rather than vanishing. */
export function effigyCounts(found: Record<string, number>): { pal: string; item: string; count: number }[] {
  const known = new Set(EFFIGY_KINDS.map((k) => k.type));
  const rows = EFFIGY_KINDS.filter((k) => (found[k.type] ?? 0) > 0).map((k) => ({
    pal: k.pal,
    item: k.item,
    count: found[k.type],
  }));
  for (const [type, count] of Object.entries(found)) {
    if (!known.has(type) && count > 0) rows.push({ pal: type, item: "Relic", count });
  }
  return rows.sort((a, b) => b.count - a.count || a.pal.localeCompare(b.pal));
}

/**
 * The effigy rank a player has put into capture power.
 *
 * This is the only one of the thirteen relic bonuses worth repeating here:
 * the other twelve are movement and utility stats the inventory view already
 * lists as adventure stats, and capture power is the one that view leaves out
 * — which is also the one people mean when they say "effigies".
 */
export function capturePower(ranks: Record<string, number>): number {
  return ranks.CapturePower ?? 0;
}

/** The solo arena ladder, lowest rank first. */
export const ARENA_RANKS = ["Bronze", "Silver", "Gold", "Platinum", "Diamond", "Master"];

/** The highest arena rank this player has cleared, or null for none. */
export function arenaRank(ranks: Record<string, number>): string | null {
  for (let i = ARENA_RANKS.length - 1; i >= 0; i -= 1) {
    if ((ranks[ARENA_RANKS[i]] ?? 0) > 0) return ARENA_RANKS[i];
  }
  return null;
}

/**
 * Summonable raid bosses, easiest first. The record keys are
 * PalSummon_<pal id>, and the catalog's `raid_` entries carry both the name
 * and the raid artwork, so the join is mechanical.
 *
 * Ordered by the vendored normal-mode level rather than by hand, so the list
 * can't drift out of order when one is added — release order and difficulty
 * order are not the same thing, and it was Hartalis (70) sitting above
 * Xenolord (65) that showed it.
 *
 * The Terraria crossover bosses are catalogued too but left off: they're
 * limited-time event content, and a roster that permanently reads "2 never
 * beaten" on every server is measuring the calendar, not the players. A save
 * that does have them still renders them, through the unknown-key path.
 */
export const RAID_ROSTER: { key: string; palId: string }[] = [
  { key: "PalSummon_NightLady", palId: "raid_nightlady" },
  { key: "PalSummon_NightLady_Dark", palId: "raid_nightlady_dark" },
  { key: "PalSummon_KingBahamut_Dragon", palId: "raid_kingbahamut_dragon" },
  { key: "PalSummon_LegendDeer", palId: "raid_legenddeer" },
  { key: "PalSummon_DarkMechaDragon", palId: "raid_darkmechadragon" },
].sort((a, b) => raidLevel(a.key) - raidLevel(b.key));

/** Normal-mode level, for ordering. A boss with no vendored fight sorts last
 * rather than to the front, which is where a 0 would put it. */
function raidLevel(key: string): number {
  return fights.raids[key]?.normal?.[0] ?? Number.MAX_SAFE_INTEGER;
}

/** Portrait for a raid key the roster doesn't list, so a new raid boss still
 * draws something: PalSummon_Foo → the catalog's raid_foo, else plain foo. */
export function raidPalId(key: string): string {
  const id = key.replace(/^PalSummon_/i, "");
  return `raid_${id.toLowerCase()}` in dex ? `raid_${id.toLowerCase()}` : id;
}

/**
 * The human bounty targets, every one the catalog knows. They're the whole
 * roster rather than the observed subset, so the view has an honest
 * denominator: 34 named criminals, of whom this player has taken down N.
 *
 * Derived from the catalog rather than typed out, so the list tracks a
 * refresh of palDex.json instead of drifting from it.
 */
export const BOUNTY_ROSTER: string[] = Object.keys(dex)
  .filter((k) => k.startsWith("boss_"))
  .sort((a, b) => dex[a].name.localeCompare(dex[b].name));

/** Human bounty targets are keyed BOSS_<who>; field alphas carry a region and
 * spawner index instead (81_1_grass_FBOSS_20), which is how the one save map
 * that mixes them gets split. */
function isBountyKey(key: string): boolean {
  return key.toLowerCase().startsWith("boss_") && key.toLowerCase() in dex;
}

/**
 * Splits NormalBossDefeatFlag into the two things it actually holds: named
 * human bounty targets, and field boss spawn points.
 *
 * Both are respawn state — the game clears these flags periodically — so
 * neither is a lifetime total.
 */
export function splitFieldBosses(keys: string[]): { bounties: string[]; fieldBosses: string[] } {
  const bounties: string[] = [];
  const fieldBosses: string[] = [];
  for (const key of keys) {
    if (isBountyKey(key)) bounties.push(key.toLowerCase());
    else fieldBosses.push(key);
  }
  return { bounties, fieldBosses };
}


/** How many of the chain's boss battles this player has cleared. Counts only
 * the known chain, so an unrecognised key can't push the figure past the
 * denominator shown beside it. */
export function bossesCleared(records: PlayerRecords): number {
  const beaten = new Set(records.towers);
  return BOSS_CHAIN.filter((b) => beaten.has(b.key)).length;
}

/** Tower-map keys in a save the chain doesn't account for — a boss battle
 * added by a game update. Rendered under the chain so it shows up unnamed
 * rather than being silently dropped. Finding the World Tree run and Panthalus
 * in here is exactly how they got their tiers. */
export function extraTowerKeys(allTowers: Set<string>): string[] {
  const known = new Set(BOSS_CHAIN.map((b) => b.key));
  return [...allTowers].filter((k) => !known.has(k)).sort();
}

/** Reads a tower's kill count across both difficulties. The count map is keyed
 * <x>_Normal / <x>_Hard where the flag map is keyed BOSS_BATTLE_NAME_<x>. */
export function towerClears(records: PlayerRecords, key: string): { normal: number; hard: number } {
  const stem = key.replace(/^BOSS_BATTLE_NAME_/, "");
  return {
    normal: records.towerCounts[`${stem}_Normal`] ?? 0,
    hard: records.towerCounts[`${stem}_Hard`] ?? 0,
  };
}

/** Main-story quests, which is the progress worth reading — the completed
 * array mixes them with side and hidden quests (Sub_DeliveryWood_Fine,
 * Hidden_ChangeWeaponBulletTutorialTrigger) that nobody tracks. */
export function mainQuests(quests: string[]): string[] {
  return quests.filter((q) => q.startsWith("Main_"));
}

/** "Main_DefeatSnowyMountainBoss" → "Defeat Snowy Mountain Boss". */
export function questLabel(quest: string): string {
  return quest
    .replace(/^(Main|Sub|Hidden)_/, "")
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/_/g, " ")
    .trim();
}
