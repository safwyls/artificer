import { useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "../../lib/api";
import { useAuth } from "../../lib/auth";
import { FtNote, FtPanel, fkLogTone } from "../../components/flametender/FtPanel";
import { cn } from "../../lib/utils";

const TAILS = [100, 300, 1000, 2000] as const;

export function FtLogs() {
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
      <div className="flametender min-h-full font-ftbody">
        <div className="mx-auto max-w-[1180px] p-4 lg:p-7">
          <FtPanel title="Server log">
            <p className="text-sm text-ft-lichen">
              The log can carry chat and player identities, so it needs the power permission. Ask a steward for the
              grant.
            </p>
          </FtPanel>
        </div>
      </div>
    );
  }

  return (
    <div className="flametender min-h-full font-ftbody">
      <div className="mx-auto max-w-[1180px] p-4 lg:p-7">
        <FtPanel
          title="Server log"
          meta={
            <span className="flex items-center gap-3">
              <label className="flex items-center gap-1.5">
                tail
                <select
                  value={tail}
                  onChange={(e) => setTail(Number(e.target.value))}
                  className="rounded-sm border border-ft-edge bg-ft-void px-1.5 py-0.5 font-mono text-xs text-ft-bone"
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
                  follow ? "border-ft-flamedim text-ft-flame" : "border-ft-edge text-ft-lichen hover:text-ft-bone",
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
            className="max-h-[65vh] overflow-y-auto rounded bg-ft-void px-3.5 py-2.5 font-mono text-xs leading-[1.75]"
          >
            {logsQuery.isLoading && <span className="text-ft-lichen">Fetching the log…</span>}
            {logsQuery.isError && (
              <span className="text-ft-spore">
                The log is out of reach. The flameagent serves it for supervised servers; a stopped agent or a
                companion-mode setup has nothing to tail.
              </span>
            )}
            {lines.map((line, i) => (
              <div key={i} className={fkLogTone(line)}>
                {line}
              </div>
            ))}
            {!logsQuery.isLoading && !logsQuery.isError && lines.length === 0 && (
              <span className="text-ft-lichen">Nothing in the log yet.</span>
            )}
          </div>
          <FtNote>
            Tailing the supervised process's output — the same stream as enshrouded_server.log. Join and leave lines
            light in flame; errors in spore red.
          </FtNote>
        </FtPanel>
      </div>
    </div>
  );
}
