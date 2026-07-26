import mapPois from "../data/mapPois.json";

/** Static map points of interest, vendored from palworld-save-pal (see
 * docs/vendored-game-data.md). Coordinates are world units — the same space
 * players and bases are plotted in. */
export type PoiKind = "fastTravel" | "watchtower" | "dungeon" | "alpha" | "predator";

export const POI_KINDS: PoiKind[] = ["fastTravel", "watchtower", "dungeon", "alpha", "predator"];

export const POI_POINTS = mapPois as Record<PoiKind, [number, number][]>;

/** Display metadata per layer. Colors are existing palette tokens — the
 * legend chips and the pins must agree, so both read from here. */
export const POI_META: Record<PoiKind, { label: string; color: string }> = {
  fastTravel: { label: "Fast travel", color: "#5B9BD5" }, // pal-blue
  watchtower: { label: "Watchtowers", color: "#2B2420" }, // ink
  dungeon: { label: "Dungeons", color: "#8B3A9E" }, // legendary violet
  alpha: { label: "Alpha pals", color: "#E8491D" }, // brand-red
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
