import mapPois from "../data/mapPois.json";
import { FIELD_BOSS_POINTS } from "./fieldBosses";

/** Static map points of interest, vendored from palworld-save-pal (see
 * docs/vendored-game-data.md). Coordinates are world units — the same space
 * players and bases are plotted in. Fast travel statues and watchtowers
 * carry their in-game names; the other layers are anonymous spawns. */
export type PoiKind = "fastTravel" | "watchtower" | "dungeon" | "alpha" | "predator";

export const POI_KINDS: PoiKind[] = ["fastTravel", "watchtower", "dungeon", "alpha", "predator"];

/** [x, y] for anonymous layers; [x, y, name] for the named ones. */
export type PoiPoint = [number, number] | [number, number, string];

export const POI_POINTS = mapPois as unknown as Record<PoiKind, PoiPoint[]>;

/** How many pins a layer draws. Field bosses come from their own table — it
 * names and levels each one, where this file only ever had anonymous
 * coordinates for them — so its count has to come from there too, or the
 * legend and the map disagree. */
export function poiCount(kind: PoiKind): number {
  return kind === "alpha" ? FIELD_BOSS_POINTS.length : POI_POINTS[kind].length;
}

// Statues and watchtowers double as the map's named landmarks — each is
// named for its locality, which makes "near X" a better answer to "where
// is this base" than raw coordinates.
const LANDMARKS: { name: string; x: number; y: number }[] = (["fastTravel", "watchtower"] as const).flatMap((kind) =>
  POI_POINTS[kind].flatMap(([x, y, name]) => (name ? [{ name, x, y }] : [])),
);

/** The closest named landmark to a world position, with the distance in
 * meters (world units are centimeters). */
export function nearestLandmark(x: number, y: number): { name: string; meters: number } | null {
  let best: { name: string; x: number; y: number } | null = null;
  let bestD = Infinity;
  for (const l of LANDMARKS) {
    const d = (l.x - x) ** 2 + (l.y - y) ** 2;
    if (d < bestD) {
      bestD = d;
      best = l;
    }
  }
  return best ? { name: best.name, meters: Math.round(Math.sqrt(bestD) / 100) } : null;
}

/** Display metadata per layer. Colors are existing palette tokens — the
 * legend chips and the pins must agree, so both read from here. */
export const POI_META: Record<PoiKind, { label: string; color: string }> = {
  fastTravel: { label: "Fast travel", color: "#5B9BD5" }, // pal-blue
  watchtower: { label: "Watchtowers", color: "#2B2420" }, // ink
  dungeon: { label: "Dungeons", color: "#8B3A9E" }, // legendary violet
  alpha: { label: "Field bosses", color: "#E8491D" }, // brand-red
  predator: { label: "Predators", color: "#9E2B21" }, // deeper alarm red
};

const STORAGE_KEY = "palcon-map-poi-layers";

/** Fast travel on by default — it's the layer people orient by; the rest
 * stay off so the map doesn't open as marker soup. */
export function loadPoiLayers(): Set<PoiKind> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as string[];
      return new Set(parsed.filter((k): k is PoiKind => POI_KINDS.includes(k as PoiKind)));
    }
  } catch {
    // fall through to the default
  }
  return new Set<PoiKind>(["fastTravel"]);
}

export function savePoiLayers(layers: Set<PoiKind>) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify([...layers]));
  } catch {
    // storage full/blocked — the toggle still works for the session
  }
}
