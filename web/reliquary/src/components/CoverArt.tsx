import { useState } from "react";
import { artFor } from "../lib/art";
import { cn } from "../lib/utils";
import type { World } from "../lib/types";

/**
 * A world's cover — the same art the companion's shelf shows, so the two
 * views of one world look like one world. With no cover (IGDB unconfigured,
 * a game it doesn't know, a lookup that failed) it falls back to the game's
 * name in a gradient tile: never a broken image, never an error.
 */
export function CoverArt({
  world,
  className,
  size = "card",
}: {
  world: World;
  className?: string;
  size?: "card" | "detail";
}) {
  const [broken, setBroken] = useState(false);
  const art = artFor(world);
  const label = world.gameTitle || world.name || "";
  const box = cn(
    "flex-none rounded-[5px] border border-edge object-cover",
    size === "detail" ? "w-24 h-32" : "w-[84px] h-28",
    className,
  );
  if (!art.cover || broken) {
    return (
      <div
        className={cn(
          box,
          "flex items-center justify-center bg-gradient-to-br from-[#221b2e] to-[#14101d] p-1.5 text-center text-[11px] leading-tight text-mist",
        )}
      >
        {label}
      </div>
    );
  }
  return (
    <img
      className={box}
      src={String(art.cover)}
      alt=""
      loading="lazy"
      onError={() => setBroken(true)}
    />
  );
}
