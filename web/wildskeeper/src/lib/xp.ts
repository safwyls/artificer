/**
 * Skill level from raw XP, on the classic RuneScape curve (levels 1–99):
 *
 *   xp(L) = floor( (1/4) · Σ_{l=1}^{L-1} floor(l + 300·2^(l/7)) )
 *
 * Dragonwilds' own curve is the same shape where it matters — exponential
 * up to 93, and 99-capped like every RuneScape before it — but the wiki
 * documents a game-specific tail (a 94–95 linear bridge into a 96–99
 * quadratic), so levels shown in that band may sit one off the game's
 * own. The console rides the family curve on the maintainer's call
 * (2026-08-19) and always shows the exact XP on hover, so the derived
 * number never hides the true one. Swap in the game's own table if it
 * ever gets vendored — see games/dragonwilds/docs/vendored-game-data.md.
 */

const MAX_LEVEL = 99;

// xpForLevelTable[L] = XP at which level L begins; index 0 unused.
const xpForLevelTable: number[] = (() => {
  const table = [0, 0];
  let points = 0;
  for (let level = 1; level < MAX_LEVEL; level++) {
    points += Math.floor(level + 300 * Math.pow(2, level / 7));
    table.push(Math.floor(points / 4));
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
