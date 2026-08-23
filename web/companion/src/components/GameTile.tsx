import { memo } from "react";
import { CoverArt } from "./CoverArt";
import { artFor, gameKey, type Artwork, type DiscoveredGame } from "../lib/types";
import { cn } from "../lib/utils";

/**
 * One entry in the games library.
 *
 * Linked tiles sit in their own section and wear a gold border; unlinked
 * ones are dimmed and un-dim on hover. Two changes from the old shelf,
 * both because the old grid said the same thing twice: the "not linked"
 * caption is gone — the dim treatment already said it, on eleven of
 * thirteen tiles — and the name wraps to two lines instead of
 * truncating, because "RuneScape: Dragon…" is not a name.
 *
 * In its place, each unlinked tile says what linking it would cost:
 * whether a save folder is already known, or whether you will be picking
 * it yourself. That is the thing a player actually wants to know before
 * clicking, and it used to live in a sentence under the whole grid.
 *
 * memo'd, and rendered under a key of the game's identity: the page polls
 * every five seconds, and an <img> that remounts re-fetches. Rebuilding
 * identical tiles on every poll made every cover flicker.
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
        "flex flex-col overflow-hidden rounded-[6px] border text-left transition-[opacity,border-color] duration-150",
        linked
          ? "border-gold/50 bg-panel"
          : "border-edge bg-well opacity-[0.72] hover:border-gold/50 hover:opacity-100",
        active && "border-goldhi opacity-100",
      )}
    >
      <div className="h-[132px] w-full overflow-hidden">
        <CoverArt art={art} game={game} variant="tile" />
      </div>
      <div className="flex items-center gap-2 px-2.5 pb-2.5 pt-2">
        <div className="min-w-0 flex-1">
          {/* Two lines, no ellipsis: a truncated title is not a title. */}
          <div className="line-clamp-2 text-[13px] leading-[1.25] text-parchment">{label}</div>
          <div
            className={cn("mt-[3px] text-[11.5px] leading-tight", linked ? "text-goldhi" : "text-mist")}
          >
            {linked
              ? `1 world · ${worldName ?? "linked"}`
              : game.hidden
                ? "hidden"
                : first
                  ? "save folder known"
                  : "pick the folder yourself"}
          </div>
        </div>
        {/* Clickability stated on the tile, rather than in a sentence
            under the whole grid. */}
        {linked ? null : (
          <span className="whitespace-nowrap text-[11.5px] text-goldhi">Link</span>
        )}
      </div>
    </button>
  );
});

/** The identity a tile is keyed by — one game, one key, everywhere. */
export const tileKey = (g: DiscoveredGame) => g.key || gameKey(g);
