import { describe, expect, it } from "vitest";
import { levelForXp, xpForLevel } from "./xp";

/**
 * The ground truth: twelve (XP, level) pairs read off a real character's
 * in-game skill panel next to the same character's save file
 * (2026-08-20). Any curve change must keep matching every one — these are
 * observations of the game, not values derived from the code under test.
 */
const OBSERVED: [xp: number, level: number][] = [
  [56, 2],
  [259, 6],
  [273, 7],
  [1_771, 19],
  [3_019, 24],
  [5_729, 30],
  [6_001, 31],
  [10_280, 36],
  [12_107, 37],
  [13_888, 39],
  [15_806, 40],
  [15_814, 40],
];

describe("levelForXp", () => {
  it("matches every level the game itself displayed", () => {
    for (const [xp, level] of OBSERVED) {
      expect(levelForXp(xp), `${xp} xp`).toBe(level);
    }
  });

  it("keeps the calibrated boundaries", () => {
    // Spot boundaries from the fitted formula (classic RS sum ÷ 10),
    // written as constants so a drifted builder fails rather than follows.
    expect(xpForLevel(2)).toBe(33);
    expect(xpForLevel(7)).toBe(260);
    expect(xpForLevel(31)).toBe(5_933);
    expect(xpForLevel(40)).toBe(14_889);
    expect(xpForLevel(99)).toBe(5_213_772);
    // Exactly at a boundary is that level; one below is not.
    expect(levelForXp(5_933)).toBe(31);
    expect(levelForXp(5_932)).toBe(30);
  });

  it("clamps the edges", () => {
    expect(levelForXp(0)).toBe(1);
    expect(levelForXp(32)).toBe(1);
    expect(levelForXp(200_000_000)).toBe(99);
  });
});
