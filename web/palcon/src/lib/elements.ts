/**
 * The nine Palworld elements: their colours, and what beats what.
 *
 * Split out of `paldex.ts` because none of this needs the pal catalog, and
 * `paldex.ts` pulls 212 KB of it. The live map draws field bosses with their
 * elements, and the map is an eagerly-loaded route — importing the catalog for
 * a nine-entry lookup table would put that weight on every first paint.
 */
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

/** What beats what. Palworld's element chart is a plain cycle plus two
 * strays — Ice loses to Fire rather than to its own predecessor, and Normal
 * only loses to Dark — so a boss's counters are derivable from its elements
 * and don't need vendoring per fight. Keyed by the *defender*. */
const BEATEN_BY: Record<string, string> = {
  Normal: "Dark",
  Fire: "Water",
  Water: "Electricity",
  Electricity: "Earth",
  Leaf: "Fire",
  Earth: "Leaf",
  Ice: "Fire",
  Dragon: "Ice",
  Dark: "Dragon",
};

/** The elements that hit these ones hardest, deduplicated and in the order
 * given. Empty for a typeless pal, which is the honest answer: nothing
 * counters Astralym. */
export function elementCounters(elements: string[]): string[] {
  const out: string[] = [];
  for (const el of elements) {
    const beats = BEATEN_BY[el];
    if (beats && !out.includes(beats)) out.push(beats);
  }
  return out;
}
