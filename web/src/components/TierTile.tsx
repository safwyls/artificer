import type { CSSProperties, ReactNode } from "react";
import { cn } from "../lib/utils";

/**
 * The hero tile both the Paldex and the Automation page wear: the anatomy of
 * the game's own passive-skill tier tiles — a thick tier-colored left edge,
 * a dark ground with a diagonal facet sliver, a thin tinted outline, and the
 * notched corner from the mocks.
 *
 * The four looks are hand-sampled from the game's tier icons (see the
 * `tier.*` tokens in tailwind.config.js): Rainbow aqua, gold, tier-1 ice,
 * negative red. They're modelled as data rather than CSS strings so a caller
 * can sit *between* two tiers — the restart tile ramps continuously from
 * aqua down to red as the restart nears, and lands exactly on the Paldex
 * tile's look at each anchor.
 */
export interface TierLook {
  /** 4px left edge, and whatever instrument the tile's footer draws. */
  accent: string;
  /** Hairline outline around the tile. */
  border: [r: number, g: number, b: number, a: number];
  /** Display value and eyebrow (the eyebrow and sub at reduced alpha). */
  text: string;
  /** Three stops of the ground gradient: 0%, mid, 100%. */
  ground: [string, string, string];
  /** Where the mid stop sits, in percent. */
  groundMid: number;
  /** The diagonal facet sliver: color, alpha at its leading edge, and the
   * two stop positions. Alpha 0 = no facet (the ice and red tiles). */
  facet: [r: number, g: number, b: number, a: number];
  facetStops: [number, number];
  /** Alpha of the outer glow; 0 = none. Only the Rainbow tier glows. */
  glow: number;
}

const AQUA: TierLook = {
  accent: "#7AFFF2",
  border: [122, 255, 242, 0.55],
  text: "#7AFFF2",
  // Opaque ground — transparent stops would show the page's paper through.
  ground: ["#0f3b41", "#14545c", "#0f3b41"],
  groundMid: 55,
  facet: [122, 255, 242, 0.16],
  facetStops: [44, 58],
  glow: 0.18,
};

// A step more saturated than the vendored tier.gold (#FFE083), which reads
// cream at this size; the reference tile's text is a richer yellow-gold.
const GOLD: TierLook = {
  accent: "#FFD34D",
  border: [255, 211, 77, 0.5],
  text: "#FFD34D",
  ground: ["#211f13", "#262214", "#211f13"],
  groundMid: 60,
  facet: [255, 211, 77, 0.12],
  facetStops: [48, 62],
  glow: 0,
};

const ICE: TierLook = {
  accent: "#E9F8FA",
  border: [233, 248, 250, 0.35],
  text: "#F5EDE1", // paper — ice-white numerals would vanish into the outline
  ground: ["#182220", "#1B2725", "#20302d"],
  groundMid: 55,
  facet: [233, 248, 250, 0],
  facetStops: [48, 62],
  glow: 0,
};

const RED: TierLook = {
  accent: "#FF4649",
  border: [255, 70, 73, 0.45],
  text: "#F5EDE1",
  ground: ["#201a1c", "#241d1f", "#2a2124"],
  groundMid: 60,
  facet: [255, 70, 73, 0],
  facetStops: [48, 62],
  glow: 0,
};

export const TIER_LOOKS = { aqua: AQUA, gold: GOLD, ice: ICE, red: RED };

function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t;
}

