import { useMemo } from "react";
import { HistoryCard, HistoryFrame, HistoryRow } from "./HistoryView";
import { plural } from "../lib/format";
import type { History, HistoryEntry } from "../lib/types";

/**
 * Conflicts: the check-ins the vault refused to fast-forward onto.
 *
 * A conflict is not a separate record — it is a version with the flag
 * set, which the vault does when a check-in arrives from a session that
 * had already ended, or from one whose base is no longer the world's
 * head. Both mean the same thing in practice: two machines wrote the same
 * world, and only one of them can be the current save.
 *
 * This list is the same read the Activity tab makes, filtered. Fetching
 * it separately would let the two disagree about what happened.
 *
 * **Nothing here is resolvable from this window, and it says so.** Moving
 * a world's head is an admin verb on the vault and is deliberately not on
 * the companion's token tier — so this view names where the ability
 * actually lives rather than showing a button that would 403. The
 * conflicting saves are safe meanwhile: flagged versions are exempt from
 * pruning until someone picks.
 */
export function ConflictsTab({
  history,
  loading,
  error,
  refreshing,
  onRefresh,
  serverUrl,
}: {
  history: History | undefined;
  loading: boolean;
  error?: string;
  refreshing: boolean;
  onRefresh: () => void;
  /** Where the vault is, so the sentence about resolving one can name it
   * rather than saying "your sync service". Shown as text, not a link:
   * this window has no way to open an external browser, and a dead link
   * is worse than an address you can read. */
  serverUrl?: string;
}) {
  // Grouped by world, because a conflict is about one world having two
  // futures — and the versions in contention are only comparable to each
  // other.
  const worlds = useMemo(() => {
    const by = new Map<number, { name: string; entries: HistoryEntry[] }>();
    for (const e of history?.entries ?? []) {
      if (!e.conflict) continue;
      const found = by.get(e.worldId);
      if (found) found.entries.push(e);
      else by.set(e.worldId, { name: e.worldName, entries: [e] });
    }
    return [...by.entries()].map(([worldId, v]) => ({ worldId, ...v }));
  }, [history]);

  const total = worlds.reduce((n, w) => n + w.entries.length, 0);

  return (
    <HistoryFrame
      title="Conflicts"
      hint="A conflict is a check-in the vault would not accept as the world's current save — it arrived from a hold that had already ended, or from one whose starting point had moved. Both saves are kept."
      history={history}
      loading={loading}
      error={error}
      refreshing={refreshing}
      onRefresh={onRefresh}
    >
      {worlds.length === 0 ? (
        <p className="text-[13px] italic text-mist">
          No conflicts. Every check-in the vault has for these worlds arrived from the hold that
          was current at the time, which is what holding a world is for.
        </p>
      ) : (
        <div className="flex flex-col gap-4">
          <p className="text-[13px] text-ember">
            {plural(total, "save is", "saves are")} waiting on a decision across{" "}
            {plural(worlds.length, "world", "worlds")}.
          </p>
          {worlds.map((w) => (
            <section key={w.worldId} className="flex flex-col gap-2">
              <h3 className="font-mono text-[10px] uppercase tracking-[0.12em] text-gold">
                {w.name || "world #" + w.worldId} · {w.entries.length}
              </h3>
              <HistoryCard>
                {w.entries.map((e) => (
                  <HistoryRow key={e.versionId} entry={e} showWorld={false} />
                ))}
              </HistoryCard>
            </section>
          ))}
          <div className="rounded-panel border border-dashed border-edge bg-well px-[18px] py-3 text-[12.5px] text-mist">
            Nothing is lost while this is unresolved: a flagged save is kept whatever else is
            pruned. Choosing which one becomes the world&apos;s current save is done on the sync
            service by an administrator — this window deliberately cannot, because the companion
            holds a player&apos;s credential and moving a world&apos;s head decides for everyone.
            {serverUrl ? (
              <>
                {" "}
                The service is at <span className="font-mono text-[11px]">{serverUrl}</span>.
              </>
            ) : null}
          </div>
        </div>
      )}
    </HistoryFrame>
  );
}

/** How many conflicts a history holds — the tab badge's number, derived
 * from the same list the view renders so the two cannot disagree. */
export function conflictCount(history: History | undefined): number {
  return (history?.entries ?? []).filter((e) => e.conflict).length;
}
