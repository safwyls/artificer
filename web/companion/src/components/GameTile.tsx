import { memo } from "react";
import { CoverArt } from "./CoverArt";
import { artFor, gameKey, type Artwork, type DiscoveredGame } from "../lib/types";
import { cn } from "../lib/utils";

/**
 * One shelf entry. Linked games are in colour with a gold ring and the
 * world's name as their caption; unlinked ones are greyed and un-grey on
 * hover — telling the two apart at a glance is the whole point of the
 * shelf.
 *
 * memo'd, and rendered under a key of the game's identity: the page polls
 * every five seconds, and an <img> that remounts re-fetches. Rebuilding
 * identical tiles on every poll made every cover flicker, which is the
 * bug the old page's shelf signature existed to prevent.
 */
export const GameTile = memo(function GameTile({
  game,
  art,
  worldName,
  linked,
  active,
  onOpen,
}: {
  game: DiscoveredGame;
  art: Record<string, Artwork>;
  worldName: string | undefined;
  linked: boolean;
  active: boolean;
  onOpen: () => void;
}) {
  const found = artFor(art, game);
  const label = found.name || game.name;
  const first = game.saveDirs?.[0]?.path;
  return (
    <button
      type="button"
      onClick={onOpen}
      title={first ? `${label} — ${first}` : label}
      className={cn(
        "flex flex-col overflow-hidden rounded-[6px] border bg-ink text-left transition-[filter,border-color,transform] duration-150 hover:-translate-y-0.5 hover:border-gold",
        linked
          ? "border-gold shadow-[0_0_0_1px_rgba(201,168,96,.25)_inset]"
          : "border-edge grayscale brightness-[.72] hover:grayscale-[.25] hover:brightness-100",
        active && "border-goldhi",
      )}
    >
      <CoverArt art={art} game={game} variant="tile" />
      <div className="px-[7px] pb-2 pt-1.5 text-[12px] leading-tight">
        <b className="block overflow-hidden text-ellipsis whitespace-nowrap">{label}</b>
        <span
          className={cn(
            "text-[10px] uppercase tracking-[0.1em]",
            linked ? "text-gold" : "text-mist",
          )}
        >
          {linked ? (worldName ?? "linked") : game.hidden ? "hidden" : "not linked"}
        </span>
      </div>
    </button>
  );
});

/** The identity a tile is keyed by — one game, one key, everywhere. */
export const tileKey = (g: DiscoveredGame) => g.key || gameKey(g);
