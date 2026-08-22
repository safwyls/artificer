import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Shelf, linkFor } from "./Shelf";
import { makeGame, makeLink, makeState, makeSyncWorld } from "../test/utils";

const noop = () => {};
const show = (props: Partial<Parameters<typeof Shelf>[0]> = {}) =>
  render(
    <Shelf
      state={makeState({ discovered: { games: [makeGame()], probes: [] } })}
      art={{}}
      artEmpty={false}
      hints={{}}
      activeKey={null}
      onOpen={noop}
      onRescan={noop}
      onLinkByHand={noop}
      {...props}
    />,
  );

describe("linkFor", () => {
  it("matches a link by the title recorded when it was made", () => {
    expect(linkFor(makeGame(), [makeLink({ gameTitle: "Enshrouded", dir: "elsewhere" })])).toBeTruthy();
  });

  // A link made before app ids or titles were recorded still matches by
  // the folder it points at.
  it("falls back to a save folder that matches one of the game's candidates", () => {
    const link = makeLink({
      gameTitle: "",
      dir: "C:\\Users\\you\\AppData\\Roaming\\Enshrouded\\savegame",
    });
    expect(linkFor(makeGame(), [link])).toBeTruthy();
  });

  it("does not match an unrelated game", () => {
    expect(linkFor(makeGame({ name: "Valheim", saveDirs: [] }), [makeLink()])).toBeUndefined();
  });
});

describe("Shelf", () => {
  it("captions a linked tile with its world's name, and greys the rest", () => {
    show({
      state: makeState({
        discovered: { games: [makeGame(), makeGame({ name: "Valheim", appId: "892970", saveDirs: [] })], probes: [] },
        links: [makeLink()],
        sync: { configured: true, username: "safwyl", busy: false, worlds: [makeSyncWorld()] },
      }),
    });
    expect(screen.getByText("Embervale")).toBeInTheDocument();
    expect(screen.getByText("not linked")).toBeInTheDocument();
  });

  // Steam's own redistributables, runtimes and controller configs start
  // out hidden; a shelf of those is a shelf nobody reads.
  it("collapses hidden entries into one tile that offers to show them", async () => {
    const onOpen = vi.fn();
    show({
      state: makeState({
        discovered: {
          games: [makeGame(), makeGame({ name: "Steamworks Common", appId: "228980", hidden: true })],
          probes: [],
        },
      }),
      onOpen,
    });
    expect(screen.queryByText("Steamworks Common")).not.toBeInTheDocument();
    await userEvent.click(screen.getByText("show them"));
    // Twice: the name is both the caption and the no-cover tile's own art.
    expect(screen.getAllByText("Steamworks Common").length).toBeGreaterThan(0);
    expect(screen.getByText("hide them again")).toBeInTheDocument();
  });

  it("names its own cause when nothing was found", () => {
    show({ state: makeState({ discovered: { games: [], probes: [] } }) });
    expect(screen.getByText(/The scan trail below says where it looked/)).toBeInTheDocument();
  });

  it("says so when every game found is hidden", () => {
    show({
      state: makeState({
        discovered: { games: [makeGame({ hidden: true })], probes: [] },
      }),
    });
    expect(screen.getByText("Every game found here is hidden.")).toBeInTheDocument();
  });

  it("points at the service's own panel when it has no covers", () => {
    show({ artEmpty: true });
    expect(screen.getByText(/check its Cover art panel/)).toBeInTheDocument();
  });

  it("distinguishes a catalogue that is unavailable from one that is empty", () => {
    const { rerender } = show({ hints: { error: "service unreachable" } });
    expect(screen.getByText(/Save-location catalogue unavailable: service unreachable/)).toBeInTheDocument();
    rerender(
      <Shelf
        state={makeState({ discovered: { games: [makeGame()], probes: [] } })}
        art={{}}
        artEmpty={false}
        hints={{ available: false }}
        activeKey={null}
        onOpen={noop}
        onRescan={noop}
        onLinkByHand={noop}
      />,
    );
    expect(screen.getByText(/no save-location catalogue loaded/)).toBeInTheDocument();
  });

  it("opens the game a tile stands for", async () => {
    const onOpen = vi.fn();
    show({ onOpen });
    await userEvent.click(screen.getByRole("button", { name: /Enshrouded/ }));
    expect(onOpen).toHaveBeenCalledWith(expect.objectContaining({ name: "Enshrouded" }));
  });
});

// Regression: rebuilding identical tiles on every five-second poll made
// every cover flicker, because an <img> that remounts re-fetches. Tiles
// are keyed by the game's identity and memoized, so a poll that changes
// nothing about a game leaves its DOM node — and its image — alone.
describe("Shelf does not remount tiles on a poll", () => {
  it("keeps the same cover element across a poll that changes nothing", () => {
    const withArt = { "app:1203620": { cover: "https://example.test/cover.jpg" } };
    const { rerender } = render(
      <Shelf
        state={makeState({ discovered: { games: [makeGame()], probes: [] } })}
        art={withArt}
        artEmpty={false}
        hints={{}}
        activeKey={null}
        onOpen={noop}
        onRescan={noop}
        onLinkByHand={noop}
      />,
    );
    const before = document.querySelector("img");
    expect(before).toBeTruthy();
    // The poll answers with equal-but-new objects, as JSON always does.
    rerender(
      <Shelf
        state={makeState({ discovered: { games: [makeGame()], probes: [] } })}
        art={withArt}
        artEmpty={false}
        hints={{}}
        activeKey={null}
        onOpen={noop}
        onRescan={noop}
        onLinkByHand={noop}
      />,
    );
    expect(document.querySelector("img")).toBe(before);
  });

  it("keeps a tile's identity when another game is filtered out beside it", async () => {
    const games = [makeGame(), makeGame({ name: "Steamworks Common", appId: "228980", hidden: true })];
    render(
      <Shelf
        state={makeState({ discovered: { games, probes: [] } })}
        art={{ "app:1203620": { cover: "https://example.test/cover.jpg" } }}
        artEmpty={false}
        hints={{}}
        activeKey={null}
        onOpen={noop}
        onRescan={noop}
        onLinkByHand={noop}
      />,
    );
    const before = document.querySelector("img");
    // Showing the hidden entries changes the list, but not this tile.
    await userEvent.click(screen.getByText("show them"));
    expect(document.querySelector("img")).toBe(before);
  });
});
