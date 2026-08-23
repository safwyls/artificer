import { WifiOff } from "lucide-react";
import { fmtBytes, fmtClock } from "../lib/format";
import { Button } from "./ui/button";
import type { QueuedWork } from "../lib/types";

/**
 * The vault is unreachable. What the player needs to hear is that this
 * is survivable: the world they already hold stays theirs and stays
 * playable, and nothing they do is lost.
 */
export function OfflineBanner({
  error,
  retrying,
  onRetry,
}: {
  error?: string;
  retrying: boolean;
  onRetry: () => void;
}) {
  return (
    <div className="flex items-start gap-3.5 rounded-panel border border-ember/50 bg-panel px-[18px] py-[15px]">
      <WifiOff className="mt-0.5 h-[17px] w-[17px] flex-none text-ember" strokeWidth={1.8} aria-hidden />
      <div className="flex-1">
        <div className="text-[14.5px] text-parchment">Working offline — the vault is unreachable</div>
        <div className="mt-[3px] text-[12.5px] text-mist">
          Keep playing the world you already hold. The hold stands until the vault answers again.
        </div>
        {/* The service's own words, not a paraphrase of them. */}
        {error ? <div className="mt-1.5 font-mono text-[11px] text-ember">{error}</div> : null}
      </div>
      <Button onClick={onRetry} disabled={retrying}>
        {retrying ? "Retrying…" : "Retry now"}
      </Button>
    </div>
  );
}

/**
 * Work waiting to go up. The engine records none today — a checkout, a
 * checkpoint and a check-in each either reach the vault now or fail now,
 * and the failure is reported rather than retried — so this section is
 * drawn only when the queue is actually non-empty. Rendering an empty
 * "Queued to send · 0" would be inventing a manifest the companion never
 * kept, which is the one thing the offline screen must not do.
 */
export function QueuedToSend({ queue }: { queue: QueuedWork[] }) {
  if (!queue.length) return null;
  return (
    <section className="flex flex-col gap-2">
      <h2 className="font-mono text-[10px] uppercase tracking-[0.12em] text-mist">
        Queued to send · {queue.length}
      </h2>
      <div className="overflow-hidden rounded-panel border border-edge bg-panel text-[13px] [&>*+*]:border-t [&>*+*]:border-edge">
        {queue.map((q, i) => (
          <div key={`${q.worldId}:${q.time}:${i}`} className="flex items-center gap-3.5 px-[18px] py-[11px]">
            <span className="w-[54px] font-mono text-[11px] text-mist">{fmtClock(q.time)}</span>
            <span className="text-parchment">
              {q.what} {q.worldName ? `— ${q.worldName}` : `— world #${q.worldId}`}
            </span>
            <span className="ml-auto font-mono text-[11px] text-mist">{fmtBytes(q.size) || "—"}</span>
          </div>
        ))}
      </div>
    </section>
  );
}
