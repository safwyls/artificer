import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { GamesTab, linkFor, matches } from "./GamesTab";
import { makeGame, makeLink, makeState, makeSyncWorld } from "../test/utils";

const noop = () => {};
const props = (over: Partial<Parameters<typeof GamesTab>[0]> = {}) => ({
  state: makeState({ discovered: { games: [makeGame()], probes: [] } }),
  art: {},
  artEmpty: false,
  hints: {},
  activeKey: null,
  onOpen: noop,
  onRescan: noop,
  onLinkByHand: noop,
  ...over,
});
const show = (over: Partial<Parameters<typeof GamesTab>[0]> = {}) => render(<GamesTab {...props(over)} />);

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

// Nobody types a game's name exactly, and nobody should have to.
describe("matches", () => {
  it("ignores case and surrounding space, and matches anywhere in the name", () => {
    expect(matches(makeGame({ name: "RuneScape: Dragonwilds" }), " dragon ")).toBe(true);
    expect(matches(makeGame(), "")).toBe(true);
    expect(matches(makeGame({ name: "Valheim" }), "shroud")).toBe(false);
  });
});

const both = () =>
  makeState({
    discovered: {
      games: [makeGame(), makeGame({ name: "Valheim", appId: "892970", saveDirs: [] })],
      probes: [],
    },
    links: [makeLink()],
    sync: { configured: true, username: "safwyl", busy: false, worlds: [makeSyncWorld()] },
  });

describe("GamesTab", () => {
  // In the old grid the one linked tile was lost among twelve dimmed
  // ones, and eleven of thirteen tiles read "not linked" — which the dim
  // treatment already said.
  it("puts linked games in their own section first, captioned with the world", () => {
    show({ state: both() });
    expect(screen.getByText("Linked · 1")).toBeInTheDocument();
    expect(screen.getByText("Not linked · 1")).toBeInTheDocument();
    expect(screen.getByText("1 world · Embervale")).toBeInTheDocument();
    expect(screen.queryByText("not linked")).not.toBeInTheDocument();
  });

  // What a player wants to know before clicking is what linking will
  // cost them.
  it("says on each unlinked tile whether a folder is known or must be picked", () => {
    show({ state: both() });
    expect(screen.getByText("pick the folder yourself")).toBeInTheDocument();
    const state = both();
    state.links = [];
    render(<GamesTab {...props({ state })} />);
    expect(screen.getAllByText("save folder known").length).toBeGreaterThan(0);
  });

  it("states clickability on the tile rather than under the grid", () => {
    show({ state: both() });
    expect(screen.getAllByText("Link").length).toBe(1);
  });

  it("narrows the grid by search", async () => {
    show({ state: both() });
    await userEvent.type(screen.getByLabelText("Search installed games"), "valh");
    expect(screen.queryByText("Linked · 1")).not.toBeInTheDocument();
    expect(screen.getByText("Not linked · 1")).toBeInTheDocument();
  });

  it("says so when a search matches nothing installed", async () => {
    show({ state: both() });
    await userEvent.type(screen.getByLabelText("Search installed games"), "zzz");
    expect(screen.getByText(/Nothing installed here matches/)).toBeInTheDocument();
  });

  it("filters to linked or unlinked, and counts each", async () => {
    show({ state: both() });
    await userEvent.click(screen.getByRole("button", { name: "Linked 1" }));
    expect(screen.getByText("Linked · 1")).toBeInTheDocument();
    expect(screen.queryByText("Not linked · 1")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Unlinked 1" }));
    expect(screen.queryByText("Linked · 1")).not.toBeInTheDocument();
    expect(screen.getByText("Not linked · 1")).toBeInTheDocument();
  });

  // Steam's own redistributables, runtimes and controller configs start
  // out hidden; a shelf of those is a shelf nobody reads. It is a line of
  // text now, not a tile — a control shaped like a game gets clicked by
  // accident.
  it("reports hidden entries as a line of text, not as a fake tile", async () => {
    show({
      state: makeState({
        discovered: {
          games: [makeGame(), makeGame({ name: "Steamworks Common", appId: "228980", hidden: true })],
          probes: [],
        },
      }),
    });
    expect(screen.queryByText("Steamworks Common")).not.toBeInTheDocument();
    expect(screen.getByText(/1 entry was hidden as duplicates or launchers/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Show them" }));
    expect(screen.getAllByText("Steamworks Common").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Hide them again" })).toBeInTheDocument();
  });

  it("names its own cause when nothing was found", () => {
    show({ state: makeState({ discovered: { games: [], probes: [] } }) });
    expect(screen.getByText(/Diagnostics, at the bottom of the window, has the scan trail/)).toBeInTheDocument();
  });

  it("says so when every game found is hidden", () => {
    show({ state: makeState({ discovered: { games: [makeGame({ hidden: true })], probes: [] } }) });
    expect(screen.getByText("Every game found here is hidden.")).toBeInTheDocument();
  });

  it("points at the service's own panel when it has no covers", () => {
    show({ artEmpty: true });
    expect(screen.getByText(/check its Cover art panel/)).toBeInTheDocument();
  });

  it("distinguishes a catalogue that is unavailable from one that is empty", () => {
    const { rerender } = show({ hints: { error: "service unreachable" } });
    expect(screen.getByText(/Save-location catalogue unavailable: service unreachable/)).toBeInTheDocument();
    rerender(<GamesTab {...props({ hints: { available: false } })} />);
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
describe("GamesTab does not remount tiles on a poll", () => {
  const withArt = { "app:1203620": { cover: "https://example.test/cover.jpg" } };

  it("keeps the same cover element across a poll that changes nothing", () => {
    const { rerender } = render(<GamesTab {...props({ art: withArt })} />);
    const before = document.querySelector("img");
    expect(before).toBeTruthy();
    // The poll answers with equal-but-new objects, as JSON always does.
    rerender(<GamesTab {...props({ art: withArt })} />);
    expect(document.querySelector("img")).toBe(before);
  });

  it("keeps a tile's identity when another game is filtered out beside it", async () => {
    const games = [makeGame(), makeGame({ name: "Steamworks Common", appId: "228980", hidden: true })];
    render(
      <GamesTab
        {...props({ state: makeState({ discovered: { games, probes: [] } }), art: withArt })}
      />,
    );
    const before = document.querySelector("img");
    // Showing the hidden entries changes the list, but not this tile.
    await userEvent.click(screen.getByRole("button", { name: "Show them" }));
    expect(document.querySelector("img")).toBe(before);
  });
});
