import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

/**
 * Live custody updates: the stream says "a world changed", the page
 * re-reads the truth. A dropped frame costs one stale render at most, and
 * the slow poll underneath is the belt to these suspenders — it also
 * covers a proxy that eats event streams entirely.
 *
 * Everything custody-shaped is invalidated on an event rather than patched
 * from it, because the event carries no payload worth trusting over a
 * re-read.
 */
export const POLL_MS = 20_000;

/** How long to wait before replacing an EventSource that died for good. */
const REOPEN_MS = 5_000;

export function useCustodyStream(): { live: boolean } {
  const queryClient = useQueryClient();
  const [live, setLive] = useState(false);

  useEffect(() => {
    let closed = false;
    let es: EventSource | null = null;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const open = () => {
      if (closed) return;
      try {
        es = new EventSource("/api/sync/events");
      } catch {
        // No EventSource (or the browser refused): the poll carries it.
        return;
      }
      es.addEventListener("ready", () => setLive(true));
      es.addEventListener("custody", () => {
        setLive(true);
        queryClient.invalidateQueries({ queryKey: ["worlds"] });
        queryClient.invalidateQueries({ queryKey: ["world"] });
      });
      es.onerror = () => {
        setLive(false);
        // EventSource reconnects on its own; if the connection is gone for
        // good (server restart, tunnel drop) a fresh one is cheap.
        if (es && es.readyState === EventSource.CLOSED) {
          es.close();
          es = null;
          timer = setTimeout(open, REOPEN_MS);
        }
      };
    };
    open();

    return () => {
      closed = true;
      clearTimeout(timer);
      es?.close();
    };
  }, [queryClient]);

  return { live };
}