function hexToRgb(hex: string): [number, number, number] {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

function lerpHex(a: string, b: string, t: number): string {
  const [ar, ag, ab] = hexToRgb(a);
  const [br, bg, bb] = hexToRgb(b);
  const to = (x: number) => Math.round(x).toString(16).padStart(2, "0");
  return `#${to(lerp(ar, br, t))}${to(lerp(ag, bg, t))}${to(lerp(ab, bb, t))}`;
}

function rgba([r, g, b, a]: [number, number, number, number], scale = 1): string {
  return `rgba(${Math.round(r)},${Math.round(g)},${Math.round(b)},${a * scale})`;
}

/** Blend two tier looks. t=0 is `a`, t=1 is `b`; every channel moves
 * together so the tile reads as one material warming up, not as four
 * properties animating independently. */
export function blendLooks(a: TierLook, b: TierLook, t: number): TierLook {
  const c = Math.max(0, Math.min(1, t));
  const four = (
    x: [number, number, number, number],
    y: [number, number, number, number],
  ): [number, number, number, number] => [
    lerp(x[0], y[0], c),
    lerp(x[1], y[1], c),
    lerp(x[2], y[2], c),
    lerp(x[3], y[3], c),
  ];
  return {
    accent: lerpHex(a.accent, b.accent, c),
    border: four(a.border, b.border),
    text: lerpHex(a.text, b.text, c),
    ground: [
      lerpHex(a.ground[0], b.ground[0], c),
      lerpHex(a.ground[1], b.ground[1], c),
      lerpHex(a.ground[2], b.ground[2], c),
    ],
    groundMid: lerp(a.groundMid, b.groundMid, c),
    facet: four(a.facet, b.facet),
    facetStops: [lerp(a.facetStops[0], b.facetStops[0], c), lerp(a.facetStops[1], b.facetStops[1], c)],
    glow: lerp(a.glow, b.glow, c),
  };
}

/** Holds each anchor for the outer third of its band and crosses in the
 * middle third, smoothly. Without it a ramp spends most of its life on a
 * blend of two tiers — the aqua→gold midpoint is a lime that belongs to no
 * tier at all — instead of reading as the tier it's nearest. */
function plateau(t: number): number {
  const u = Math.max(0, Math.min(1, (t - 0.33) / 0.34));
  return u * u * (3 - 2 * u);
}

/** Walks a ramp of (stop, look) pairs and blends between the two the value
 * falls between. Stops must be ascending. */
export function rampLook(ramp: [number, TierLook][], value: number): TierLook {
  if (value <= ramp[0][0]) return ramp[0][1];
  for (let i = 1; i < ramp.length; i++) {
    const [prev, prevLook] = ramp[i - 1];
    const [next, nextLook] = ramp[i];
    if (value <= next) return blendLooks(prevLook, nextLook, plateau((value - prev) / (next - prev)));
  }
  return ramp[ramp.length - 1][1];
}

/** The tile's own colors as inline styles — the ground, facet, edge and
 * outline. Text colors ride along via `look.text`. */
export function tileStyle(look: TierLook): CSSProperties {
  const [f0, f1] = look.facetStops;
  return {
    background: [
      `linear-gradient(115deg, transparent ${f0}%, ${rgba(look.facet)} ${f0}%, ${rgba(look.facet, 0.3)} ${f1}%, transparent ${f1}%)`,
      `linear-gradient(120deg, ${look.ground[0]} 0%, ${look.ground[1]} ${look.groundMid}%, ${look.ground[2]} 100%)`,
    ].join(", "),
    borderColor: rgba(look.border),
    borderLeft: `4px solid ${look.accent}`,
    boxShadow: look.glow > 0 ? `0 0 18px rgba(122,255,242,${look.glow})` : undefined,
  };
}

/** `look.text` at an alpha, for the eyebrow and the sub-line. */
export function tierText(look: TierLook, alpha: number): string {
  const [r, g, b] = hexToRgb(look.text);
  return `rgba(${r},${g},${b},${alpha})`;
}

/**
 * One hero tile: eyebrow, a display value with a mono sub beside it, and a
 * footer instrument (the Paldex draws a progress bar there, Automation the
 * warning-broadcast rail).
 */
export function TierTile({
  look,
  eyebrow,
  value,
  sub,
  valueClass = "text-4xl lg:text-5xl",
  footer,
  className,
}: {
  look: TierLook;
  eyebrow: string;
  value: ReactNode;
  sub?: ReactNode;
  /** Type scale for the value — long strings ("Saturday 04:00") step down. */
  valueClass?: string;
  footer?: ReactNode;
  className?: string;
}) {
  return (
    <section
      className={cn("rounded-br-[10px] rounded-tl-[10px] border px-6 py-5 lg:px-8", className)}
      // The ramp only moves on a countdown tick, so a slow transition keeps
      // the step invisible; reduced-motion users lose nothing but the fade.
      style={{ ...tileStyle(look), transition: "background 1.2s linear, border-color 1.2s linear" }}
    >
      <p
        className="text-xs font-bold uppercase tracking-widest"
        style={{ color: tierText(look, 0.7), transition: "color 1.2s linear" }}
      >
        {eyebrow}
      </p>
      <div className="mt-1 flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span
          className={cn("font-display font-extrabold", valueClass)}
          style={{ color: look.text, transition: "color 1.2s linear" }}
        >
          {value}
        </span>
        {sub && (
          <span
            className="font-mono text-sm"
            style={{ color: tierText(look, 0.7), transition: "color 1.2s linear" }}
          >
            {sub}
          </span>
        )}
      </div>
      {footer}
    </section>
  );
}
