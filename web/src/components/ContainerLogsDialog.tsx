import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Download, Pause, Play } from "lucide-react";
import { api, errorDetail } from "../lib/api";
import { cn } from "../lib/utils";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "./ui/dialog";
import { Select } from "./ui/select";

const TAILS = [200, 500, 1000];

/**
 * Inherited from palcon, where the game logged every successful REST call
 * and the dashboard's own polling drowned the log in them. Enshrouded has
 * no REST interface, so nothing matches this today — kept because the
 * filter costs one regex and a companion-mode server running some other
 * build may still produce these lines.
 */
const REST_NOISE = /REST accessed endpoint \S+ OK\s*$/;

/**
 * The container's recent log, polled while open. Follows the standard
 * log-viewer contract: pinned to the bottom until the user scrolls up,
 * and a pause button when they want the text to hold still.
 */
export function ContainerLogsDialog({
  serverId,
  containerName,
  open,
  onOpenChange,
}: {
  serverId: number;
  containerName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [tail, setTail] = useState(500);
  const [paused, setPaused] = useState(false);
  const [hideRest, setHideRest] = useState(true);
  const scrollRef = useRef<HTMLPreElement>(null);
  const pinnedRef = useRef(true);

  const logsQuery = useQuery({
    queryKey: ["container-logs", serverId, tail],
    queryFn: () => api.containerLogs(serverId, tail),
    enabled: open && !paused,
    refetchInterval: 5000,
    retry: false,
  });

  // Stick to the bottom on new content — unless the user has scrolled up
  // to read something, in which case yanking them down would be hostile.
  useEffect(() => {
    const el = scrollRef.current;
    if (el && pinnedRef.current) el.scrollTop = el.scrollHeight;
  }, [logsQuery.dataUpdatedAt]);

  // Re-pin whenever the dialog opens fresh.
  useEffect(() => {
    if (open) {
      pinnedRef.current = true;
      setPaused(false);
    }
  }, [open]);

  // Filtered client-side so nothing is hidden silently: the toggle shows
  // how many lines it's holding back, and Download saves what's on screen.
  const allLines = logsQuery.data?.lines ?? [];
  const lines = hideRest ? allLines.filter((l) => !REST_NOISE.test(l)) : allLines;
  const hiddenCount = allLines.length - lines.length;

  const download = () => {
    const blob = new Blob([lines.join("\n") + "\n"], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${containerName || "container"}-logs.txt`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] w-[calc(100vw-2rem)] max-w-4xl flex-col">
        <DialogHeader>
          <DialogTitle className="flex flex-wrap items-center gap-x-3 gap-y-1">
            Container logs
            <span className="flex items-center gap-1.5 font-mono text-xs font-normal text-fk-bone/45">
              {containerName}
              <span
                className={cn(
                  "h-1.5 w-1.5 rounded-full",
                  paused ? "bg-fk-bone/25" : "animate-pulse bg-fk-ok",
                )}
                title={paused ? "Paused" : "Refreshing every 5s"}
              />
            </span>
          </DialogTitle>
        </DialogHeader>

        <div className="flex items-center gap-2">
          <Select value={String(tail)} onChange={(e) => setTail(Number(e.target.value))} className="w-36">
            {TAILS.map((n) => (
              <option key={n} value={n}>
                Last {n} lines
              </option>
            ))}
          </Select>
          <button
            className="flex items-center gap-1.5 rounded-lg border border-fk-edge px-2.5 py-1.5 text-xs font-semibold text-fk-bone/60 hover:bg-fk-bone/5"
            onClick={() => setPaused((p) => !p)}
          >
            {paused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
            {paused ? "Resume" : "Pause"}
          </button>
          <button
            className="flex items-center gap-1.5 rounded-lg border border-fk-edge px-2.5 py-1.5 text-xs font-semibold text-fk-bone/60 hover:bg-fk-bone/5 disabled:opacity-50"
            disabled={lines.length === 0}
            onClick={download}
          >
            <Download className="h-3.5 w-3.5" />
            Download
          </button>
          <label className="ml-auto flex cursor-pointer items-center gap-1.5 text-xs text-fk-bone/60">
            <input
              type="checkbox"
              checked={hideRest}
              onChange={(e) => setHideRest(e.target.checked)}
              className="h-3.5 w-3.5 accent-fk-spore"
            />
            Hide REST polling
            {hideRest && hiddenCount > 0 && <span className="font-mono text-fk-bone/35">· {hiddenCount} hidden</span>}
          </label>
        </div>

        <pre
          ref={scrollRef}
          onScroll={(e) => {
            const el = e.currentTarget;
            pinnedRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
          }}
          className="min-h-48 flex-1 overflow-auto whitespace-pre-wrap break-all rounded-xl bg-fk-void p-4 font-mono text-[11px] leading-relaxed text-fk-bone/80"
        >
          {logsQuery.isError
            ? `Could not read logs: ${errorDetail(logsQuery.error) ?? "unknown error"}`
            : lines.length > 0
              ? lines.join("\n")
              : logsQuery.isFetching
                ? "Loading…"
                : "No log output."}
        </pre>
      </DialogContent>
    </Dialog>
  );
}
