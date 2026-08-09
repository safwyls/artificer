import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "../../lib/api";
import { useAuth } from "../../lib/auth";
import { WkNote, WkPanel, wkLogTone } from "../../components/wildskeeper/WkPanel";
import { cn } from "../../lib/utils";

const TAILS = [100, 300, 1000, 2000] as const;

export function WkLogs() {
  const { serverID } = useParams();
  const id = Number(serverID);
  const { can } = useAuth();
  const [tail, setTail] = useState<number>(300);
  const [follow, setFollow] = useState(true);
  const wellRef = useRef<HTMLDivElement>(null);

  const logsQuery = useQuery({
    queryKey: ["container-logs", id, tail],
    queryFn: () => api.containerLogs(id, tail),
    refetchInterval: follow ? 5_000 : false,
    retry: false,
    enabled: can("power"),
  });

  const lines = logsQuery.data?.lines ?? [];
  useEffect(() => {
    if (follow && wellRef.current) wellRef.current.scrollTop = wellRef.current.scrollHeight;
  }, [lines, follow]);

  if (!can("power")) {
    return (
      <div className="wildskeeper min-h-full font-wkbody">
        <div className="mx-auto max-w-[1180px] p-4 lg:p-7">
          <WkPanel title="Server log">
            <p className="text-sm text-wk-mist">
              The log can carry chat and player identities, so it needs the power permission. Ask a steward for the
              grant.
            </p>
          </WkPanel>
        </div>
      </div>
    );
  }

  return (
    <div className="wildskeeper min-h-full font-wkbody">
      <div className="mx-auto max-w-[1180px] p-4 lg:p-7">
        <WkPanel
          title="Server log"
          meta={
            <span className="flex items-center gap-3">
              <label className="flex items-center gap-1.5">
                tail
                <select
                  value={tail}
                  onChange={(e) => setTail(Number(e.target.value))}
                  className="rounded-sm border border-wk-edge bg-wk-ink px-1.5 py-0.5 font-mono text-xs text-wk-parchment"
                >
                  {TAILS.map((t) => (
                    <option key={t} value={t}>
                      {t}
                    </option>
                  ))}
                </select>
              </label>
              <button
                onClick={() => setFollow((f) => !f)}
                className={cn(
                  "rounded-sm border px-2 py-0.5 text-xs uppercase tracking-[0.08em] transition",
                  follow ? "border-wk-runedim text-wk-rune" : "border-wk-edge text-wk-mist hover:text-wk-parchment",
                )}
              >
                {follow ? "following" : "paused"}
              </button>
            </span>
          }
        >
          <div
            ref={wellRef}
            aria-live="polite"
            className="max-h-[65vh] overflow-y-auto rounded bg-wk-ink px-3.5 py-2.5 font-mono text-xs leading-[1.75]"
          >
            {logsQuery.isLoading && <span className="text-wk-mist">Fetching the log…</span>}
            {logsQuery.isError && (
              <span className="text-wk-ember">
                The log is out of reach. The palagent serves it for supervised servers; a stopped agent or a
                companion-mode setup has nothing to tail.
              </span>
            )}
            {lines.map((line, i) => (
              <div key={i} className={wkLogTone(line)}>
                {line}
              </div>
            ))}
            {!logsQuery.isLoading && !logsQuery.isError && lines.length === 0 && (
              <span className="text-wk-mist">Nothing in the log yet.</span>
            )}
          </div>
          <WkNote>
            Tailing the supervised process's output — the same stream as RSDragonwilds.log. Join and leave lines light
            in rune; errors in ember.
          </WkNote>
        </WkPanel>
      </div>
    </div>
  );
}
