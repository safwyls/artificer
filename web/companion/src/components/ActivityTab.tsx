import { useMemo } from "react";
import { HistoryCard, HistoryFrame, HistoryRow } from "./HistoryView";
import type { History, HistoryEntry } from "../lib/types";

/**
 * Activity: what has happened to the worlds this machine is linked to,
 * newest first.
 *
 * This is the vault's record, not this machine's, and that distinction is
 * the whole point. The companion already knows what it did itself and
 * says so in the titlebar. What it cannot know without asking is that
 * someone else checked a world in an hour ago — which is exactly what
 * anyone opening this tab came to find out.
 *
 * Grouped by day because that is how the question gets asked ("what
 * happened today?"), and a flat column of timestamps answers it badly.
 */
export function ActivityTab({
  history,
  loading,
  error,
  refreshing,
  onRefresh,
}: {
  history: History | undefined;
  loading: boolean;
  error?: string;
  refreshing: boolean;
  onRefresh: () => void;
}) {
  // The entries arrive newest-first, so a single pass groups them: a day
  // changes exactly once per run of rows.
  const days = useMemo(() => {
    const out: { label: string; entries: HistoryEntry[] }[] = [];
    for (const e of history?.entries ?? []) {
      const label = dayLabel(e.createdAt);
      const last = out[out.length - 1];
      if (last && last.label === label) last.entries.push(e);
      else out.push({ label, entries: [e] });
    }
    return out;
  }, [history]);

  return (
    <HistoryFrame
      title="Activity"
      hint="Every version of every world this machine is linked to, as the vault recorded it — including the ones checked in from somewhere else."
      history={history}
      loading={loading}
      error={error}
      refreshing={refreshing}
      onRefresh={onRefresh}
    >
      {days.length === 0 ? (
        <p className="text-[13px] italic text-mist">
          Nothing has happened to these worlds yet. A check-in, a checkpoint, and the save that
          seeded a world all appear here.
        </p>
      ) : (
        <div className="flex flex-col gap-4">
          {days.map((day) => (
            <section key={day.label} className="flex flex-col gap-2">
              <h3 className="text-[11px] font-semibold uppercase tracking-[0.12em] text-mist">
                {day.label} · {day.entries.length}
              </h3>
              <HistoryCard>
                {day.entries.map((e) => (
                  <HistoryRow key={e.worldId + "-" + e.versionId} entry={e} />
                ))}
              </HistoryCard>
            </section>
          ))}
        </div>
      )}
    </HistoryFrame>
  );
}

/**
 * "Today", "Yesterday", then the date. Only the grouping — the row
 * itself already says how long ago it was.
 *
 * Exported for its own test: off-by-one in a day boundary is the kind of
 * bug that only shows up near midnight.
 */
export function dayLabel(iso: string, now = new Date()): string {
  const at = new Date(iso);
  if (Number.isNaN(at.getTime())) return "At an unknown time";
  const midnight = new Date(now);
  midnight.setHours(0, 0, 0, 0);
  if (at.getTime() >= midnight.getTime()) return "Today";
  if (at.getTime() >= midnight.getTime() - 86_400_000) return "Yesterday";
  return at.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}
