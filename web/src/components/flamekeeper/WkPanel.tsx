import { cn } from "../../lib/utils";

/**
 * The Flamekeeper panel grammar from the mock: a Cinzel header over a faint
 * brass gradient, an optional right-aligned meta line, and a body. Every
 * Dragonwilds page is built from these so the pages read as one system.
 */
export function WkPanel({
  title,
  meta,
  children,
  className,
  bodyClassName,
}: {
  title: string;
  meta?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
  bodyClassName?: string;
}) {
  return (
    <section className={cn("overflow-hidden rounded-md border border-wk-edge bg-wk-panel", className)}>
      <div className="flex items-baseline justify-between gap-3 border-b border-wk-edge bg-gradient-to-b from-wk-brasshi/5 to-transparent px-4 py-3">
        <h3 className="font-wkdisplay text-sm font-semibold uppercase tracking-[0.08em] text-wk-brasshi">{title}</h3>
        {meta && <span className="text-xs text-wk-mist">{meta}</span>}
      </div>
      <div className={cn("px-4 py-3.5", bodyClassName)}>{children}</div>
    </section>
  );
}

/** The mock's stat tile: small caps label, Cinzel value, hint, meter. */
export function WkStat({
  label,
  value,
  unit,
  hint,
  meterPct,
  warm,
}: {
  label: string;
  value: React.ReactNode;
  unit?: string;
  hint?: string;
  /** 0–100 fill for the little meter; omit for no meter. */
  meterPct?: number;
  /** Brass fill instead of rune — for capacity-ish readings, per the mock. */
  warm?: boolean;
}) {
  return (
    <div className="rounded-md border border-wk-edge bg-wk-panel px-4 pb-3 pt-3.5">
      <div className="text-[11px] uppercase tracking-[0.14em] text-wk-mist">{label}</div>
      <div className="mt-1.5 font-wkdisplay text-2xl font-semibold text-wk-parchment">
        {value}
        {unit && <small className="ml-1 font-wkbody text-sm font-normal text-wk-mist">{unit}</small>}
      </div>
      {hint && <div className="mt-0.5 text-xs text-wk-mist">{hint}</div>}
      {meterPct !== undefined && (
        <div className="mt-2.5 h-[5px] overflow-hidden rounded bg-wk-ink">
          <i
            className={cn(
              "block h-full",
              warm
                ? "bg-gradient-to-r from-[#6e5a2a] to-wk-brasshi"
                : "bg-gradient-to-r from-wk-runedim to-wk-rune",
            )}
            style={{ width: `${Math.max(0, Math.min(100, meterPct))}%` }}
          />
        </div>
      )}
    </div>
  );
}

/** Capability note, the mock's italic log-panel footnote. */
export function WkNote({ children }: { children: React.ReactNode }) {
  return <p className="mt-2.5 text-xs italic text-wk-mist">{children}</p>;
}

/** Colorize one server-log line by what it says, not by inventing levels. */
export function wkLogTone(line: string): string {
  if (/error|fatal/i.test(line)) return "text-wk-ember";
  if (/warning/i.test(line)) return "text-wk-brasshi";
  if (line.includes("LogNet: Join succeeded:")) return "text-wk-rune";
  if (line.includes("ClientRequestDisconnect")) return "text-wk-brasshi";
  if (/save/i.test(line)) return "text-wk-ok";
  return "text-wk-parchment/70";
}
