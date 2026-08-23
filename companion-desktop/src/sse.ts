// sse.ts — consumes GET /api/events from Electron main. companiond's SSE
// stream is token-gated (docs/companion-api-surface.md, companion/server.go
// Phase 1), so the renderer's EventSource can't be used directly there
// (no header support); main.ts can use `fetch` with an Authorization
// header and read the body as a stream instead, which this module wraps.
//
// Events carry no payload (`event: ready` once, `event: changed` per
// nudge, `: keepalive` every 25s) — the caller re-fetches /api/state on
// any `changed` event, same contract the web renderer follows.

export type SSEEventName = "ready" | "changed" | string;

export interface SSEHandle {
  stop(): void;
}

/**
 * Opens the SSE stream and invokes `onEvent` for each named event line.
 * Reconnects with backoff on stream error/close, until `stop()` is
 * called. Uses the global `fetch` (available in Electron's main process
 * since Chromium's network stack backs it), not EventSource, so the
 * bearer token can go on the request as a normal header.
 */
export function watchEvents(
  baseUrl: string,
  token: string,
  onEvent: (name: SSEEventName) => void,
  onLog?: (msg: string) => void
): SSEHandle {
  let stopped = false;
  let controller: AbortController | null = null;

  async function loop() {
    let backoffMs = 500;
    while (!stopped) {
      controller = new AbortController();
      try {
        const res = await fetch(`${baseUrl}/api/events`, {
          headers: { Authorization: `Bearer ${token}` },
          signal: controller.signal,
        });
        if (!res.ok || !res.body) {
          throw new Error(`SSE connect failed: ${res.status}`);
        }
        backoffMs = 500; // reset once connected
        const reader = res.body.getReader();
        const decoder = new TextDecoder();
        let buf = "";
        let currentEvent = "message";
        while (!stopped) {
          const { value, done } = await reader.read();
          if (done) break;
          buf += decoder.decode(value, { stream: true });
          let idx;
          while ((idx = buf.indexOf("\n")) !== -1) {
            const line = buf.slice(0, idx);
            buf = buf.slice(idx + 1);
            if (line.startsWith(":")) continue; // keepalive comment
            if (line.startsWith("event:")) {
              currentEvent = line.slice(6).trim();
            } else if (line === "") {
              onEvent(currentEvent);
              currentEvent = "message";
            }
          }
        }
      } catch (err) {
        if (stopped) return;
        onLog?.(`SSE stream error, reconnecting in ${backoffMs}ms: ${String(err)}`);
      }
      if (stopped) return;
      await new Promise((r) => setTimeout(r, backoffMs));
      backoffMs = Math.min(backoffMs * 2, 10_000);
    }
  }

  loop();

  return {
    stop() {
      stopped = true;
      controller?.abort();
    },
  };
}
