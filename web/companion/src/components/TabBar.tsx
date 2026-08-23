import { cn } from "../lib/utils";

/**
 * The five places this window goes. Worlds is the whole page and the
 * default; everything else is somewhere you go on purpose.
 *
 * Offline is not a tab — it is a variant of Worlds, so the Worlds tab
 * stays active while the vault is unreachable.
 */
export type Tab = "worlds" | "games" | "activity" | "conflicts" | "settings";

const TABS: { id: Tab; label: string }[] = [
  { id: "worlds", label: "Worlds" },
  { id: "games", label: "Games" },
  { id: "activity", label: "Activity" },
  { id: "conflicts", label: "Conflicts" },
  { id: "settings", label: "Settings" },
];

export function TabBar({
  tab,
  onTab,
  conflicts = 0,
}: {
  tab: Tab;
  onTab: (t: Tab) => void;
  /** Shown as a badge, and only when non-zero: a "0" beside Conflicts is
   * a number nobody needs. */
  conflicts?: number;
}) {
  return (
    <div role="tablist" aria-label="Companion sections" className="flex gap-1 border-b border-edge px-7 pt-4">
      {TABS.map(({ id, label }) => {
        const active = tab === id;
        return (
          <button
            key={id}
            role="tab"
            type="button"
            aria-selected={active}
            onClick={() => onTab(id)}
            className={cn(
              // -1px so the active tab's rule sits *on* the header's own
              // border rather than under it.
              "-mb-px flex items-center gap-2 border-b-2 px-3.5 pb-2.5 pt-2 text-[14.5px] transition-colors",
              active
                ? "border-gold text-goldhi"
                : "border-transparent text-mist hover:text-parchment",
            )}
          >
            {label}
            {id === "conflicts" && conflicts > 0 ? (
              <span className="rounded-[3px] border border-ember/50 px-1.5 py-px font-mono text-[10px] tracking-[0.06em] text-ember">
                {conflicts}
              </span>
            ) : null}
          </button>
        );
      })}
    </div>
  );
}
