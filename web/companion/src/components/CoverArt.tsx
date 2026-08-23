import { useState } from "react";
import { artFor, type Artwork } from "../lib/types";
import { cn } from "../lib/utils";

/**
 * Every cover the service answers with is IGDB's `t_cover_big`, which is
 * 264x374 (core/igdb/igdb.go). The frame is cut to that ratio so a
 * poster is shown whole; the shelf used to draw a 132px-tall letterbox
 * and crop the middle out of every one of them.
 *
 * Declared once, and both variants wear it: a tile and a row thumbnail
 * that disagree about the shape of a cover is the drift this component
 * exists to prevent.
 */
export const COVER_ASPECT = "aspect-[264/374]";

/**
 * The one place a cover is drawn, so the shelf tile and the world row
 * cannot drift apart. A broken image falls back to the game's name rather
 * than the browser's torn-page icon.
 *
 * Callers must key this by the game's identity and never remount it on a
 * poll: an <img> that remounts re-fetches, and a shelf of covers blinking
 * every five seconds was the bug the old page's memoization existed to
 * stop.
 */
export function CoverArt({
  art,
  game,
  variant,
}: {
  art: Record<string, Artwork>;
  game: { appId?: string; name?: string };
  variant: "tile" | "thumb";
}) {
  const [broken, setBroken] = useState(false);
  const found = artFor(art, game);
  const label = found.name || game.name || "";
  const shape = cn(
    COVER_ASPECT,
    variant === "tile"
      ? // The grid column sets the tile's width; the ratio sets its height.
        "w-full"
      : "w-[54px] flex-none rounded-[5px] border border-edge",
  );
  if (!found.cover || broken) {
    return (
      <div
        className={cn(
          shape,
          "flex items-center justify-center bg-gradient-to-br from-[#221b2e] to-[#14101d] p-1.5 text-center leading-tight text-mist",
          variant === "tile" ? "text-[11px]" : "text-[9px]",
        )}
      >
        {label}
      </div>
    );
  }
  return (
    <img
      className={cn(shape, "block object-cover")}
      src={found.cover}
      alt=""
      loading="lazy"
      onError={() => setBroken(true)}
    />
  );
}
