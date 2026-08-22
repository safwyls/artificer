import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "./api";
import { appIdOf, artFor, artSignature, gameKey, resetArtCache, useArtwork } from "./art";
import { makeStatus, makeWorld } from "../test/utils";

const wrapper = ({ children }: { children: ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

describe("cover-art keys", () => {
  it("prefers the Steam app id and falls back to a lowercased name", () => {
    expect(gameKey({ appId: "1623730", name: "Palworld" })).toBe("app:1623730");
    expect(gameKey({ appId: "", name: "  Palworld " })).toBe("name:palworld");
  });

  it("survives whatever the companion put in gameMeta", () => {
    expect(appIdOf(makeWorld({ gameMeta: '{"appId":1623730}' }))).toBe("1623730");
    expect(appIdOf(makeWorld({ gameMeta: "not json at all" }))).toBe("");
    expect(appIdOf(makeWorld({ gameMeta: "" }))).toBe("");
    expect(appIdOf(makeWorld({ gameMeta: "null" }))).toBe("");
  });

  it("orders the signature, so the same set of worlds is the same key", () => {
    const a = makeStatus({ world: makeWorld({ id: 1, gameTitle: "Palworld" }) });
    const b = makeStatus({ world: makeWorld({ id: 2, gameTitle: "Enshrouded" }) });
    expect(artSignature([a, b])).toBe(artSignature([b, a]));
  });
});

// Regression: covers are fetched when the *set* of worlds changes, never on
// the refresh timer. Asking on every poll would be a lookup every twenty
// seconds for a list that changes about once a week.
describe("useArtwork asks once per set of worlds", () => {
  beforeEach(() => {
    resetArtCache();
    vi.spyOn(api, "artwork").mockResolvedValue({
      art: { "name:palworld": { cover: "https://example.test/cover.jpg" } },
    });
  });
  afterEach(() => vi.restoreAllMocks());

  it("does not refetch when the same worlds are re-rendered", async () => {
    const worlds = [makeStatus({ world: makeWorld({ gameTitle: "Palworld" }) })];
    const { rerender } = renderHook(({ w }) => useArtwork(w), {
      wrapper,
      initialProps: { w: worlds },
    });
    await waitFor(() => expect(api.artwork).toHaveBeenCalledTimes(1));
    // A poll returns fresh objects for the same worlds — the *set* has not
    // changed, so nothing is asked again.
    rerender({ w: [makeStatus({ world: makeWorld({ gameTitle: "Palworld" }) })] });
    await new Promise((r) => setTimeout(r, 10));
    expect(api.artwork).toHaveBeenCalledTimes(1);
  });

  it("asks again when a world joins the shelf", async () => {
    const one = [makeStatus({ world: makeWorld({ gameTitle: "Palworld" }) })];
    const { rerender } = renderHook(({ w }) => useArtwork(w), {
      wrapper,
      initialProps: { w: one },
    });
    await waitFor(() => expect(api.artwork).toHaveBeenCalledTimes(1));
    rerender({
      w: [...one, makeStatus({ world: makeWorld({ id: 2, gameTitle: "Enshrouded" }) })],
    });
    await waitFor(() => expect(api.artwork).toHaveBeenCalledTimes(2));
  });

  it("asks nothing at all for an empty shelf", async () => {
    renderHook(() => useArtwork([]), { wrapper });
    await new Promise((r) => setTimeout(r, 10));
    expect(api.artwork).not.toHaveBeenCalled();
  });

  it("swallows a failed lookup: covers are decoration", async () => {
    vi.spyOn(api, "artwork").mockRejectedValue(new Error("igdb is down"));
    const { result } = renderHook(
      () => useArtwork([makeStatus({ world: makeWorld({ gameTitle: "Palworld" }) })]),
      { wrapper },
    );
    await waitFor(() => expect(result.current.ready).toBe(true));
    expect(artFor(makeWorld({ gameTitle: "Palworld" }))).toEqual({});
  });

  it("finds a cover fetched under the name key for a world keyed by name", async () => {
    renderHook(() => useArtwork([makeStatus({ world: makeWorld({ gameTitle: "Palworld" }) })]), {
      wrapper,
    });
    await waitFor(() =>
      expect(artFor(makeWorld({ gameTitle: "Palworld" })).cover).toBe(
        "https://example.test/cover.jpg",
      ),
    );
  });
});
