import { describe, expect, it } from "vitest";
import { levelForXp, xpForLevel } from "./xp";

/**
 * The expected values are the published classic-curve table, written here
 * as independent constants rather than derived from the code under test —
 * if the builder drifts, these fail rather than follow it.
 */
const KNOWN: [level: number, xp: number][] = [
  [2, 83],
  [3, 174],
  [5, 388],
  [10, 1_154],
  [30, 13_363],
  [50, 101_333],
  [70, 737_627],
  [92, 6_517_253],
  [99, 13_034_431],
];

describe("levelForXp", () => {
  it("matches the published table at its boundaries", () => {
    for (const [level, xp] of KNOWN) {
      expect(xpForLevel(level), `xp for level ${level}`).toBe(xp);
      expect(levelForXp(xp), `level at exactly ${xp} xp`).toBe(level);
      expect(levelForXp(xp - 1), `level just under ${xp} xp`).toBe(level - 1);
    }
  });

  it("clamps the edges", () => {
    expect(levelForXp(0)).toBe(1);
    expect(levelForXp(82)).toBe(1);
    expect(levelForXp(200_000_000)).toBe(99);
  });
});
