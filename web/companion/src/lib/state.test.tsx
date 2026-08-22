import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "./api";
import { gameSignature, useArtwork, useSaveHints, useSeededField } from "./state";
import { makeGame } from "../test/utils";

const wrapper = ({ children }: { children: ReactNode }) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

describe("gameSignature", () => {
  it("is stable under reordering — the same shelf is the same key", () => {
    const a = makeGame({ name: "Enshrouded", appId: "1" });
    const b = makeGame({ name: "Valheim", appId: "2" });
    expect(gameSignature([a, b])).toBe(gameSignature([b, a]));
  });

  it("changes when a game joins the shelf", () => {
    const a = makeGame({ appId: "1" });
    expect(gameSignature([a])).not.toBe(gameSignature([a, makeGame({ appId: "2" })]));
  });
});

// Regression: covers were fetched once at page load. Discovery is a
// filesystem walk that finishes *after* the first render, so that call
// always found an empty shelf, asked for nothing, and never ran again —
// the service's counter read "0 asked" while its credentials tested fine.
describe("useArtwork asks when the game set changes, never on the poll", () => {
  beforeEach(() => {
    vi.spyOn(api, "artwork").mockResolvedValue({ art: { "app:1203620": { cover: "c" } }, asked: true });
  });
  afterEach(() => vi.restoreAllMocks());

  it("asks nothing for an empty shelf, then asks once discovery lands", async () => {
    const { rerender, result } = renderHook(({ g }) => useArtwork(g), {
      wrapper,
      initialProps: { g: [] as ReturnType<typeof makeGame>[] },
    });
    await new Promise((r) => setTimeout(r, 10));
    expect(api.artwork).not.toHaveBeenCalled();

    rerender({ g: [makeGame()] });
    await waitFor(() => expect(api.artwork).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(result.current.art["app:1203620"].cover).toBe("c"));
  });

  it("does not ask again for the same shelf", async () => {
    const games = [makeGame()];
    const { rerender } = renderHook(({ g }) => useArtwork(g), { wrapper, initialProps: { g: games } });
    await waitFor(() => expect(api.artwork).toHaveBeenCalledTimes(1));
    // A poll returns fresh objects describing the same games.
    rerender({ g: [makeGame()] });
    await new Promise((r) => setTimeout(r, 10));
    expect(api.artwork).toHaveBeenCalledTimes(1);
  });

  it("asks again when a game is installed", async () => {
    const { rerender } = renderHook(({ g }) => useArtwork(g), {
      wrapper,
      initialProps: { g: [makeGame()] },
    });
    await waitFor(() => expect(api.artwork).toHaveBeenCalledTimes(1));
    rerender({ g: [makeGame(), makeGame({ name: "Valheim", appId: "892970" })] });
    await waitFor(() => expect(api.artwork).toHaveBeenCalledTimes(2));
  });

  it("swallows a failed lookup — covers are decoration", async () => {
    vi.spyOn(api, "artwork").mockRejectedValue(new Error("igdb is down"));
    const { result } = renderHook(() => useArtwork([makeGame()]), { wrapper });
    await waitFor(() => expect(result.current.art).toEqual({}));
  });

  it("says when the service was asked and had nothing", async () => {
    vi.spyOn(api, "artwork").mockResolvedValue({ art: {}, asked: true });
    const { result } = renderHook(() => useArtwork([makeGame()]), { wrapper });
    await waitFor(() => expect(result.current.empty).toBe(true));
  });
});

describe("useSaveHints", () => {
  beforeEach(() => {
    vi.spyOn(api, "saveHints").mockResolvedValue({ available: true, known: 5 });
  });
  afterEach(() => vi.restoreAllMocks());

  // Hints come from the service, so an unconnected companion must not ask.
  it("asks nothing while the companion is not connected", async () => {
    renderHook(() => useSaveHints([makeGame()], false), { wrapper });
    await new Promise((r) => setTimeout(r, 10));
    expect(api.saveHints).not.toHaveBeenCalled();
  });

  it("asks once per game set once connected", async () => {
    const { rerender } = renderHook(({ g }) => useSaveHints(g, true), {
      wrapper,
      initialProps: { g: [makeGame()] },
    });
    await waitFor(() => expect(api.saveHints).toHaveBeenCalledTimes(1));
    rerender({ g: [makeGame()] });
    await new Promise((r) => setTimeout(r, 10));
    expect(api.saveHints).toHaveBeenCalledTimes(1);
  });

  it("survives a catalogue that is unavailable — hints are an improvement", async () => {
    vi.spyOn(api, "saveHints").mockRejectedValue(new Error("no catalogue"));
    const { result } = renderHook(() => useSaveHints([makeGame()], true), { wrapper });
    await waitFor(() => expect(result.current).toEqual({}));
  });
});

// Regression: the poll runs every five seconds. A field seeded from it
// without this guard loses a character every time someone types slowly.
describe("useSeededField", () => {
  it("follows the server's value until the player touches it", () => {
    const { result, rerender } = renderHook(({ v }) => useSeededField(v), {
      wrapper,
      initialProps: { v: "https://vault.example.test" },
    });
    expect(result.current.value).toBe("https://vault.example.test");
    rerender({ v: "https://moved.example.test" });
    expect(result.current.value).toBe("https://moved.example.test");
  });

  it("stops following once edited, even across polls", () => {
    const { result, rerender } = renderHook(({ v }) => useSeededField(v), {
      wrapper,
      initialProps: { v: "https://vault.example.test" },
    });
    act(() =>
      result.current.props.onChange({
        target: { value: "https://typing" },
      } as React.ChangeEvent<HTMLInputElement>),
    );
    expect(result.current.value).toBe("https://typing");
    rerender({ v: "https://vault.example.test" });
    expect(result.current.value).toBe("https://typing");
  });

  it("stops following while focused, even before a keystroke lands", () => {
    const { result, rerender } = renderHook(({ v }) => useSeededField(v), {
      wrapper,
      initialProps: { v: "a" },
    });
    act(() => result.current.props.onFocus());
    rerender({ v: "b" });
    expect(result.current.value).toBe("a");
  });

  it("lets the server lead again once the change is saved", () => {
    const { result, rerender } = renderHook(({ v }) => useSeededField(v), {
      wrapper,
      initialProps: { v: "a" },
    });
    act(() =>
      result.current.props.onChange({ target: { value: "b" } } as React.ChangeEvent<HTMLInputElement>),
    );
    act(() => result.current.settle("b"));
    rerender({ v: "c" });
    expect(result.current.value).toBe("c");
  });
});
