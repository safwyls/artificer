import { useEffect } from "react";

/**
 * Rendered only in demo builds (VITE_DEMO=1): a fixed ribbon so nobody
 * mistakes the sample world for their own server, with the way home.
 */
export function DemoBanner() {
  useEffect(() => {
    document.title = "Palcon — live demo";
  }, []);

  return (
    <div className="pointer-events-none fixed inset-x-0 bottom-3 z-50 flex justify-center px-3">
      <div className="pointer-events-auto flex items-center gap-2.5 rounded-full border border-white/10 bg-ink px-4 py-1.5 text-xs text-paper/80 shadow-lg">
        <span className="h-2 w-2 shrink-0 rounded-full bg-brand-amber" />
        <span>
          <span className="font-semibold text-paper">Demo</span> · sample world, actions are simulated
        </span>
        <a
          href="https://safwyls.github.io/palcon/"
          className="font-semibold text-brand-amber hover:underline"
        >
          Get Palcon →
        </a>
      </div>
    </div>
  );
}
