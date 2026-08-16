import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FtPlayerRows } from "./FtFlameborn";
import { api, type BansResult, type Player } from "../../lib/api";
import { renderWithProviders } from "../../test/utils";

const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("sonner", () => ({
  toast: {
    success: (...args: unknown[]) => toastSuccess(...args),
    error: (...args: unknown[]) => toastError(...args),
  },
}));

// The moderation grant is what decides whether the roster's ban action is
// live at all, so each case sets it explicitly.
const can = vi.fn();
vi.mock("../../lib/auth", () => ({
  useAuth: () => ({ username: "mod", isAdmin: false, can: (p: string) => can(p), logout: vi.fn() }),
}));

const STEAM = "76561198000000001";

function player(over: Partial<Player> = {}): Player {
  return {
    name: "Wren",
    playerId: STEAM,
    userId: STEAM,
    level: 0,
    ping: 0,
    location_x: 0,
    location_y: 0,
    ...over,
  };
}

function bans(list: BansResult["bans"]): BansResult {
  return {
    bans: list,
    path: "enshrouded_server.json",
    writable: true,
    objectShape: false,
    unreadable: 0,
    running: true,
    pending: [],
    reverted: [],
  };
}

function renderRoster(players: Player[], presentCount?: number) {
  return renderWithProviders(
    <FtPlayerRows serverId={1} players={players} online loading={false} presentCount={presentCount} />,
  );
}

describe("FtPlayerRows moderation", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    toastSuccess.mockClear();
    toastError.mockClear();
    can.mockReturnValue(true);
  });

  // There is no kick outside the in-game player menu, and no amount of
  // permission changes that.
  it("keeps Kick dead with its reason, for everyone", async () => {
    vi.spyOn(api, "serverBans").mockResolvedValue(bans([]));
    renderRoster([player()]);

    const kick = await screen.findByRole("button", { name: "Kick" });
    expect(kick).toBeDisabled();
    expect(kick).toHaveAttribute("title", expect.stringContaining("in-game player menu"));
  });

  it("offers a ban that names what it actually does, and confirms first", async () => {
    vi.spyOn(api, "serverBans").mockResolvedValue(bans([]));
    const update = vi.spyOn(api, "updateServerBans").mockResolvedValue(bans([{ index: -1, id: STEAM }]));
    renderRoster([player()]);

    // Radix marks the page inert behind its modal; the check would fail on
    // the confirm click for that reason alone.
    const user = userEvent.setup({ pointerEventsCheck: 0 });

    // The label is deliberately not "Ban": it writes a list, it doesn't
    // eject anyone.
    await user.click(await screen.findByRole("button", { name: "Add to ban list" }));
    expect(await screen.findByText(/does not remove them now/i)).toBeInTheDocument();
    expect(update).not.toHaveBeenCalled();

    const dialog = within(screen.getByRole("dialog"));
    await user.click(dialog.getByRole("button", { name: "Add to ban list" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(1, [{ index: -1, id: STEAM }]));
    expect(toastSuccess).toHaveBeenCalledWith("Wren added to the ban list", expect.anything());
  });

  it("shows an already-banned player as banned rather than offering it again", async () => {
    vi.spyOn(api, "serverBans").mockResolvedValue(bans([{ index: 0, id: STEAM }]));
    renderRoster([player()]);

    expect(await screen.findByText("Banned")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add to ban list" })).not.toBeInTheDocument();
  });

  // A session whose join line scrolled out of the log ring has no id, only
  // a peer handle — which is not something that can be banned.
  it("offers nothing for a player with no account id", async () => {
    vi.spyOn(api, "serverBans").mockResolvedValue(bans([]));
    renderRoster([player({ name: "Peer #3", userId: "peer-3", playerId: "peer-3" })]);

    await screen.findByRole("button", { name: "Kick" });
    expect(screen.queryByRole("button", { name: "Add to ban list" })).not.toBeInTheDocument();
    expect(screen.queryByText("Banned")).not.toBeInTheDocument();
  });

  // The game's own count can exceed what the log can name: a join line
  // older than the agent's ring is gone, but the player is not. The
  // roster has to own up to that rather than quietly under-report.
  it("says how many present players the log can't name", async () => {
    vi.spyOn(api, "serverBans").mockResolvedValue(bans([]));
    renderRoster([player()], 3);

    expect(await screen.findByText(/reports 2 more players online/i)).toBeInTheDocument();
  });

  it("explains an empty roster on a server that reports players", async () => {
    vi.spyOn(api, "serverBans").mockResolvedValue(bans([]));
    renderRoster([], 2);

    expect(await screen.findByText(/2 players are on the server/i)).toBeInTheDocument();
    // Not the "nobody is here" copy, which would be a different claim.
    expect(screen.queryByText(/fire burns alone/i)).not.toBeInTheDocument();
  });

  // A count below the roster is a player who left between two reads, and
  // the next refresh fixes it; it must never render as negative.
  it("says nothing when the count trails the roster", async () => {
    vi.spyOn(api, "serverBans").mockResolvedValue(bans([]));
    renderRoster([player(), player({ name: "Ash", userId: "76561198000000002", playerId: "76561198000000002" })], 1);

    await screen.findByText("Wren");
    expect(screen.queryByText(/more players online/i)).not.toBeInTheDocument();
  });

  it("falls back to the disabled Ban with its reason without the moderation grant", async () => {
    can.mockReturnValue(false);
    const list = vi.spyOn(api, "serverBans").mockResolvedValue(bans([]));
    renderRoster([player()]);

    const ban = await screen.findByRole("button", { name: "Ban" });
    expect(ban).toBeDisabled();
    // And no ban list is fetched for someone who could not act on it.
    expect(list).not.toHaveBeenCalled();
  });
});
