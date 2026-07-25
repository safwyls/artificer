import { palEntry, palIconUrl, rarityTier } from "../lib/paldex";
import { cn } from "../lib/utils";

const SIZES = {
  sm: { box: "h-11 w-11 rounded-lg", img: "h-9 w-9" },
  md: { box: "h-14 w-14 rounded-xl", img: "h-11 w-11" },
  lg: { box: "h-24 w-24 rounded-2xl", img: "h-20 w-20" },
};

/** A pal's icon in the app's rarity-tinted frame — blue for rare, gold for
 * legendary, neutral otherwise. Shared by the pal viewer and the calculators
 * so a pal looks the same everywhere. */
export function PalPortrait({
  characterId,
  size = "md",
  className,
}: {
  characterId: string;
  size?: keyof typeof SIZES;
  className?: string;
}) {
  const tier = rarityTier(palEntry(characterId)?.rarity ?? 0);
  const s = SIZES[size];
  return (
    <div
      className={cn(
        "flex shrink-0 items-center justify-center border",
        s.box,
        tier === "legendary"
          ? "border-legendary/40 bg-legendary/10"
          : tier === "rare"
            ? "border-pal-blue/40 bg-pal-blue/10"
            : "border-ink/10 bg-ink/5",
        className,
      )}
    >
      <img
        src={palIconUrl(characterId)}
        alt=""
        className={cn("object-contain", s.img)}
        loading="lazy"
        onError={(e) => {
          e.currentTarget.style.visibility = "hidden";
        }}
      />
    </div>
  );
}
