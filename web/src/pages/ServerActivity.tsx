import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ScrollText, Users } from "lucide-react";
import { api, type PlayerEvent } from "../lib/api";
import { useAuth } from "../lib/auth";
import { initials, playerColor } from "../lib/palette";
import { cn } from "../lib/utils";

const RANGES = [
  { hours: 24, label: "24h" },
  { hours: 48, label: "48h" },
  { hours: 168, label: "7d" },
];

function fmtDuration(ms: number): string {
  const mins = Math.round(ms / 60_000);
  if (mins < 1) return "<1m";
  const h = Math.floor(mins / 60);
  const m = mins % 60;
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}

interface FeedRow {
  event: PlayerEvent;
  /** For a leave: how long the session it closed lasted (when its join is
   * in range). For a join with no leave yet: undefined. */
  sessionMs?: number;
  ongoing?: boolean;
}

interface PlayerSummary {
  name: string;
  userId: string;
  totalMs: number;
  sessions: number;
  online: boolean;
}

/** Pairs joins with leaves per player (events arrive newest-first) to hang
 * a session duration on each leave, plus per-player playtime totals. A
 * session still open at "now" counts as ongoing. */
function analyze(events: PlayerEvent[], rangeStart: Date, now: Date) {
  const asc = [...events].reverse();
  const open = new Map<string, number>(); // userId → join time (ms)
  const sessionByEventId = new Map<number, { ms: number; ongoing: boolean }>();
  const totals = new Map<string, PlayerSummary>();

  const summary = (e: PlayerEvent): PlayerSummary => {
    let s = totals.get(e.userId);
    if (!s) {
      s = { name: e.name, userId: e.userId, totalMs: 0, sessions: 0, online: false };
      totals.set(e.userId, s);
    }
    s.name = e.name; // latest display name wins
    return s;
  };

  for (const e of asc) {
    const t = new Date(e.ts).getTime();
    const s = summary(e);
    if (e.event === "join") {
      open.set(e.userId, t);
    } else {
      // A leave without a visible join means the join predates the window;
      // clamp the session to the range start rather than inventing time.
      const start = open.get(e.userId) ?? rangeStart.getTime();
      open.delete(e.userId);
      const ms = Math.max(0, t - start);
      s.totalMs += ms;
      s.sessions += 1;
      sessionByEventId.set(e.id, { ms, ongoing: false });
    }
  }
  for (const [userId, start] of open) {
    const s = totals.get(userId);
    if (s) {
      s.totalMs += Math.max(0, now.getTime() - start);
      s.sessions += 1;
      s.online = true;
    }
  }

  const rows: FeedRow[] = events.map((e) => {
    const hit = sessionByEventId.get(e.id);
    return { event: e, sessionMs: hit?.ms, ongoing: hit?.ongoing };
  });
  const summaries = [...totals.values()].sort((a, b) => b.totalMs - a.totalMs);
  return { rows, summaries };
}

/** Action chips colored by verb family, so a scan of the table separates
 * power events from config edits without reading every row. */
function actionTone(action: string): string {
  if (action.startsWith("power-") || action === "scheduled-restart" || action === "shutdown")
    return "border-brand-red/30 bg-brand-red/10 text-brand-red";
  if (action === "save-world") return "border-pal-green/40 bg-pal-green/10 text-pal-green";
  if (action === "broadcast") return "border-pal-blue/40 bg-pal-blue/10 text-pal-blue";
  if (action.startsWith("config-") || action.startsWith("schedule-") || action.startsWith("discord-"))
    return "border-brand-amber/50 bg-brand-amber/10 text-ink/70";
  return "border-ink/15 bg-ink/5 text-ink/60";
}

function dayLabel(d: Date, now: Date): string {
  const day = d.toDateString();
  if (day === now.toDateString()) return "Today";
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (day === yesterday.toDateString()) return "Yesterday";
  return d.toLocaleDateString([], { weekday: "long", month: "short", day: "numeric" });
}

