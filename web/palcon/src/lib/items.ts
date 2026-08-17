import itemCatalog from "../data/items.json";

/**
 * Lookups that turn a save file's internal item ids into what the game calls
 * things — "PalSphere_Mega" is a Mega Sphere, "AncientArmor" the Ancient
 * Civilization Armor.
 *
 * Vendored from palworld-save-pal's English localization, trimmed to the
 * fields this viewer renders; see web/public/item-icons/README.md. Every
 * lookup falls back to a humanized id, so an item added by a game update
 * still shows a readable name rather than a raw internal one.
 */

interface ItemEntry {
  /** Localized name. */
  n: string;
  /** Category, the game's own `type_a`: Weapon, Armor, Food, Material… */
  c: string;
  /** 0–4, the game's rarity ladder. 0 is common. */
  r: number;
  /** Kilograms per unit. */
  w: number;
  /** Icon basename under /item-icons. */
  i: string;
  /** Description; absent when the catalog only repeats the name. */
  d?: string;
  /** Max stack size; absent for items that don't stack. */
  s?: number;
  /** Full durability, for gear that wears down. */
  dur?: number;
  /** Magazine size, for guns. */
  mag?: number;
  /** Attack, for weapons. */
  dmg?: number;
  /** Defense, for armor. */
  def?: number;
  /** Passives the item grants by design (a dropped weapon can roll its own,
   * which arrive on the slot instead). */
  ps?: string[];
  /** Equipment slot the game puts this in; absent for anything not worn. */
  g?: "Head" | "Body" | "Accessory" | "Shield" | "Glider" | "SphereModule";
}

const catalog = itemCatalog as Record<string, ItemEntry>;

export function itemEntry(itemId: string): ItemEntry | undefined {
  return catalog[itemId];
}

/** "PalSphere_Mega" → "Mega Sphere"; an unknown id degrades to a spaced-out
 * version of itself rather than disappearing. */
export function itemName(itemId: string): string {
  const name = catalog[itemId]?.n;
  if (name) return name;
  return itemId
    .replace(/_/g, " ")
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .trim();
}

export function itemIconUrl(itemId: string): string {
  // BASE_URL-prefixed so subpath deployments (e.g. the static demo) resolve.
  const icon = catalog[itemId]?.i;
  return icon ? `${import.meta.env.BASE_URL}item-icons/${icon}.webp` : "";
}

/**
 * Rarity as the game ranks it, 0–4. Only 1+ gets a color: if every slot in a
 * bag is outlined, the outline stops meaning anything, and rarity 0 covers
 * ore, wood and berries.
 */
export const RARITY_COLORS: Record<number, string> = {
  1: "#4A9D7C", // pal-green — uncommon
  2: "#5B9BD5", // pal-blue — rare
  3: "#8B3A9E", // legendary purple — epic
  4: "#F2A93B", // brand amber — legendary
};

export function rarityColor(itemId: string): string | undefined {
  return RARITY_COLORS[catalog[itemId]?.r ?? 0];
}

export const RARITY_NAMES: Record<number, string> = {
  0: "Common",
  1: "Uncommon",
  2: "Rare",
  3: "Epic",
  4: "Legendary",
};

/**
 * Categories in the order the filter offers them, coarser than the game's own
 * `type_a`: Glider/SphereModule/SpecialWeapon are one-or-two-item groups that
 * would each cost a filter row for nothing, so they fold into "Gear" and
 * "Spheres" where a player would look for them.
 */
const CATEGORY_ALIASES: Record<string, string> = {
  Weapon: "Weapons",
  SpecialWeapon: "Spheres",
  Ammo: "Ammo",
  Armor: "Gear",
  Accessory: "Gear",
  Glider: "Gear",
  SphereModule: "Spheres",
  Food: "Food",
  Consume: "Consumables",
  Material: "Materials",
  Essential: "Key items",
  Blueprint: "Schematics",
};

export const CATEGORIES = [
  "Weapons",
  "Gear",
  "Spheres",
  "Ammo",
  "Food",
  "Consumables",
  "Materials",
  "Key items",
  "Schematics",
];

export function itemCategory(itemId: string): string {
  const raw = catalog[itemId]?.c;
  return (raw && CATEGORY_ALIASES[raw]) || "Other";
}

/**
 * The sockets a player has exactly one of, in the order the game racks them.
 * Accessories are deliberately not here: how many a player can wear varies
 * with level and technology, so there's no fixed set to draw.
 *
 * The armour container mixes all of them into one list, so `equipSlot` is what
 * splits it back apart.
 */
export const GEAR_SLOTS = ["Head", "Body", "Shield", "Glider", "SphereModule"] as const;

export function equipSlot(itemId: string): string | undefined {
  return catalog[itemId]?.g;
}

/** Total kilograms a stack adds to what the player is hauling. */
export function stackWeight(itemId: string, count: number): number {
  return (catalog[itemId]?.w ?? 0) * count;
}

/**
 * Durability as a fraction of full, or undefined when the item has no
 * durability or the catalog doesn't know its maximum. Clamped: a save written
 * mid-repair can hold a value a hair over the listed max.
 */
export function durabilityFraction(itemId: string, durability: number | undefined): number | undefined {
  const max = catalog[itemId]?.dur;
  if (!max || durability === undefined) return undefined;
  return Math.max(0, Math.min(1, durability / max));
}
