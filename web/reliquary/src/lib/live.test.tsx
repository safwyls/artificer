import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useCustodyStream } from "./live";

// A stand-in for the browser's EventSource, so a test can push the events
// the server would and watch what the app does with them.
class FakeEventSource {
  static instances: FakeEventSource[] = [];
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 2;
  readyState = 1;
  onerror: (() => void) | null = null;
  listeners: Record<string, (() => void)[]> = {};
  closed = false;
  constructor(public url: string) {
    FakeEventSource.instances.push(this);
  }
  addEventListener(name: string, fn: () => void) {
    (this.listeners[name] ??= []).push(fn);
  }
  emit(name: string) {
    for (const fn of this.listeners[name] ?? []) fn();
  }
  close() {
    this.closed = true;
    this.readyState = 2;
  }
}

let client: QueryClient;
const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={client}>{children}</QueryClientProvider>
);

describe("useCustodyStream", () => {
  beforeEach(() => {
    client = new QueryClient();
    FakeEventSource.instances = [];
    vi.stubGlobal("EventSource", FakeEventSource);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("subscribes to the vault's event stream once", () => {
    renderHook(() => useCustodyStream(), { wrapper });
    expect(FakeEventSource.instances).toHaveLength(1);
    expect(FakeEventSource.instances[0].url).toBe("/api/sync/events");
  });

  it("goes live on the ready frame", async () => {
    const { result } = renderHook(() => useCustodyStream(), { wrapper });
    expect(result.current.live).toBe(false);
    act(() => FakeEventSource.instances[0].emit("ready"));
    await waitFor(() => expect(result.current.live).toBe(true));
  });

  // The stream says "a world changed"; the page re-reads the truth. Nothing
  // is patched from the event itself.
  it("re-reads custody state on a custody event", async () => {
    const invalidate = vi.spyOn(client, "invalidateQueries");
    renderHook(() => useCustodyStream(), { wrapper });
    act(() => FakeEventSource.instances[0].emit("custody"));
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["worlds"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["world"] });
  });

  it("shows reconnecting, and opens a fresh stream once the old one is dead", async () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useCustodyStream(), { wrapper });
    const first = FakeEventSource.instances[0];
    act(() => first.emit("ready"));
    // A transient error: EventSource retries on its own, so nothing is
    // replaced — only the dot changes.
    first.readyState = FakeEventSource.CONNECTING;
    act(() => first.onerror?.());
    expect(result.current.live).toBe(false);
    expect(FakeEventSource.instances).toHaveLength(1);

    // Gone for good (a restart, a dropped tunnel): a fresh one is cheap.
    first.readyState = FakeEventSource.CLOSED;
    act(() => first.onerror?.());
    act(() => void vi.advanceTimersByTime(5000));
    expect(FakeEventSource.instances).toHaveLength(2);
  });

  it("closes the stream when the shell unmounts", () => {
    const { unmount } = renderHook(() => useCustodyStream(), { wrapper });
    unmount();
    expect(FakeEventSource.instances[0].closed).toBe(true);
  });

  it("carries on without a stream at all — the poll is the belt", () => {
    vi.stubGlobal(
      "EventSource",
      class {
        constructor() {
          throw new Error("no event streams here");
        }
      },
    );
    expect(() => renderHook(() => useCustodyStream(), { wrapper })).not.toThrow();
  });
});