export function ServerActivity() {
  const { serverID } = useParams();
  const id = Number(serverID);
  const { isAdmin } = useAuth();
  const [hours, setHours] = useState(48);

  const activityQuery = useQuery({
    queryKey: ["server-activity", id, hours],
    queryFn: () => api.serverActivity(id, hours),
    refetchInterval: 60_000,
  });
  const auditQuery = useQuery({
    queryKey: ["server-audit", id],
    queryFn: () => api.serverAudit(id),
    enabled: isAdmin,
    refetchInterval: 60_000,
  });

  const now = useMemo(() => new Date(), [activityQuery.dataUpdatedAt]); // eslint-disable-line react-hooks/exhaustive-deps
  const { rows, summaries } = useMemo(
    () => analyze(activityQuery.data?.events ?? [], new Date(now.getTime() - hours * 3_600_000), now),
    [activityQuery.data, hours, now],
  );

  // Group feed rows (already newest-first) by calendar day.
  const days = useMemo(() => {
    const out: { label: string; rows: FeedRow[] }[] = [];
    for (const row of rows) {
      const label = dayLabel(new Date(row.event.ts), now);
      if (out.length === 0 || out[out.length - 1].label !== label) out.push({ label, rows: [] });
      out[out.length - 1].rows.push(row);
    }
    return out;
  }, [rows, now]);

  return (
    <div className="pb-24">
      <header className="sticky top-0 z-10 hidden items-center justify-between border-b border-ink/10 bg-paper px-8 py-6 lg:flex">
        <div>
          <h1 className="font-display text-2xl font-extrabold">Activity</h1>
          <p className="text-sm text-ink/60">Who's been on, and what changed</p>
        </div>
      </header>

      <div className="mx-auto max-w-5xl space-y-4 p-4 lg:space-y-6 lg:p-8">
        <section className="rounded-xl border border-ink/10 bg-white">
          <div className="flex flex-wrap items-center justify-between gap-2 border-b border-ink/5 px-5 py-4">
            <div className="flex items-center gap-2">
              <Users className="h-4 w-4 text-pal-green" />
              <h2 className="font-display text-base font-bold">Player activity</h2>
            </div>
            <div className="flex gap-1 rounded-lg border border-ink/10 p-0.5">
              {RANGES.map((r) => (
                <button
                  key={r.hours}
                  onClick={() => setHours(r.hours)}
                  className={cn(
                    "rounded-md px-2.5 py-1 font-mono text-xs font-semibold transition-colors",
                    hours === r.hours ? "bg-ink text-paper" : "text-ink/45 hover:text-ink",
                  )}
                >
                  {r.label}
                </button>
              ))}
            </div>
          </div>

          {activityQuery.isError && (
            <p className="px-5 py-6 text-sm text-ink/60">Activity could not be loaded. Refresh to try again.</p>
          )}
          {activityQuery.isLoading && <p className="px-5 py-6 text-sm text-muted-foreground">Loading…</p>}

          {activityQuery.data && summaries.length > 0 && (
            <div className="flex flex-wrap gap-2 border-b border-ink/5 px-5 py-3">
              {summaries.map((s) => (
                <span
                  key={s.userId}
                  className="flex items-center gap-1.5 rounded-full border border-ink/10 py-1 pl-1 pr-2.5 text-xs"
                  title={`${s.sessions} session${s.sessions === 1 ? "" : "s"} in the last ${hours}h`}
                >
                  <span
                    className="flex h-5 w-5 items-center justify-center rounded-full text-[9px] font-bold text-paper"
                    style={{ backgroundColor: playerColor(s.userId) }}
                  >
                    {initials(s.name)}
                  </span>
                  <span className="font-semibold">{s.name}</span>
                  <span className="font-mono text-ink/45">{fmtDuration(s.totalMs)}</span>
                  {s.online && <span className="h-1.5 w-1.5 rounded-full bg-pal-green" title="online now" />}
                </span>
              ))}
            </div>
          )}

          {activityQuery.data && rows.length === 0 && (
            <p className="px-5 py-6 text-sm text-ink/60">
              No joins or leaves seen in the last {hours} hours. Events are recorded from when this Palcon
              version started watching the server.
            </p>
          )}

          {days.map((day) => (
            <div key={day.label}>
              <p className="border-b border-ink/5 bg-ink/[0.02] px-5 py-1.5 text-xs font-semibold uppercase tracking-wide text-ink/35">
                {day.label}
              </p>
              <ul className="divide-y divide-ink/5">
                {day.rows.map(({ event: e, sessionMs }) => (
                  <li key={e.id} className="flex items-center gap-3 px-5 py-2 text-sm">
                    <span className="w-16 shrink-0 font-mono text-xs tabular-nums text-ink/40">
                      {new Date(e.ts).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
                    </span>
                    <span
                      className={cn("h-2 w-2 shrink-0 rounded-full", e.event === "join" ? "bg-pal-green" : "bg-ink/25")}
                    />
                    <span className="min-w-0 flex-1 truncate">
                      <span className="font-semibold">{e.name}</span>{" "}
                      <span className="text-ink/50">{e.event === "join" ? "joined" : "left"}</span>
                      {e.event === "leave" && sessionMs !== undefined && (
                        <span className="font-mono text-xs text-ink/40"> · {fmtDuration(sessionMs)} session</span>
                      )}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </section>

        {isAdmin && (
          <section className="rounded-xl border border-ink/10 bg-white">
            <div className="flex items-center gap-2 border-b border-ink/5 px-5 py-4">
              <ScrollText className="h-4 w-4 text-brand-amber" />
              <h2 className="font-display text-base font-bold">Admin actions</h2>
            </div>
            {auditQuery.isError && (
              <p className="px-5 py-6 text-sm text-ink/60">The audit log could not be loaded.</p>
            )}
            {auditQuery.data && auditQuery.data.entries.length === 0 && (
              <p className="px-5 py-6 text-sm text-ink/60">
                Nothing yet — management actions (power, saves, broadcasts, settings and automation changes)
                will be recorded here.
              </p>
            )}
            {auditQuery.data && auditQuery.data.entries.length > 0 && (
              <ul className="divide-y divide-ink/5">
                {auditQuery.data.entries.map((e) => (
                  <li key={e.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-5 py-2.5 text-sm">
                    <span className="w-32 shrink-0 font-mono text-xs tabular-nums text-ink/40">
                      {new Date(e.ts).toLocaleString([], {
                        month: "short",
                        day: "numeric",
                        hour: "2-digit",
                        minute: "2-digit",
                      })}
                    </span>
                    <span className={cn("rounded-md border px-1.5 py-0.5 font-mono text-[11px]", actionTone(e.action))}>
                      {e.action}
                    </span>
                    <span className="font-semibold">{e.username}</span>
                    {e.detail && <span className="min-w-0 flex-1 truncate font-mono text-xs text-ink/45">{e.detail}</span>}
                  </li>
                ))}
              </ul>
            )}
          </section>
        )}
      </div>
    </div>
  );
}
