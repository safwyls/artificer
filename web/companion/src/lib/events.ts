import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { apiUrl, authHeaders } from "./runtime";

/**
 * The companion's push channel: `GET /api/events`, server-sent events.
 *
 * The stream carries no payload — `event: ready` once, then `event:
 * changed` per nudge — and the client re-reads `GET /api/state`. That is
 * what makes a dropped or coalesced event harmless, and it is why this
 * can be layered on top of the existing poll rather than replacing it:
 * the poll is the fallback, and it is still correct on its own.
 *
 * `fetch` plus a ReadableStream rather than `EventSource`, because
 * EventSource cannot send an Authorization header and the daemon under
 * the Electron shell requires one.
 */

/** One parsed SSE frame. Comments (keepalives) yield nothing. */
export function parseFrame(frame: string): { event: string } | null {
  let event = "";
  let sawData = false;
  for (const raw of frame.split("\n")) {
    const line = raw.replace(/\r$/, "");
    if (!line || line.startsWith(":")) continue;
    if (line.startsWith("event:")) event = line.slice(6).trim();
    else if (line.startsWith("data:")) sawData = true;
  }
  if (!event && !sawData) return null;
  return { event: event || "message" };
}

/**
 * Split a buffer into complete frames, returning the frames and whatever
 * partial frame is left over. A chunk boundary can land anywhere, so the
 * remainder has to survive to the next read.
 */
export function splitFrames(buffer: string): { frames: string[]; rest: string } {
  const parts = buffer.split(/\n\n|\r\n\r\n/);
  return { frames: parts.slice(0, -1), rest: parts[parts.length - 1] ?? "" };
}

/** How long the page waits between reconnect attempts, and how slowly it
 * polls once the stream is carrying the news instead. */
export const RECONNECT_MS = 5_000;
export const LIVE_POLL_MS = 30_000;

/**
 * Subscribes to the stream for as long as the page is mounted and
 * re-reads state on every nudge. Returns whether the stream is currently
 * carrying — the caller slows its poll down when it is, and back up when
 * it is not.
 *
 * A daemon too old to serve `/api/events` answers 404, which is not an
 * error worth showing anyone: the poll was already keeping the page
 * current, and it keeps doing so.
 */
export function useLiveUpdates(enabled = true): boolean {
  const queryClient = useQueryClient();
  const [live, setLive] = useState(false);

  useEffect(() => {
    if (!enabled || typeof AbortController === "undefined") return;
    let stopped = false;
    let controller: AbortController | null = null;
    let timer: ReturnType<typeof setTimeout> | null = null;

    const connect = async () => {
      if (stopped) return;
      controller = new AbortController();
      try {
        const res = await fetch(apiUrl("/api/events"), {
          headers: { ...authHeaders(), Accept: "text/event-stream" },
          signal: controller.signal,
        });
        if (!res.ok || !res.body) throw new Error(String(res.status));
        const reader = res.body.getReader();
        const decode = new TextDecoder();
        let buffer = "";
        setLive(true);
        for (;;) {
          const { done, value } = await reader.read();
          if (done || stopped) break;
          buffer += decode.decode(value, { stream: true });
          const { frames, rest } = splitFrames(buffer);
          buffer = rest;
          for (const frame of frames) {
            const parsed = parseFrame(frame);
            // `ready` only says the stream is open; `changed` is the news.
            if (parsed?.event === "changed") {
              queryClient.invalidateQueries({ queryKey: ["state"] });
            }
          }
        }
      } catch {
        // Every failure mode is the same failure: no stream, so the poll
        // carries the page. Nothing to report.
      } finally {
        setLive(false);
        if (!stopped) timer = setTimeout(connect, RECONNECT_MS);
      }
    };

    connect();
    return () => {
      stopped = true;
      controller?.abort();
      if (timer) clearTimeout(timer);
    };
  }, [enabled, queryClient]);

  return live;
}
