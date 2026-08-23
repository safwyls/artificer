import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { LIVE_POLL_MS, parseFrame, splitFrames, useLiveUpdates } from "./events";

afterEach(() => {
  delete window.companion;
  vi.restoreAllMocks();
});

describe("the SSE wire format", () => {
  it("reads the event name off a frame", () => {
    expect(parseFrame("event: changed\ndata:\n")?.event).toBe("changed");
    expect(parseFrame("event: ready\n")?.event).toBe("ready");
  });

  // The 25s keepalive is a bare comment. Treating it as an event would
  // re-read state every 25 seconds forever.
  it("ignores a keepalive comment", () => {
    expect(parseFrame(": keepalive")).toBeNull();
    expect(parseFrame("")).toBeNull();
  });

  it("survives CRLF and a nameless data frame", () => {
    expect(parseFrame("event: changed\r\ndata: {}\r\n")?.event).toBe("changed");
    expect(parseFrame("data: hi")?.event).toBe("message");
  });

  // A chunk boundary lands wherever the network puts it, so a partial
  // frame has to survive to the next read.
  it("keeps a partial frame back until the rest of it arrives", () => {
    const first = splitFrames("event: ready\n\nevent: chan");
    expect(first.frames).toEqual(["event: ready"]);
    expect(first.rest).toBe("event: chan");
    const second = splitFrames(first.rest + "ged\n\n");
    expect(second.frames).toEqual(["event: changed"]);
    expect(second.rest).toBe("");
  });
});

/** A stream that yields the given chunks and then stays open. */
function streamOf(chunks: string[]): Response {
  const encoder = new TextEncoder();
  let i = 0;
  const body = new ReadableStream<Uint8Array>({
    pull(controller) {
      if (i < chunks.length) controller.enqueue(encoder.encode(chunks[i++]));
      else controller.close();
    },
  });
  return new Response(body, { status: 200, headers: { "Content-Type": "text/event-stream" } });
}

const wrap = (client: QueryClient) =>
  function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };

describe("useLiveUpdates", () => {
  it("re-reads state on every changed event, and ignores ready", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      streamOf(["event: ready\n\n", ": keepalive\n\n", "event: changed\n\n"]),
    );
    const client = new QueryClient();
    const invalidate = vi.spyOn(client, "invalidateQueries");
    renderHook(() => useLiveUpdates(), { wrapper: wrap(client) });
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: ["state"] }));
    expect(invalidate).toHaveBeenCalledTimes(1);
  });

  it("carries the bearer token to the daemon's own address", async () => {
    window.companion = {
      baseUrl: "http://127.0.0.1:41234",
      token: "tok",
      pickFolder: async () => null,
      openPath: async () => {},
    };
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(streamOf([]));
    renderHook(() => useLiveUpdates(), { wrapper: wrap(new QueryClient()) });
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(fetchMock.mock.calls[0][0]).toBe("http://127.0.0.1:41234/api/events");
    expect((fetchMock.mock.calls[0][1] as RequestInit).headers).toMatchObject({
      Authorization: "Bearer tok",
      Accept: "text/event-stream",
    });
  });

  // A daemon too old to serve /api/events answers 404. That is not an
  // error worth showing anyone: the poll was already keeping the page
  // current and keeps doing so.
  it("stays quiet and un-live when the route is not there", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("", { status: 404 }));
    const { result } = renderHook(() => useLiveUpdates(), { wrapper: wrap(new QueryClient()) });
    await waitFor(() => expect(result.current).toBe(false));
  });

  it("stays un-live when the daemon cannot be reached at all", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("connection refused"));
    const { result } = renderHook(() => useLiveUpdates(), { wrapper: wrap(new QueryClient()) });
    await waitFor(() => expect(result.current).toBe(false));
  });

  it("does not connect at all when it is switched off", () => {
    const fetchMock = vi.spyOn(globalThis, "fetch");
    renderHook(() => useLiveUpdates(false), { wrapper: wrap(new QueryClient()) });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  // The poll is the fallback and stays correct on its own, so the live
  // heartbeat is slower than the poll rather than absent.
  it("keeps a heartbeat poll even while the stream carries", () => {
    expect(LIVE_POLL_MS).toBeGreaterThan(0);
  });
});
