import type { ItemSlot } from "../lib/api";
import { palIconUrl, palName, passiveDescription, passiveName } from "../lib/paldex";
import {
  RARITY_NAMES,
  durabilityFraction,
  itemCategory,
  itemEntry,
  itemIconUrl,
  itemName,
  rarityColor,
} from "../lib/items";
import { cn } from "../lib/utils";
import { Badge } from "./ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "./ui/dialog";

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-ink/10 bg-ink/[0.03] px-3 py-2">
      <p className="text-[10px] font-semibold uppercase tracking-wide text-ink/40">{label}</p>
      <p className="mt-0.5 font-mono text-sm font-bold text-ink">{value}</p>
    </div>
  );
}

export function ItemDetailDialog({
  slot,
  location,
  onClose,
}: {
  slot: ItemSlot | null;
  /** The container the item sits in, named the way the page names it. */
  location: string;
  onClose: () => void;
}) {
  if (!slot) return null;

  const entry = itemEntry(slot.itemId);
  const color = rarityColor(slot.itemId);
  const wear = durabilityFraction(slot.itemId, slot.durability);
  // The item's own designed passives, plus anything this particular drop
  // rolled — shown together because a player can't tell them apart either.
  const passives = [...(entry?.ps ?? []), ...(slot.passives ?? [])];
  const icon = itemIconUrl(slot.itemId);

  const stats: [string, string][] = [];
  if (entry?.dmg) stats.push(["Attack", entry.dmg.toLocaleString()]);
  if (entry?.def) stats.push(["Defense", entry.def.toLocaleString()]);
  if (slot.ammo !== undefined) stats.push(["Loaded", `${slot.ammo}${entry?.mag ? ` / ${entry.mag}` : ""}`]);
  if (entry?.w) stats.push(["Weight", `${(entry.w * slot.count).toLocaleString()} kg`]);

  // The container this came from is often named for its category too
  // ("Weapons · Weapons"), so the two collapse into one when they agree.
  const category = itemCategory(slot.itemId);
  const where = [category, location === category ? "" : location, `slot ${slot.slot + 1}`].filter(Boolean);

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="sr-only">{itemName(slot.itemId)}</DialogTitle>
        </DialogHeader>

        <div className="flex items-start gap-4">
          <div
            className="flex h-16 w-16 shrink-0 items-center justify-center rounded-xl border bg-ink"
            style={{ borderColor: color ? `${color}66` : "rgba(43,36,32,0.15)" }}
          >
            {icon && (
              <img
                src={icon}
                alt=""
                className="h-12 w-12 object-contain"
                loading="lazy"
                decoding="async"
                onError={(e) => {
                  e.currentTarget.style.visibility = "hidden";
                }}
              />
            )}
          </div>

          <div className="min-w-0 flex-1">
            <h2 className="font-display text-xl font-bold leading-tight">{itemName(slot.itemId)}</h2>
            <p className="mt-0.5 font-mono text-xs text-ink/45">{where.join(" · ")}</p>
            <div className="mt-2 flex flex-wrap items-center gap-1.5">
              {color && (
                <Badge
                  variant="outline"
                  className="px-1.5 py-0 text-[10px]"
                  style={{ borderColor: `${color}66`, backgroundColor: `${color}1a`, color }}
                >
                  {RARITY_NAMES[entry?.r ?? 0]}
                </Badge>
              )}
              {slot.count > 1 && (
                <span className="rounded-full bg-ink px-2 py-0.5 font-mono text-xs font-bold text-paper">
                  ×{slot.count.toLocaleString()}
                </span>
              )}
            </div>
          </div>
        </div>

        {slot.eggSpecies && (
          <div className="mt-4 flex items-center gap-3 rounded-xl border border-brand-amber/30 bg-brand-amber/10 px-3 py-2.5">
            <img
              src={palIconUrl(slot.eggSpecies)}
              alt=""
              className="h-9 w-9 shrink-0 object-contain"
              loading="lazy"
              decoding="async"
              onError={(e) => {
                e.currentTarget.style.visibility = "hidden";
              }}
            />
            <div>
              <p className="text-[10px] font-semibold uppercase tracking-wide text-ink/45">Hatches into</p>
              <p className="font-display text-base font-bold leading-tight">{palName(slot.eggSpecies)}</p>
            </div>
          </div>
        )}

        {wear !== undefined && (
          <div className="mt-4">
            <div className="flex items-baseline justify-between">
              <span className="text-xs text-ink/50">Condition</span>
              {/* The exact figures live here rather than in a stat tile of
                  their own — one element, one job. */}
              <span className="font-mono text-xs font-bold text-ink">
                {Math.round(slot.durability!).toLocaleString()} / {entry!.dur!.toLocaleString()}
                <span className="ml-1.5 font-normal text-ink/45">{Math.round(wear * 100)}%</span>
              </span>
            </div>
            <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-ink/10">
              <div
                className="h-full rounded-full"
                style={{
                  width: `${wear * 100}%`,
                  // Green while it's fine, amber past halfway, red when a
                  // repair is overdue — the only place condition is judged
                  // rather than just reported.
                  backgroundColor: wear > 0.5 ? "#4A9D7C" : wear > 0.2 ? "#F2A93B" : "#E8491D",
                }}
              />
            </div>
          </div>
        )}

        {stats.length > 0 && (
          <div className={cn("mt-4 grid gap-2", stats.length > 2 ? "grid-cols-3" : "grid-cols-2")}>
            {stats.map(([label, value]) => (
              <Stat key={label} label={label} value={value} />
            ))}
          </div>
        )}

        {passives.length > 0 && (
          <div className="mt-4">
            <p className="text-[10px] font-semibold uppercase tracking-wide text-ink/40">Effects</p>
            <ul className="mt-1.5 space-y-1">
              {passives.map((code) => (
                <li key={code} className="text-sm text-ink/70">
                  <span className="font-semibold text-ink">{passiveName(code)}</span>
                  {passiveDescription(code) && <span className="text-ink/50"> — {passiveDescription(code)}</span>}
                </li>
              ))}
            </ul>
          </div>
        )}

        {entry?.d && <p className="mt-4 text-sm leading-relaxed text-ink/60">{entry.d}</p>}

        <p className="mt-4 border-t border-ink/10 pt-3 font-mono text-[11px] text-ink/30">{slot.itemId}</p>
      </DialogContent>
    </Dialog>
  );
}
