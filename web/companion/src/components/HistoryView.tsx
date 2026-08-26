import type { ReactNode } from "react";
import { AlertTriangle, RefreshCw } from "lucide-react";
import { fmtBytes, fmtWhen, plural } from "../lib/format";
import { cn } from "../lib/utils";
import { SectionHeader } from "./Panel";
import { Button } from "./ui/button";
import type { History, HistoryEntry } from "../lib/types";

/** The verb that made a version, as a word rather than a database value. */
const KIND: Record<string, string> = {
  checkin: "checked in",
  checkpoint: "checkpointed",
  import: "seeded",
};

const kindOf = (e: HistoryEntry) => KIND[e.kind] ?? e.kind;

/**
 * One version, as a line.
 *
 * The same row serves both views. In Activity it is a thing that
 * happened; in Conflicts it is a thing that happened *and was refused* —
 * so the conflict marking lives on the row rather than in the view, and a
 * conflict is recognisable wherever it turns up.
 */
export function HistoryRow({
  entry,
  showWorld = true,
}: {
  entry: HistoryEntry;
  showWorld?: boolean;
}) {
  return (
    <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 px-[18px] py-3">
      {showWorld ? <span className="font-serif text-[14px] text-parchment">{entry.worldName}</span> : null}
      <span className="text-[13px] text-mist">
        {entry.uploader ? <span className="text-parchment">{entry.uploader}</span> : "someone"}{" "}
        {kindOf(entry)}
      </span>
      {entry.conflict ? (
        <span className="rounded-[3px] border border-ember/50 px-1.5 py-px font-mono text-[10px] uppercase tracking-[0.08em] text-ember">
          conflict
        </span>
      ) : null}
      {entry.head ? (
        <span className="rounded-[3px] border border-gold/50 px-1.5 py-px font-mono text-[10px] uppercase tracking-[0.08em] text-gold">
          current
        </span>
      ) : null}
      <span className="ml-auto whitespace-nowrap font-mono text-[11px] text-mist">
        v{entry.versionId} · {fmtBytes(entry.bytes)} · {fmtWhen(entry.createdAt)}
      </span>
    </div>
  );
}

/** The card the rows sit in — the same treatment as a worlds group, and
 * deliberately not clipping, for the same reason: things open inside it. */
export function HistoryCard({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-panel border border-edge bg-panel [&>*+*]:border-t [&>*+*]:border-edge [&>*:first-child]:rounded-t-panel [&>*:last-child]:rounded-b-panel">
      {children}
    </div>
  );
}

/**
 * What both views share: the heading with its refresh, and every way this
 * list can fail to be the whole truth.
 *
 * A history is a claim about what happened, so an incomplete one has to
 * say so. These views exist to notice something you did not do yourself;
 * a short list that looks complete is worse than an error.
 */
export function HistoryFrame({
  title,
  hint,
  history,
  loading,
  error,
  refreshing,
  onRefresh,
  children,
}: {
  title: string;
  hint: string;
  history: History | undefined;
  loading: boolean;
  error?: string;
  refreshing: boolean;
  onRefresh: () => void;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3.5 px-7 pb-6 pt-5">
      <div className="flex flex-wrap items-baseline gap-3">
        <SectionHeader title={title} />
        <Button
          variant="bare"
          size="icon"
          className="ml-auto"
          onClick={onRefresh}
          disabled={refreshing}
          aria-label={refreshing ? "Refreshing…" : "Refresh"}
          title={refreshing ? "Refreshing…" : "Refresh"}
        >
          <RefreshCw className={cn("h-4 w-4", refreshing && "animate-spin")} aria-hidden />
        </Button>
      </div>
      <p className="max-w-[68ch] text-[13px] text-mist">{hint}</p>

      {error ? (
        <p className="font-mono text-[12px] text-ember">The vault could not be read: {error}</p>
      ) : loading ? (
        <p className="text-[13px] italic text-mist">Reading the vault…</p>
      ) : (
        children
      )}

      {history?.failed?.length ? (
        <div className="flex flex-col gap-1.5 rounded-panel border border-dashed border-ember/50 bg-well px-[18px] py-3">
          <div className="flex items-center gap-2 text-[12.5px] text-ember">
            <AlertTriangle className="h-3.5 w-3.5 flex-none" aria-hidden />
            {plural(history.failed.length, "world", "worlds")} could not be read, so this list is
            incomplete.
          </div>
          {history.failed.map((f) => (
            <div key={f.worldId} className="font-mono text-[11px] text-mist">
              {f.worldName || "world #" + f.worldId}: {f.error}
            </div>
          ))}
        </div>
      ) : null}

      {history?.truncated ? (
        <p className="text-[12px] italic text-mist">
          {plural(history.truncated, "further world is", "further worlds are")} linked and not
          shown — the newest are read first.
        </p>
      ) : null}
    </div>
  );
}
