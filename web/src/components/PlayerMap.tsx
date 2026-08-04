import { useEffect, useRef, useState, type ComponentType, type ReactNode } from "react";
import {
  TransformWrapper,
  TransformComponent,
  useControls,
  useTransformContext,
  useTransformEffect,
  useTransformInit,
} from "react-zoom-pan-pinch";
import { DoorOpen, Eye, Ghost, Home, MapPin, Maximize, Skull, ZoomIn, ZoomOut } from "lucide-react";
import type { LucideProps } from "lucide-react";
import type { Player } from "../lib/api";
import { MAP_AREAS, mapOf, worldToMapPercent, type MapArea } from "../lib/map";
import { POI_META, POI_POINTS, type PoiKind } from "../lib/pois";
import { BOUNTY_POINTS, FIELD_BOSS_POINTS, type MapBossPin, fieldBossIconUrl } from "../lib/fieldBosses";
import { FieldBossDialog } from "./FieldBossDialog";
import { playerColor } from "../lib/palette";
import { cn } from "../lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";
import { Button } from "./ui/button";

function markerId(playerId: string) {
  return `player-marker-${playerId}`;
}

/**
 * A map pin that stays a constant pixel size regardless of zoom — the
 * Leaflet/Mapbox pin convention.
 *
 * This exists instead of the library's own KeepScale because KeepScale only
 * applies its counter-scale inside an onChange subscription, which fires on
 * transform *changes* and never on mount. A pin that mounts while the map is
 * already zoomed — exactly what save-derived markers do, arriving seconds
 * after the map has settled or been zoomed — therefore renders at full
 * content-space size (huge) until the next interaction nudges the transform.
 * Reading the current scale at render time fixes the initial size; the
 * effect keeps it right as the map zooms.
 *
 * transformOrigin "0 0" matters: the scale is a bare scale() whose default
 * origin is the element centre, which fights the left/top anchoring and
 * drifts the pin with zoom. Anchoring the origin to the same corner removes
 * the drift (invisible at 1:1, since scale(1) ignores origin).
 */
