import type { PlayerPals } from "./api";
import { friendshipRank } from "./stats";
import type { SavePal } from "../components/PalPicker";

/**
 * Flattens the /pals payload into the calculators' SavePal shape — one
 * entry per pal, deduped across buckets, with talents/souls/stars lifted
 * out of the save's spellings. Shared by the Calculators page and the
 * advisor overlay so both read a pal identically.
 */
export function toSavePals(players: PlayerPals[]): SavePal[] {
  const out: SavePal[] = [];
  const seen = new Set<string>();
  for (const player of players) {
    const buckets: [typeof player.party, string][] = [
      [player.party, "Party"],
      [player.palbox, "Palbox"],
      [player.base, "At base"],
      [player.storage ?? [], "Pal storage"],
    ];
    for (const [list, where] of buckets) {
      for (const pal of list) {
        if (seen.has(pal.instanceId)) continue;
        seen.add(pal.instanceId);
        // Soul upgrades come back keyed by the game's stat labels; pull the
        // three combat ones. Rank is 1-based (1 = no condenser), so stars = rank-1.
        const souls = pal.souls ?? {};
        out.push({
          key: pal.instanceId,
          characterId: pal.characterId,
          nickname: pal.nickname,
          level: pal.level,
          gender: pal.gender,
          ivHp: pal.talentHp,
          ivAttack: pal.talentShot,
          ivDefense: pal.talentDefense,
          condenser: Math.max(0, (pal.rank ?? 1) - 1),
          souls: {
            hp: souls["Max HP"] ?? 0,
            attack: souls["Attack"] ?? 0,
            defense: souls["Defense"] ?? 0,
          },
          passives: pal.passives ?? [],
          isAlpha: pal.isBoss,
          trust: friendshipRank(pal.friendship),
          playerUid: player.uid,
          playerName: player.nickname,
          pal,
          where,
        });
      }
    }
  }
  return out;
}
