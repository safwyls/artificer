import partnerSkills from "../data/partnerSkills.json";
import palWork from "../data/palWork.json";
import { palKey, palName } from "./paldex";

/**
 * Partner-skill and work-suitability catalogs, vendored by
 * data/vendor_theorycraft.py (see docs/vendored-game-data.md).
 *
 * The partner-skill tags are parsed from the game's English skill
 * descriptions at vendor time, so the frontend ships data rather than
 * regexes. Magnitudes are deliberately absent everywhere: they scale with a
 * skill's rank, which a static catalog can't know — the tools reason from
 * what a skill *does*, never from how big it currently is.
 */

export type MountKind = "ground" | "fly" | "water" | "glider";

export interface PartnerSkill {
  /** Skill name as the game shows it — "Pacapaca Wool". */
  n: string;
  /** Description with markup stripped and rank-scaled numbers elided. */
  d: string;
  /** Mount kind, when the pal can be ridden or worn. */
  m?: MountKind;
  /** Element the player's own attacks become while it's active. */
  ac?: string;
  /** [element, kind]: a party buff scoped to that element's pals — their
   * damage ("attack"), their loot ("drops"), or something else ("other"). */
  eb?: [string, "attack" | "drops" | "other"];
  /** [species key, stat]: a buff for one specific species — the game has
   * exactly two of these (Melpaca → Kingpaca, Beegarde → Elizabee). */
  pb?: [string, string];
  /** Produces something when assigned to a Ranch. */
  ranch?: 1;
  /** Raises a work suitability for every other pal at the base. */
  base?: 1;
}

/** Work levels (nonzero only), appetite, and the movement figures. */
export interface WorkProfile {
  w: Record<string, number>;
  /** Food amount — how fast it empties the feed box. */
  f: number;
  /** Nocturnal: works through the night. */
  n?: 1;
  /** Ride sprint speed — the mount ranking figure. */
  r?: number;
  /** Transport speed. */
  t?: number;
}

// Via unknown: JSON imports infer string[] where the catalog means tuples.
const partner = partnerSkills as unknown as Record<string, PartnerSkill>;
const work = palWork as Record<string, WorkProfile>;

/** The species' partner skill; capture decorations fold like everywhere else,
 * so a BOSS_ Melpaca reads the same as its species. Undefined for pals the
 * catalog doesn't know — the graceful-drift rule, same as names and icons. */
export function partnerSkill(characterId: string): PartnerSkill | undefined {
  return partner[palKey(characterId)];
}

export function workProfile(characterId: string): WorkProfile | undefined {
  return work[palKey(characterId)];
}

/** One chip's worth of what a partner skill does. `element` tints it the
 * element it talks about; `bond` marks the two species pair-bonds, which
 * tint amber like every other special-pairing cue in the app. */
export interface PartnerTag {
  label: string;
  element?: string;
  bond?: boolean;
}

export const MOUNT_LABELS: Record<MountKind, string> = {
  fly: "flying mount",
  ground: "mount",
  water: "surf mount",
  glider: "glider",
};

/** The skill's effects as chips, in the same wording the team builder's
 * reasons use — one vocabulary for what a skill does, wherever it appears. */
export function partnerTags(skill: PartnerSkill): PartnerTag[] {
  const tags: PartnerTag[] = [];
  if (skill.m) tags.push({ label: MOUNT_LABELS[skill.m] });
  if (skill.ac) tags.push({ label: `arms you with ${skill.ac}`, element: skill.ac });
  if (skill.eb) {
    const [el, kind] = skill.eb;
    tags.push({
      label: kind === "attack" ? `arms ${el} pals` : kind === "drops" ? `${el} pals drop more` : `boosts ${el} pals`,
      element: el,
    });
  }
  if (skill.pb) tags.push({ label: `boosts ${palName(skill.pb[0])}`, bond: true });
  if (skill.ranch) tags.push({ label: "ranch drops" });
  if (skill.base) tags.push({ label: "base-wide work boost" });
  return tags;
}
