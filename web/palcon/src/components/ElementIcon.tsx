import type { ComponentType } from "react";
import type { LucideProps } from "lucide-react";
import { Circle, Droplet, Flame, Leaf, Moon, Mountain, Snowflake, Zap } from "lucide-react";
import { elementColor } from "../lib/elements";
import { cn } from "../lib/utils";

/**
 * The nine Palworld elements as glyphs rather than as bare coloured dots.
 *
 * A dot can only say "this element is orange", which means nothing until you
 * already know the palette — and the two smallest element treatments in the
 * app were exactly that, a 6px circle carrying the whole meaning. A glyph
 * still takes the element's colour, so anyone who *has* learned the palette
 * loses nothing, but the shape carries it for everyone else.
 *
 * Drawn from lucide, which is the icon language the rest of palcon already
 * speaks, rather than vendored from the game: these need to sit inline with
 * text at 14px, take a colour, and stay crisp at any zoom — none of which a
 * lifted PNG does well, and it avoids redistributing more of Pocketpair's art
 * than the pal portraits already do.
 */

/**
 * Dragon is the one element lucide has no glyph for, so it's drawn here to
 * match: same 24 viewBox, same round-capped stroke.
 *
 * A claw, after a horn was tried and thrown away — a horn is a pointed
 * teardrop, which is also what a leaf is, and side by side at 14px the two
 * elements were telling apart by colour alone. That is the exact failure the
 * glyphs exist to fix. Three raking talons share their silhouette with
 * nothing else in the set.
 */
function DragonClaw(props: LucideProps) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <path d="M3.5 4.5c4.2 1.4 8 4.8 11 10.2" />
      <path d="M8.5 2.8c3.4 2.1 6.3 5.5 8.4 10.1" />
      <path d="M14 2.6c2.4 2.2 4.3 5 5.6 8.4" />
    </svg>
  );
}

// ComponentType, not a plain function type: lucide exports forwardRef
// components, which a bare (props) => JSX.Element signature rejects.
const GLYPHS: Record<string, ComponentType<LucideProps>> = {
  Normal: Circle,
  Fire: Flame,
  Water: Droplet,
  Leaf: Leaf,
  Electricity: Zap,
  Ice: Snowflake,
  Earth: Mountain,
  Dark: Moon,
  Dragon: DragonClaw,
};

export function ElementIcon({ element, className }: { element: string; className?: string }) {
  // An element the catalog grows later still draws something rather than
  // leaving a gap, in the same neutral grey elementColor falls back to.
  const Glyph = GLYPHS[element] ?? Circle;
  return (
    <Glyph
      aria-hidden
      className={cn("h-4 w-4 shrink-0", className)}
      style={{ color: elementColor(element) }}
      strokeWidth={2.25}
    />
  );
}

/** Icon plus name, the shape most callers want. */
export function ElementTag({ element, className }: { element: string; className?: string }) {
  return (
    <span className={cn("inline-flex items-center gap-1", className)}>
      <ElementIcon element={element} />
      {element}
    </span>
  );
}
