import { describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { WorldsTab, groupOf } from "./WorldsTab";
import { makeGame, makeLink, makeState, makeSyncWorld, renderWithProviders } from "../test/utils";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn(), loading: vi.fn() },
}));

const world = (id: number, name: string, over: Record<string, unknown> = {}) =>
  makeSyncWorld({ world: { ...makeSyncWorld().world, id, name }, ...over });

const holder = (username: string, o: Record<string, unknown> = {}) => ({
  sessionId: 7,
  username,
  expiresAt: new Date(Date.now() + 40 * 3_600_000).toISOString(),
  claimable: false,
  ...o,
});

/** One of each: a world this machine holds, a free one, one someone else
 * has. */
const populated = () =>
  makeState({
    links: [
      makeLink({ worldId: 1, sessionId: 7 }),
      makeLink({ worldId: 2 }),
      makeLink({ worldId: 3 }),
    ],
    discovered: { games: [makeGame(), makeGame({ name: "Valheim", appId: "892970" })], probes: [] },
    sync: {
      configured: true,
      username: "safwyl",
      busy: false,
      worlds: [
        world(1, "Embervale", { holder: holder("safwyl") }),
        world(2, "Cinderfall"),
        world(3, "Verdant Reach", { holder: holder("rook") }),
      ],
    },
  });

const show = (state = populated(), offline = false) =>
  renderWithProviders(
    <WorldsTab state={state} art={{}} offline={offline} onOpenGames={() => {}} />,
  );

describe("groupOf", () => {
  // A world still arriving is a world you hold — the row just has nothing
  // to offer yet, which is a row concern, not a grouping one.
  it("puts every custody state in exactly one group", () => {
    expect(groupOf("mine")).toBe("yours");
    expect(groupOf("fetching")).toBe("yours");
    expect(groupOf("free")).toBe("free");
    expect(groupOf("held")).toBe("held");
    expect(groupOf("expired")).toBe("held");
    expect(groupOf("gone")).toBe("held");
  });
});

describe("WorldsTab", () => {
  it("groups worlds by what you can do with them, and counts each group", () => {
    show();
    expect(screen.getByText("Checked out to you")).toBeInTheDocument();
    expect(screen.getByText("Free to take · 1")).toBeInTheDocument();
    expect(screen.getByText("Held by someone else · 1")).toBeInTheDocument();
  });

  // Grouping replaces the per-row status prose the old page carried.
  it("gives each group's rows the one action that group calls for", () => {
    show();
    expect(screen.getByRole("button", { name: "Play" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Check out & play" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Ask for it back" })).toBeInTheDocument();
  });

  it("drops a group entirely rather than showing an empty one", () => {
    const state = populated();
    state.links = [makeLink({ worldId: 2 })];
    show(state);
    expect(screen.queryByText("Checked out to you")).not.toBeInTheDocument();
    expect(screen.getByText("Free to take · 1")).toBeInTheDocument();
  });

  it("says what is installed and points at the games library, once", async () => {
    const onOpenGames = vi.fn();
    renderWithProviders(
      <WorldsTab state={populated()} art={{}} onOpenGames={onOpenGames} />,
    );
    expect(screen.getByText(/2 games installed on this machine/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Open the games library" }));
    expect(onOpenGames).toHaveBeenCalled();
  });

  it("says nothing is linked rather than showing three empty panels", () => {
    const state = populated();
    state.links = [];
    show(state);
    expect(screen.getByText(/Nothing linked yet/)).toBeInTheDocument();
  });
});

// Holds last 48 hours. A countdown that runs for two days is noise; one
// under three hours is the reason to go and check the world in.
describe("WorldsTab — hold pressure", () => {
  const pressing = () => {
    const state = populated();
    state.sync.worlds = [
      world(3, "Verdant Reach", {
        holder: holder("rook", { expiresAt: new Date(Date.now() + 2 * 3_600_000 + 12 * 60_000 + 1_000).toISOString() }),
      }),
    ];
    state.links = [makeLink({ worldId: 3 })];
    return state;
  };

  it("counts a hold down only once it is under three hours", () => {
    show(pressing());
    expect(screen.getByText(/2h 1[12]m left/)).toBeInTheDocument();
  });

  it("just names who has it when the hold has hours to spare", () => {
    const state = populated();
    state.links = [makeLink({ worldId: 3 })];
    show(state);
    expect(screen.getByText("held by rook")).toBeInTheDocument();
    expect(screen.queryByText(/left$/)).not.toBeInTheDocument();
  });
});

describe("WorldsTab — offline", () => {
  // The whole point of the offline state: the hold you already have
  // stands, so Play stays. Nothing that would *take* a world is offered,
  // because custody cannot be confirmed.
  it("keeps Play and withdraws every verb that would take custody", () => {
    show(populated(), true);
    expect(screen.getByRole("button", { name: "Play" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Check out & play" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Ask for it back" })).not.toBeInTheDocument();
  });

  it("says the hold stands while offline", () => {
    show(populated(), true);
    const group = screen.getByText("Checked out to you").parentElement as HTMLElement;
    expect(within(group).getByText(/the hold stands until the vault answers/)).toBeInTheDocument();
  });
});
