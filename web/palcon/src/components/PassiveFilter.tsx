import { useMemo, useState } from "react";
import { Check, SlidersHorizontal } from "lucide-react";
import { passiveTierByName } from "../lib/paldex";
import { cn } from "../lib/utils";
import { PassiveTierTile } from "./PassiveBadge";
import { Input } from "./ui/input";

/**
 * Multi-select over the passives present in a roster, shared by the pal
 * viewer's filters and the calculators' parent picker.
 *
 * Options are keyed by display name, so two internal codes that read the
 * same ("Brave") collapse into one row — and they're ranked by the game's
 * tier rather than alphabetically. When you're hunting a breeding parent
 * you want Legend and Lucky at the top of the list, not Aggressive; the
 * tier tiles down the left edge make the ranking legible as bands.
 */
export function PassiveFilter({
  counts,
  selected,
  onChange,
  align = "left",
}: {
  /** Display name -> how many pals carry it. */
  counts: Map<string, number>;
  selected: Set<string>;
  onChange: (next: Set<string>) => void;
  /** Which edge the menu hangs from, for triggers near a right margin. */
  align?: "left" | "right";
}) {
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const options = useMemo(() => {
    const all = [...counts.keys()]
      .map((name) => ({ name, tier: passiveTierByName(name) }))
      // Best tier first; untiered codes (gear, gym-boss leftovers) sink
      // below the ranked ones rather than sorting in among the negatives.
      .sort((a, b) => (b.tier || -9) - (a.tier || -9) || a.name.localeCompare(b.name));
    const needle = q.trim().toLowerCase();
    return needle ? all.filter((o) => o.name.toLowerCase().includes(needle)) : all;
  }, [counts, q]);

  const toggle = (name: string) => {
    const next = new Set(selected);
    next.has(name) ? next.delete(name) : next.add(name);
    onChange(next);
  };

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className={cn(
          "flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors",
          selected.size
            ? "border-brand-red/40 bg-brand-red/10 text-brand-red"
            : "border-ink/15 bg-white text-ink/70 hover:bg-ink/5",
        )}
      >
        <SlidersHorizontal className="h-3.5 w-3.5" />
        Passives
        {selected.size > 0 && <span className="font-mono text-xs">· {selected.size}</span>}
      </button>

      {open && (
        <>
          <div className="fixed inset-0 z-20" onClick={() => setOpen(false)} />
          <div
            className={cn(
              "absolute top-10 z-30 w-64 overflow-hidden rounded-xl border border-ink/10 bg-paper shadow-lg",
              align === "right" ? "right-0" : "left-0",
            )}
          >
            <div className="space-y-1.5 border-b border-ink/10 p-2">
              <Input
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder="Find a passive…"
                className="h-8 text-sm"
              />
              {/* The order isn't alphabetical on purpose — say so once. */}
              <p className="text-[10px] font-semibold uppercase tracking-wide text-ink/35">Best tier first</p>
            </div>
            <div className="max-h-72 overflow-y-auto p-1">
              {options.map(({ name, tier }) => {
                const on = selected.has(name);
                return (
                  <button
                    key={name}
                    type="button"
                    onClick={() => toggle(name)}
                    className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-ink/5"
                  >
                    <span
                      className={cn(
                        "flex h-4 w-4 shrink-0 items-center justify-center rounded border",
                        on ? "border-brand-red bg-brand-red text-paper" : "border-ink/25",
                      )}
                    >
                      {on && <Check className="h-3 w-3" />}
                    </span>
                    <PassiveTierTile tier={tier} />
                    <span className="min-w-0 flex-1 truncate text-foreground">{name}</span>
                    <span className="shrink-0 font-mono text-[11px] text-ink/40">{counts.get(name)}</span>
                  </button>
                );
              })}
              {options.length === 0 && <p className="px-2 py-3 text-sm text-muted-foreground">No passives match.</p>}
            </div>
            {selected.size > 0 && (
              <button
                type="button"
                onClick={() => onChange(new Set())}
                className="w-full border-t border-ink/10 px-3 py-2 text-left text-xs font-semibold text-brand-red hover:bg-ink/5"
              >
                Clear {selected.size} selected
              </button>
            )}
          </div>
        </>
      )}
    </div>
  );
}
