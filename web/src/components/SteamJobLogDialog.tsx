import { useEffect, useRef } from "react";
import { Download } from "lucide-react";
import type { SteamJob } from "../lib/api";
import { cn } from "../lib/utils";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "./ui/dialog";

/** Elapsed time as "38s" / "4m 12s"; jobs never run long enough for hours
 * (the agent kills them at 45 minutes). */
function duration(startedAt: string, finishedAt?: string) {
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now();
  const s = Math.max(0, Math.round((end - new Date(startedAt).getTime()) / 1000));
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`;
}

/**
 * The SteamCMD output of the agent's update job, rendered from the tail
 * the job status poll already carries — no extra endpoint, and it keeps
 * updating live while the job runs. Follows the same log-viewer contract
 * as ContainerLogsDialog: pinned to the bottom until the user scrolls up.
 */
export function SteamJobLogDialog({
  job,
  open,
  onOpenChange,
}: {
  job: SteamJob | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const scrollRef = useRef<HTMLPreElement>(null);
  const pinnedRef = useRef(true);
  const lines = job?.log ?? [];
  const running = job?.state === "running";

  // Stick to the bottom on new content — unless the user has scrolled up
  // to read something, in which case yanking them down would be hostile.
  useEffect(() => {
    const el = scrollRef.current;
    if (el && pinnedRef.current) el.scrollTop = el.scrollHeight;
  }, [lines.length, open]);

  // Re-pin whenever the dialog opens fresh.
  useEffect(() => {
    if (open) pinnedRef.current = true;
  }, [open]);

  const download = () => {
    const blob = new Blob([lines.join("\n") + "\n"], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `steamcmd-${job?.id ?? "job"}.log`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] w-[calc(100vw-2rem)] max-w-4xl flex-col">
        <DialogHeader>
          <DialogTitle className="flex flex-wrap items-center gap-x-3 gap-y-1">
            SteamCMD update
            {job && (
              <span className="flex items-center gap-1.5 font-mono text-xs font-normal text-fk-bone/45">
                {job.state === "running"
                  ? `running · ${duration(job.startedAt)}`
                  : `${job.state} · ${duration(job.startedAt, job.finishedAt)}`}
                <span
                  className={cn(
                    "h-1.5 w-1.5 rounded-full",
                    running ? "animate-pulse bg-fk-ok" : job.state === "done" ? "bg-fk-ok" : "bg-fk-spore",
                  )}
                  title={running ? "Live" : job.state}
                />
              </span>
            )}
          </DialogTitle>
        </DialogHeader>

        {job?.error && (
          <p className="rounded-lg bg-fk-spore/10 px-3 py-2 font-mono text-xs text-fk-spore">{job.error}</p>
        )}

        <div className="flex items-center gap-2">
          <button
            className="flex items-center gap-1.5 rounded-lg border border-fk-edge px-2.5 py-1.5 text-xs font-semibold text-fk-bone/60 hover:bg-fk-bone/5 disabled:opacity-50"
            disabled={lines.length === 0}
            onClick={download}
          >
            <Download className="h-3.5 w-3.5" />
            Download
          </button>
          <span className="text-xs text-fk-bone/40">Last {lines.length} lines from the agent</span>
        </div>

        <pre
          ref={scrollRef}
          onScroll={(e) => {
            const el = e.currentTarget;
            pinnedRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
          }}
          className="min-h-48 flex-1 overflow-auto whitespace-pre-wrap break-all rounded-xl bg-fk-void p-4 font-mono text-[11px] leading-relaxed text-fk-bone/80"
        >
          {lines.length > 0
            ? lines.join("\n")
            : job
              ? "No output yet."
              : "No update has run since the agent started."}
        </pre>
      </DialogContent>
    </Dialog>
  );
}
