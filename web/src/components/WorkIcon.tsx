import {
  Axe,
  BatteryCharging,
  Droplets,
  Fence,
  FlameKindling,
  Hand,
  Package,
  Pickaxe,
  Pill,
  Snowflake,
  Sprout,
  Wheat,
  type LucideIcon,
} from "lucide-react";

/**
 * Lucide glyphs for the 12 work suitabilities — the same icon language the
 * rest of palcon speaks (see ElementIcon for the element set). Nothing of
 * the game's art is redistributed for these; they're neutral stand-ins that
 * tint and scale with the text they sit beside.
 */
export const WORK_ICONS: Record<string, LucideIcon> = {
  EmitFlame: FlameKindling,
  Watering: Droplets,
  Seeding: Sprout,
  GenerateElectricity: BatteryCharging,
  Handcraft: Hand,
  Collection: Wheat,
  Deforest: Axe,
  Mining: Pickaxe,
  ProductMedicine: Pill,
  Cool: Snowflake,
  Transport: Package,
  MonsterFarm: Fence,
};

/** Each type's colour, in the material's own hue — flame orange, wood
 * brown, ore grey — kept muted enough to sit on paper and white cards
 * beside ink text. Like ELEMENT_COLORS, plain data so chips or labels can
 * borrow the tint. */
export const WORK_COLORS: Record<string, string> = {
  EmitFlame: "#E8761D", // orange
  Watering: "#5B9BD5", // light blue (the Water element token)
  Seeding: "#8BC97C", // light green
  GenerateElectricity: "#EDBB2A", // yellow
  Handcraft: "#D9A87E", // light flesh tone
  Collection: "#4A9D7C", // green (the Leaf element token)
  Deforest: "#8B5A33", // wood brown
  Mining: "#8A8578", // ore grey
  ProductMedicine: "#7F8C3F", // olive green
  Cool: "#7FC8E8", // ice blue (the Ice element token)
  Transport: "#B08968", // light brown
  MonsterFarm: "#C4A484", // a shade lighter again
};

/** A work type the catalog grows later still draws something, in the same
 * neutral grey the colour map falls back to — the graceful-drift rule that
 * elements and names follow. */
export function WorkIcon({ type, className }: { type: string; className?: string }) {
  const Glyph = WORK_ICONS[type] ?? Hand;
  return <Glyph aria-hidden className={className} style={{ color: WORK_COLORS[type] ?? "#8A8578" }} />;
}