function ScaledPin({
  id,
  left,
  top,
  children,
}: {
  id?: string;
  left: number;
  top: number;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const context = useTransformContext();

  useTransformEffect(({ state }) => {
    if (ref.current) ref.current.style.transform = `scale(${1 / state.scale})`;
  });

  return (
    <div
      ref={ref}
      id={id}
      className="absolute flex"
      style={{
        left: `${Math.min(100, Math.max(0, left))}%`,
        top: `${Math.min(100, Math.max(0, top))}%`,
        transform: `scale(${1 / (context.state.scale || 1)})`,
        transformOrigin: "0 0",
      }}
    >
      {children}
    </div>
  );
}

/** DOM id for a save-derived marker, so it can be zoomed to like a player. */
export function mapMarkerId(id: string) {
  return `map-marker-${id}`;
}

/**
 * Per-kind pin glyph. Clip-path silhouettes came first — a triangle for a
 * tower, a diamond for a dungeon — but a 9px shape in the layer's colour still
 * had to compete with snow, lava and jungle underneath it, and lost. A glyph
 * on a paper chip wins that fight twice over: the chip is a constant light
 * ground whatever the terrain does, and the shape says what the thing is
 * rather than requiring the legend to be memorised.
 *
 * The chip is light and the glyph carries the layer's colour, not the other
 * way round. Reversed, the watchtower ink (#2B2420) would vanish into its own
 * pin. Field bosses keep a dark chip because a portrait needs one.
 */
export const POI_ICONS: Record<PoiKind, ComponentType<LucideProps>> = {
  fastTravel: MapPin,
  // An eye, not a tower: TowerControl and Castle both collapse into noise at
  // this size — checked by rendering the candidates at 15/18/20px rather than
  // guessing. A lookout is what a watchtower is for, and an eye survives.
  watchtower: Eye,
  dungeon: DoorOpen,
  alpha: MapPin, // unused — field bosses draw their own portrait
  // A hooded figure is what the game puts on these; Ghost is the closest
  // silhouette lucide has that still reads at pin size.
  bounty: Ghost,
  predator: Skull,
};

/**
 * Static POI pins for the visible layers. Hundreds of markers, so unlike
 * ScaledPin (one transform subscription per pin) the counter-scale is a
 * single CSS variable on the layer container that every pin inherits.
 * The layer itself is pointer-events-none so terrain annotation never
 * blocks the map; only the NAMED pins (statues, watchtowers) opt back in,
 * with a native title on hover and a tap reporting the name to the HUD.
 */
function PoiLayer({
  area,
  layers,
  onPoiSelect,
  onFieldBossSelect,
}: {
  area: MapArea;
  layers: Set<PoiKind>;
  onPoiSelect?: (name: string, x: number, y: number) => void;
  onFieldBossSelect?: (boss: MapBossPin) => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const context = useTransformContext();

  useTransformEffect(({ state }) => {
    ref.current?.style.setProperty("--poi-inv", String(1 / state.scale));
  });

  return (
    <div
      ref={ref}
      className="pointer-events-none absolute inset-0"
      style={{ "--poi-inv": String(1 / (context.state.scale || 1)) } as React.CSSProperties}
    >
      {[...layers].map((kind) =>
        (kind === "alpha" || kind === "bounty" ? [] : POI_POINTS[kind])
          .filter(([x, y]) => mapOf(x, y) === area)
          .map(([x, y, name], i) => {
            const { xPct, yPct } = worldToMapPercent(x, y, area);
            const Glyph = POI_ICONS[kind];
            const style: React.CSSProperties = {
              left: `${xPct}%`,
              top: `${yPct}%`,
              width: 18,
              height: 18,
              transform: "translate(-50%,-50%) scale(var(--poi-inv))",
            };
            const chip = (
              <Glyph
                className="h-full w-full p-[2.5px]"
                style={{ color: POI_META[kind].color }}
                strokeWidth={2.25}
                aria-hidden
              />
            );
            const chipClass =
              "absolute rounded-full border border-ink/25 bg-paper shadow-[0_1px_2px_rgba(0,0,0,0.35)]";
            if (!name) {
              return (
                <span key={`${kind}-${i}`} className={chipClass} style={style}>
                  {chip}
                </span>
              );
            }
            return (
              <button
                key={`${kind}-${i}`}
                type="button"
                className={cn(
                  chipClass,
                  "pointer-events-auto cursor-pointer after:absolute after:-inset-1.5 after:content-['']",
                  "focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-brand-amber",
                )}
                style={style}
                title={name}
                onClick={() => onPoiSelect?.(name, x, y)}
              >
                {chip}
              </button>
            );
          }),
      )}

      {layers.has("bounty") &&
        BOUNTY_POINTS.filter(({ x, y }) => mapOf(x, y) === area).map((boss, i) => {
          const { xPct, yPct } = worldToMapPercent(boss.x, boss.y, area);
          return (
            <button
              key={`bounty-${i}`}
              type="button"
              title={`${boss.name} · Lv ${boss.level}`}
              aria-label={`Bounty target: ${boss.name}`}
              onClick={() => onFieldBossSelect?.(boss)}
              className="pointer-events-auto absolute cursor-pointer rounded-full border border-ink/25 bg-paper shadow-[0_1px_2px_rgba(0,0,0,0.35)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-brand-amber"
              style={{
                left: `${xPct}%`,
                top: `${yPct}%`,
                width: 18,
                height: 18,
                transform: "translate(-50%,-50%) scale(var(--poi-inv))",
              }}
            >
              <Ghost
                className="h-full w-full p-[2.5px]"
                style={{ color: POI_META.bounty.color }}
                strokeWidth={2.25}
                aria-hidden
              />
            </button>
          );
        })}

      {/* Field bosses draw as the pal that stands there rather than as a
          shape, because for this one layer we know: every spawn point has
          exactly one occupant, and the vendored table names all of them. A
          portrait answers "what is that" without a hover, which no amount of
          silhouette-and-colour can. They're bigger than the 9px pins for the
          same reason — a 9px face is a smudge — and there are only ninety. */}
      {layers.has("alpha") &&
        FIELD_BOSS_POINTS.filter(({ x, y }) => mapOf(x, y) === area).map((boss) => {
          const { xPct, yPct } = worldToMapPercent(boss.x, boss.y, area);
          return (
            // A button wrapping the portrait, not a bare <img>. The pan/zoom
            // library ships `.transform-component-... img { pointer-events:
            // none }` so image-dragging can't fight panning, and that beats a
            // utility class on specificity — an <img> here can never take a
            // click. Wrapping sidesteps it, and brings keyboard focus with it.
            <button
              key={`alpha-${boss.x}-${boss.y}`}
              type="button"
              title={boss.level ? `${boss.name} · Lv ${boss.level}` : boss.name}
              aria-label={`Field boss: ${boss.name}`}
              onClick={() => onFieldBossSelect?.(boss)}
              className="pointer-events-auto absolute cursor-pointer rounded-full border border-paper/80 bg-ink/70 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-brand-amber"
              style={{
                left: `${xPct}%`,
                top: `${yPct}%`,
                width: 20,
                height: 20,
                transform: "translate(-50%,-50%) scale(var(--poi-inv))",
                boxShadow: `0 0 0 1.5px ${POI_META.alpha.color}`,
              }}
            >
              <img
                src={fieldBossIconUrl(boss.palId ?? "")}
                alt=""
                loading="lazy"
                className="h-full w-full rounded-full object-contain"
              />
            </button>
          );
        })}
    </div>
  );
}

const MAX_SCALE = 12;

// Rendered inside <TransformWrapper> so useControls() can reach its pan/zoom
// context. Watches for a pending "go to this player" request and animates
// there once the target marker actually exists in the DOM — which, after an
// area switch, is only true once the *new* TransformWrapper (it remounts via
// `key={area}`) has mounted.
//
// On a fresh mount the library sets up its own post-init ResizeObserver
// (for `centerOnInit`) that re-centers the view on the first layout change
// it sees and then disconnects — which, if the background image finishes
// loading shortly after our zoomToElement call, fires *after* us and
// silently undoes it. `settleSignal` (bumped by the image's onLoad) makes
// this effect re-fire once that's happened, so our call is the one that
// actually sticks regardless of which one lands first.
function FocusOnPlayer({
  focusId,
  settleSignal,
  onDone,
}: {
  /** DOM id of the marker to zoom to — a player pin or a base marker. */
  focusId: string | null;
  settleSignal: number;
  onDone: () => void;
}) {
  const { zoomToElement } = useControls();

  useEffect(() => {
    if (!focusId) return;
    const raf = requestAnimationFrame(() => zoomToElement(focusId, 4, 500));
    return () => cancelAnimationFrame(raf);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusId, settleSignal]);

  useEffect(() => {
    if (!focusId) return;
    // Keep focusId "live" past the animation so a late settleSignal change
    // (image load) still triggers the re-fire above, then clear it — long
    // enough to cover a slow-ish image load, short enough that re-clicking
    // the same player again later still registers as a fresh request.
    const t = setTimeout(onDone, 1500);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusId]);

  return null;
}

// The library's centerOnInit measures the wrapper exactly once, via a
// one-shot ResizeObserver that disconnects after its first layout change —
// so if the surrounding layout settles later (nav panels mounting, fonts,
// a window resize), the content is left centered against a stale wrapper
// size until the next gesture snaps it. This keeps a persistent observer
// and re-centers while at fit zoom; when zoomed in, a recenter would yank
// the view around, so the library's own bounds clamping on the next
// gesture is left to handle it.
function KeepCenteredOnResize() {
  const context = useTransformContext();
  const { centerView } = useControls();

  // useTransformInit (not a plain effect): wrapperComponent is only
  // assigned once the library finishes initializing, which is after this
  // component's own mount effect would have run and found null.
  useTransformInit(() => {
    const wrapper = context.wrapperComponent;
    if (!wrapper) return;
    const ro = new ResizeObserver(() => {
      if (context.state.scale <= 1.001) centerView(1, 0);
    });
    ro.observe(wrapper);
    return () => ro.disconnect();
  });

  return null;
}

// Backing resolution of the texture canvas. The source images are 8192x8192,
// but rendered via <img> Chromium re-decodes the full webp (~300-450ms,
// blocking) every time the zoom crosses into a draw scale whose downscaled
// bitmap isn't in its decoded-image cache — and the full-res entry is too
// big (268MB raw) to stay cached reliably, so the stall can recur at any
// time ("sometimes laggy"). Decoding ONCE off-main-thread via
// createImageBitmap into a fixed-size canvas removes decoding from the
// zoom path entirely: canvases never re-decode, so every zoom/pan is pure
// GPU compositing. 4096² (67MB) trades a little sharpness at extreme zoom
// (>4.8x on a ~850px viewport) for deterministic smoothness.
const TEXTURE_SIZE = 4096;

/**
 * The zoomable map: texture, pins, zoom controls. Selection state lives in
 * the page (ServerMap) so the HUD and player list stay in sync with it;
 * this component only reports clicks up via onSelect.
 *
 * The component root — the zoom viewport — fills whatever region it's given
 * at any aspect ratio. Only the CONTENT inside the transform must stay
 * square: player positions are percentages of the square 8192x8192 texture,
 * so a non-square content box would crop the image asymmetrically while the
 * percentage math stayed naive to it, drifting every pin. The content square
 * is sized min(100cqw,100cqh) against the root (container-type: size) —
 * beware "aspect-square h-full max-w-full" here: an explicit h-full stops
 * aspect-ratio from shrinking the height once max-w-full caps the width,
 * which silently produced a non-square box in portrait regions.
 */
export interface MapMarker {
  id: string;
  label: string;
  sublabel?: string;
  x: number;
  y: number;
  kind: "base" | "offline";
}

export function PlayerMap({
  players,
  markers = [],
  poiLayers,
  area,
  selectedId,
  focusId,
  onSelect,
  onMarkerSelect,
  onPoiSelect,
  onFocusDone,
  className,
}: {
  players: Player[];
  /** Save-derived overlays: guild bases and where offline players logged
   * off. Drawn beneath live players, which are the reason for the view. */
  markers?: MapMarker[];
  /** Static POI layers to draw (vendored data), beneath everything else. */
  poiLayers?: Set<PoiKind>;
  area: MapArea;
  selectedId: string | null;
  focusId: string | null;
  onSelect: (player: Player) => void;
  /** Tap/click on a base or offline marker. Without hover there are no
   * tooltips on touch, so tapping is how mobile learns what a marker is. */
  onMarkerSelect?: (marker: MapMarker) => void;
  /** Tap/click on a named POI (fast travel statue, watchtower). */
  onPoiSelect?: (name: string, x: number, y: number) => void;
  onFocusDone: () => void;
  className?: string;
}) {
  const [texState, setTexState] = useState<"loading" | "ready" | "missing">("loading");
  // Which field boss pin is open. Local to the map: nothing above it needs to
  // know, and the dialog reads everything it shows off the pin itself.
  const [fieldBoss, setFieldBoss] = useState<MapBossPin | null>(null);
  const [settleSignal, setSettleSignal] = useState(0);
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  // Decode the area's texture once (off the main thread) into the canvas,
  // trying each candidate path in order. The canvas remounts per area along
  // with the TransformWrapper (key={area}), so this re-runs against the
  // fresh element.
  useEffect(() => {
    let cancelled = false;
    setTexState("loading");
    (async () => {
      for (const url of MAP_AREAS[area].textureCandidates) {
        try {
          const res = await fetch(url);
          if (!res.ok) continue;
          const blob = await res.blob();
          const bitmap = await createImageBitmap(blob, {
            resizeWidth: TEXTURE_SIZE,
            resizeHeight: TEXTURE_SIZE,
            resizeQuality: "high",
          });
          if (cancelled) {
            bitmap.close();
            return;
          }
          const ctx = canvasRef.current?.getContext("2d");
          if (!ctx) return;
          ctx.drawImage(bitmap, 0, 0);
          bitmap.close();
          setTexState("ready");
          setSettleSignal((t) => t + 1);
          return;
        } catch {
          // decode/network failure — try the next candidate
        }
      }
      if (!cancelled) setTexState("missing");
    })();
    return () => {
      cancelled = true;
    };
  }, [area]);

  const hasBackground = texState !== "missing";

  const playersHere = players.filter((p) => mapOf(p.location_x, p.location_y) === area);

  return (
    <div className={cn("relative h-full w-full overflow-hidden [container-type:size]", className)}>
      <TransformWrapper
        key={area}
        minScale={1}
        maxScale={MAX_SCALE}
        initialScale={1}
        centerOnInit
        // Whenever the square content is smaller than the viewport in an
        // axis (e.g. horizontally at 1x on a wide screen), pin it centered
        // in that axis instead of letting it be dragged around the empty
        // space; panning behaves normally once zoomed past the fit size.
        centerZoomedOut
        doubleClick={{ mode: "zoomIn" }}
        panning={{ velocityDisabled: true }}
      >
        {({ zoomIn, zoomOut, resetTransform }) => (
          <>
            <FocusOnPlayer focusId={focusId} settleSignal={settleSignal} onDone={onFocusDone} />
            <KeepCenteredOnResize />

            <div className="absolute bottom-2 right-2 z-10 flex flex-col gap-1">
              <Button variant="secondary" size="icon" className="h-7 w-7" title="Zoom in" onClick={() => zoomIn()}>
                <ZoomIn className="h-4 w-4" />
              </Button>
              <Button variant="secondary" size="icon" className="h-7 w-7" title="Zoom out" onClick={() => zoomOut()}>
                <ZoomOut className="h-4 w-4" />
              </Button>
              <Button
                variant="secondary"
                size="icon"
                className="h-7 w-7"
                title="Reset view"
                onClick={() => resetTransform()}
              >
                <Maximize className="h-4 w-4" />
              </Button>
            </div>

            <TransformComponent wrapperClass="!w-full !h-full">
              {/* The CONTENT square: pins are percentage-positioned against
                  this box, so it must stay square — but the zoom viewport
                  around it fills the whole region at any aspect ratio, so
                  zooming in uses every pixel of a wide (or tall) screen
                  instead of staying letterboxed to the square. Sized by
                  container query against the component root. */}
              <div
                className="relative h-[min(100cqw,100cqh)] w-[min(100cqw,100cqh)]"
                style={
                  !hasBackground
                    ? {
                        backgroundImage:
                          "linear-gradient(hsl(var(--border)) 1px, transparent 1px), linear-gradient(90deg, hsl(var(--border)) 1px, transparent 1px)",
                        backgroundSize: "5% 5%",
                      }
                    : undefined
                }
              >
                <canvas
                  ref={canvasRef}
                  width={TEXTURE_SIZE}
                  height={TEXTURE_SIZE}
                  role="img"
                  aria-label={`Palworld map — ${MAP_AREAS[area].label}`}
                  className="absolute inset-0 h-full w-full"
                />

                {poiLayers && poiLayers.size > 0 && (
                  <PoiLayer
                    area={area}
                    layers={poiLayers}
                    onPoiSelect={onPoiSelect}
                    onFieldBossSelect={setFieldBoss}
                  />
                )}

                {markers
                  .filter((m) => mapOf(m.x, m.y) === area)
                  .map((m) => {
                    const { xPct, yPct } = worldToMapPercent(m.x, m.y, area);
                    return (
                      <ScaledPin key={m.id} id={mapMarkerId(m.id)} left={xPct} top={yPct}>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            {/* after:-inset-2 pads the tap target past the
                                16px pin — markers must be hittable with a
                                thumb, not just a cursor. */}
                            <button
                              onClick={() => onMarkerSelect?.(m)}
                              className={cn(
                                "relative flex h-4 w-4 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-[3px] border shadow after:absolute after:-inset-2 after:content-['']",
                                m.kind === "base"
                                  ? "border-paper bg-brand-amber"
                                  : "border-paper/70 bg-ink/60",
                              )}
                            >
                              {m.kind === "base" ? (
                                <Home className="h-2.5 w-2.5 text-ink" />
                              ) : (
                                <span className="h-1.5 w-1.5 rounded-full bg-paper/80" />
                              )}
                            </button>
                          </TooltipTrigger>
                          <TooltipContent>
                            {m.label}
                            {m.sublabel && <span className="block text-xs opacity-70">{m.sublabel}</span>}
                          </TooltipContent>
                        </Tooltip>
                      </ScaledPin>
                    );
                  })}

                {playersHere.map((p) => {
                  const { xPct, yPct } = worldToMapPercent(p.location_x, p.location_y, area);
                  const selected = selectedId === p.playerId;
                  return (
                    <ScaledPin key={p.playerId} id={markerId(p.playerId)} left={xPct} top={yPct}>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <button
                            onClick={() => onSelect(p)}
                            className={cn(
                              "relative h-4 w-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-paper shadow transition-transform after:absolute after:-inset-2 after:content-['']",
                              selected && "scale-[1.35]",
                            )}
                            style={{
                              backgroundColor: playerColor(p.playerId),
                              boxShadow: selected ? "0 0 0 4px rgba(232,73,29,0.35)" : undefined,
                            }}
                          />
                        </TooltipTrigger>
                        <TooltipContent>{p.name}</TooltipContent>
                      </Tooltip>
                    </ScaledPin>
                  );
                })}
              </div>
            </TransformComponent>
          </>
        )}
      </TransformWrapper>

      {!hasBackground && (
        <p className="pointer-events-none absolute bottom-2 left-2 z-10 text-xs text-muted-foreground">
          No map image found — see web/public/README.md
        </p>
      )}

      {hasBackground && playersHere.length === 0 && players.length > 0 && (
        <p className="pointer-events-none absolute bottom-2 left-2 z-10 rounded bg-background/80 px-2 py-1 text-xs text-muted-foreground">
          No players on this map right now.
        </p>
      )}

      <FieldBossDialog boss={fieldBoss} onClose={() => setFieldBoss(null)} />
    </div>
  );
}
