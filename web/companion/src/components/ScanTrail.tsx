import { useEffect, useRef, useState } from "react";
import { plural } from "../lib/format";
import type { Probe } from "../lib/types";

/**
 * Every path the scan tried, what it resolved to, and why a miss missed.
 * This is the whole answer to "why no games", so its open state belongs
 * to the player and must survive the five-second poll — rewriting its
 * markup on every poll was what snapped it shut two seconds after anyone
 * expanded it.
 *
 * A fresh scan that found nothing opens itself; otherwise the player's
 * last choice stands, and a first render defaults to closed-on-success.
 */
export function ScanTrail({ probes }: { probes: Probe[] }) {
  const hits = probes.filter((p) => p.resolved);
  // null means "the player has not decided" — distinct from "closed".
  const [chosen, setChosen] = useState<boolean | null>(null);
  const seen = useRef<string | null>(null);

  const sig = JSON.stringify(probes);
  useEffect(() => {
    const fresh = seen.current !== null && seen.current !== sig;
    seen.current = sig;
    // A new scan that found nothing opens itself. One that found
    // something leaves the player's choice alone.
    if (fresh && !probes.some((p) => p.resolved)) setChosen(true);
  }, [sig, probes]);

  if (!probes.length) return null;
  const open = chosen === null ? !hits.length : chosen;

  return (
    <details
      open={open}
      onToggle={(e) => setChosen((e.target as HTMLDetailsElement).open)}
      className="font-mono text-[12px] text-mist"
    >
      <summary className="cursor-pointer">
        scan trail — {plural(hits.length, "library", "libraries")} found,{" "}
        {plural(probes.length, "path", "paths")} tried
      </summary>
      {probes.map((p, i) => (
        <div key={`${p.source}:${p.path}:${i}`} className="py-0.5">
          <span className={p.resolved ? "text-ok" : "text-mist"}>{p.resolved ? "✓" : "·"}</span>{" "}
          <b>{p.source}</b> <span className="break-all">{p.path}</span>
          {p.resolved && p.resolved !== p.path ? (
            <div className="break-all pl-4">→ {p.resolved}</div>
          ) : null}
          {p.note ? (
            <div className={p.resolved ? "pl-4 text-mist" : "pl-4 text-ember"}>{p.note}</div>
          ) : null}
        </div>
      ))}
    </details>
  );
}
