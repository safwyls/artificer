import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api, ApiError } from "../lib/api";
import { cn } from "../lib/utils";

/** Coarse "in 9h 12m" countdown, re-rendered each half-minute. */
function useCountdown(to: string | undefined): string | null {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(t);
  }, []);
  if (!to) return null;
  const mins = Math.round((new Date(to).getTime() - now) / 60_000);
  if (mins <= 0) return "any moment now";
  const d = Math.floor(mins / 1440);
  const h = Math.floor((mins % 1440) / 60);
  const m = mins % 60;
  if (d > 0) return `in ${d}d ${h}h`;
  if (h > 0) return `in ${h}h ${m}m`;
  return `in ${m}m`;
}

/**
 * The public, no-login status card. Standalone route with none of the app
 * chrome: this page is what gets pinned in a community's Discord, so it
 * has one job — is the server up, how full is it, when's the restart.
 */
export function PublicStatus() {
  const { token } = useParams();

  const statusQuery = useQuery({
    queryKey: ["public-status", token],
    queryFn: () => api.publicStatus(token ?? ""),
    refetchInterval: 30_000,
    retry: false,
  });

  const status = statusQuery.data;
  const countdown = useCountdown(status?.nextRestartAt);
  const unknown = statusQuery.isError && statusQuery.error instanceof ApiError && statusQuery.error.status === 404;

  return (
    <div className="app-min-height flex flex-col items-center justify-center bg-wk-bg p-6">
      <main className="clip-notch-lg w-full max-w-md border border-wk-edge bg-wk-panel px-8 py-10 text-center shadow-sm">
        {unknown ? (
          <>
            <h1 className="font-display text-2xl font-extrabold">No status here</h1>
            <p className="mt-2 text-sm text-wk-parchment/60">
              This status page doesn't exist — the link may have been revoked.
            </p>
          </>
        ) : statusQuery.isLoading ? (
          <p className="text-sm text-wk-parchment/50">Loading…</p>
        ) : statusQuery.isError ? (
          <p className="text-sm text-wk-parchment/60">Status is unavailable right now. Refresh to try again.</p>
        ) : status ? (
          <>
            <h1 className="break-words font-display text-3xl font-extrabold">{status.name}</h1>

            <p
              className={cn(
                "mt-4 inline-flex items-center gap-2 rounded-full px-4 py-1.5 text-sm font-bold",
                status.online ? "bg-wk-ok/15 text-wk-ok" : "bg-wk-parchment/10 text-wk-parchment/50",
              )}
            >
              <span className={cn("h-2.5 w-2.5 rounded-full", status.online ? "bg-wk-ok" : "bg-wk-parchment/30")} />
              {status.online ? "Online" : "Offline"}
            </p>

            {status.online && status.players !== undefined && (
              <div className="mt-6">
                <p className="font-mono text-5xl font-semibold tabular-nums">
                  {status.players}
                  {status.maxPlayers !== undefined && (
                    <span className="text-2xl text-wk-parchment/35">/{status.maxPlayers}</span>
                  )}
                </p>
                <p className="mt-1 text-xs uppercase tracking-widest text-wk-parchment/40">players online</p>
              </div>
            )}

            {countdown && (
              <p className="mt-6 text-sm text-wk-parchment/60">
                Next scheduled restart <span className="font-mono font-semibold text-wk-parchment">{countdown}</span>
              </p>
            )}
          </>
        ) : null}
      </main>
      <p className="mt-4 font-mono text-[11px] text-wk-parchment/35">powered by Flamekeeper</p>
    </div>
  );
}
