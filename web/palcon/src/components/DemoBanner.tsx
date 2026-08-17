import { useEffect } from "react";

/**
 * Rendered only in demo builds (VITE_DEMO=1): a fixed ribbon so nobody
 * mistakes the sample world for their own server, with the way home.
 */
export function DemoBanner() {
  useEffect(() => {
    document.title = "Palcon — live demo";
  }, []);

  // Below lg the mobile server rail owns the bottom of the screen, so the
  // ribbon has to clear it: the rail is pt-2.5 plus a 40px sphere plus a
  // bottom pad of at least the safe-area inset, and this leaves a 12px gap
  // above that. From lg up the rail is hidden and the ribbon sits where it
  // always did.
  return (
    <div className="pointer-events-none fixed inset-x-0 bottom-[calc(3.875rem_+_max(0.625rem,_env(safe-area-inset-bottom)))] z-50 flex justify-center px-3 lg:bottom-3">
      <div className="pointer-events-auto flex items-center gap-2.5 rounded-full border border-white/10 bg-ink px-4 py-1.5 text-xs text-paper/80 shadow-lg">
        <span className="h-2 w-2 shrink-0 rounded-full bg-brand-amber" />
        <span>
          <span className="font-semibold text-paper">Demo</span> · sample world, actions are simulated
        </span>
        <a
          href="https://safwyls.github.io/sampo/"
          className="shrink-0 whitespace-nowrap font-semibold text-brand-amber hover:underline"
        >
          Get Palcon →
        </a>
      </div>
    </div>
  );
}
