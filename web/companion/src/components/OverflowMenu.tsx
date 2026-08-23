import { useEffect, useRef, useState, type ReactNode } from "react";
import { MoreVertical } from "lucide-react";
import { cn } from "../lib/utils";

export interface MenuItem {
  label: string;
  onSelect: () => void;
  danger?: boolean;
  /** Draws a rule above this item — for separating the destructive verb
   * from the ordinary ones. */
  separated?: boolean;
}

/**
 * The rare and the destructive verbs, off the row.
 *
 * A world row used to carry four equal-weight buttons, which meant it
 * carried none: nothing on it said which one you were meant to press.
 * One primary, one quiet, and everything else behind these three dots.
 *
 * Built rather than pulled in: the app already ships one Radix dialog and
 * a menu is a button, a list, an outside click and Escape. Adding a
 * dependency for that would cost more than it saves.
 */
export function OverflowMenu({
  label = "More actions",
  header,
  items,
}: {
  label?: string;
  /** A machine fact worth having but not worth a row — the save path. */
  header?: ReactNode;
  items: MenuItem[];
}) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => {
      if (box.current && !box.current.contains(e.target as Node)) setOpen(false);
    };
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", away);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", away);
      document.removeEventListener("keydown", esc);
    };
  }, [open]);

  return (
    <div ref={box} className="relative">
      <button
        type="button"
        aria-label={label}
        aria-expanded={open}
        aria-haspopup="menu"
        onClick={() => setOpen((v) => !v)}
        className="inline-flex rounded border border-edge px-2.5 py-[7px] text-mist transition-colors hover:border-gold/60 hover:text-parchment"
      >
        <MoreVertical className="h-3.5 w-3.5" aria-hidden />
      </button>
      {open ? (
        <div
          role="menu"
          className="absolute right-0 top-[calc(100%+4px)] z-20 min-w-[13rem] rounded border border-edge bg-panel py-1 text-[13px] shadow-[0_20px_25px_-5px_rgba(0,0,0,0.4),0_8px_10px_-6px_rgba(0,0,0,0.4)]"
        >
          {header ? (
            <div className="break-all border-b border-edge px-3 py-2 font-mono text-[11px] text-mist">
              {header}
            </div>
          ) : null}
          {items.map((item) => (
            <div key={item.label}>
              {item.separated ? <div className="my-1 h-px bg-edge" /> : null}
              <button
                type="button"
                role="menuitem"
                onClick={() => {
                  setOpen(false);
                  item.onSelect();
                }}
                className={cn(
                  "block w-full px-3 py-1.5 text-left text-mist transition-colors",
                  item.danger ? "hover:bg-ember/10 hover:text-ember" : "hover:bg-ink hover:text-parchment",
                )}
              >
                {item.label}
              </button>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}
