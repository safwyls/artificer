/**
 * Skill level from raw XP, on the game's own curve — calibrated against a
 * real character (2026-08-20): a player's in-game skill panel gave twelve
 * exact (XP → level) pairs across levels 2–40, and exactly one clean
 * formula matches all twelve. It is the classic RuneScape curve with the
 * divisor changed from 4 to 10:
 *
 *   xp(L) = floor( (1/10) · Σ_{l=1}^{L-1} floor(l + 300·2^(l/7)) )
 *
 * (So level 99 costs 5,213,772 XP here, not RuneScape's 13,034,431.)
 * The wiki describes a bend above level 93 (a 94–95 linear bridge into a
 * 96–99 quadratic) that these observations cannot reach; until someone's
 * up there, this formula stands for the whole range and the exact XP
 * always rides on hover. Recalibrate from a new screenshot + save pair if
 * levels ever drift — see games/dragonwilds/docs/vendored-game-data.md.
 */

const MAX_LEVEL = 99;

// xpForLevelTable[L] = XP at which level L begins; index 0 unused.
const xpForLevelTable: number[] = (() => {
  const table = [0, 0];
  let points = 0;
  for (let level = 1; level < MAX_LEVEL; level++) {
    points += Math.floor(level + 300 * Math.pow(2, level / 7));
    table.push(Math.floor(points / 10));
  }
  return table;
})();

/** The level a skill sits at for a given raw XP; clamped to 1–99. */
export function levelForXp(xp: number): number {
  for (let level = MAX_LEVEL; level >= 2; level--) {
    if (xp >= xpForLevelTable[level]) return level;
  }
  return 1;
}

/** XP at which a level begins — exported for tests and progress meters. */
export function xpForLevel(level: number): number {
  return xpForLevelTable[Math.max(1, Math.min(MAX_LEVEL, level))];
}
