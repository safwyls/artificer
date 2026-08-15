import { cn } from "../../lib/utils";

/**
 * The Flamekeeper panel grammar (docs/design.md): a terrace — calm
 * rectangle, top-light catching the panel's lintel, a quiet stone title
 * in Karla (the blackletter display face stays reserved for identity
 * moments), an optional right-aligned meta line, and a body. Every page
 * is built from these so they read as one system.
 */
export function FkPanel({
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
    <section className={cn("fk-toplight overflow-hidden rounded-md border border-fk-edge bg-fk-panel", className)}>
      <div className="flex items-baseline justify-between gap-3 border-b border-fk-edge bg-gradient-to-b from-fk-stonehi/5 to-transparent px-4 py-3">
        <h3 className="text-[13px] font-bold uppercase tracking-[0.12em] text-fk-stonehi">{title}</h3>
        {meta && <span className="text-xs text-fk-lichen">{meta}</span>}
      </div>
      <div className={cn("px-4 py-3.5", bodyClassName)}>{children}</div>
    </section>
  );
}

/** The stat tile: small caps label, a display-face value (one of the
 * blackletter's few budgeted appearances), hint, meter. */
export function FkStat({
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
  /** Stone fill instead of flame — for capacity-ish readings. */
  warm?: boolean;
}) {
  return (
    <div className="fk-toplight rounded-md border border-fk-edge bg-fk-panel px-4 pb-3 pt-3.5">
      <div className="text-[11px] uppercase tracking-[0.14em] text-fk-lichen">{label}</div>
      <div className="mt-1.5 font-fkdisplay text-3xl font-medium leading-none text-fk-bone">
        {value}
        {unit && <small className="ml-1 font-fkbody text-sm font-normal text-fk-lichen">{unit}</small>}
      </div>
      {hint && <div className="mt-0.5 text-xs text-fk-lichen">{hint}</div>}
      {meterPct !== undefined && (
        <div className="mt-2.5 h-[5px] overflow-hidden rounded bg-fk-void">
          <i
            className={cn(
              "block h-full",
              warm
                ? "bg-gradient-to-r from-[#5c5747] to-fk-stonehi"
                : "bg-gradient-to-r from-fk-flamedim to-fk-flame",
            )}
            style={{ width: `${Math.max(0, Math.min(100, meterPct))}%` }}
          />
        </div>
      )}
    </div>
  );
}

/** Capability note, the italic log-panel footnote. */
export function FkNote({ children }: { children: React.ReactNode }) {
  return <p className="mt-2.5 text-xs italic text-fk-lichen">{children}</p>;
}

/** Colorize one server-log line by what it says, not by inventing levels.
 * The markers are Enshrouded's own (docs/enshrouded-recon.md, "Logs"). */
export function fkLogTone(line: string): string {
  if (/\[E |error|fatal|Failed to save/i.test(line)) return "text-fk-spore";
  if (/\[W |warning/i.test(line)) return "text-fk-stonehi";
  if (line.includes("Session accepted with peer") || line.includes("Added Peer")) return "text-fk-flame";
  if (line.includes("Removed Peer")) return "text-fk-stonehi";
  if (/Start Saving|'HostOnline' \(up\)/.test(line)) return "text-fk-ok";
  return "text-fk-bone/70";
}
