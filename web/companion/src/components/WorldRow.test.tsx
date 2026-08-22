import { describe, expect, it, vi, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { api } from "../lib/api";
import { WorldRow } from "./WorldRow";
import { makeLink, makeSyncWorld, renderWithProviders } from "../test/utils";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn(), loading: vi.fn() },
}));

const holder = (o: Record<string, unknown> = {}) => ({
  sessionId: 7,
  username: "mira",
  expiresAt: "2026-08-22T23:00:00Z",
  claimable: false,
  ...o,
});

const show = (world = makeSyncWorld(), link = makeLink()) =>
  renderWithProviders(
    <WorldRow link={link} world={world} me="safwyl" art={{}} configured />,
  );

afterEach(() => vi.restoreAllMocks());

describe("WorldRow", () => {
  it("shows the folder on this machine, in full", () => {
    show();
    expect(
      screen.getByText("C:\\Users\\you\\AppData\\Roaming\\Enshrouded\\savegame"),
    ).toBeInTheDocument();
  });

  it("offers checking out a free world, and nothing else custodial", () => {
    show();
    expect(screen.getByRole("button", { name: "Check out & host" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Check in" })).not.toBeInTheDocument();
  });

  it("offers check in, checkpoint and renew to the holder on this machine", () => {
    show(makeSyncWorld({ holder: holder({ username: "safwyl" }) }), makeLink({ sessionId: 7 }));
    expect(screen.getByRole("button", { name: "Check in" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Checkpoint now" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Renew hold" })).toBeInTheDocument();
  });

  // A checkpoint the service will not keep is a button that lies.
  it("hides the checkpoint verb for a world that does not keep checkpoints", () => {
    const world = makeSyncWorld({ holder: holder({ username: "safwyl" }) });
    world.world.checkpoints = false;
    show(world, makeLink({ sessionId: 7 }));
    expect(screen.queryByRole("button", { name: "Checkpoint now" })).not.toBeInTheDocument();
  });

  // The account holds it, but the save is still on its way to this
  // machine: checking in here would upload a folder that has not received
  // it yet.
  it("offers nothing custodial while the world is still being fetched here", () => {
    show(makeSyncWorld({ holder: holder({ username: "safwyl", sessionId: 9 }) }), makeLink({ sessionId: 7 }));
    expect(screen.getByText(/fetching it to this machine/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Check in" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Check out & host" })).not.toBeInTheDocument();
  });

  it("offers a takeover only once the hold has expired, and says what survives", async () => {
    const checkout = vi.spyOn(api, "checkout").mockResolvedValue({});
    show(makeSyncWorld({ holder: holder({ claimable: true }) }));
    await userEvent.click(screen.getByRole("button", { name: "Take over expired hold" }));
    expect(
      await screen.findByText("The old holder's late check-in is kept and flagged, not lost."),
    ).toBeInTheDocument();
    expect(checkout).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Take over" }));
    await waitFor(() => expect(checkout).toHaveBeenCalledWith(1, true));
  });

  it("offers to claim next only when nobody has", () => {
    const { unmount } = show(makeSyncWorld({ holder: holder() }));
    expect(screen.getByRole("button", { name: "Claim next" })).toBeInTheDocument();
    unmount();
    show(makeSyncWorld({ holder: holder(), claimedBy: "torv" }));
    expect(screen.queryByRole("button", { name: "Claim next" })).not.toBeInTheDocument();
    expect(screen.getByText(/next claim: torv/)).toBeInTheDocument();
  });

  it("says you're next rather than naming you", () => {
    show(makeSyncWorld({ holder: holder(), claimedBy: "safwyl" }));
    expect(screen.getByText(/you're next/)).toBeInTheDocument();
  });

  // A link to a world the service no longer has must say so, not render a
  // row of verbs that all fail.
  it("reports a world that has left the service", () => {
    renderWithProviders(<WorldRow link={makeLink()} world={undefined} me="safwyl" art={{}} configured />);
    expect(screen.getByText(/is not on the service any more/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Check out & host" })).not.toBeInTheDocument();
  });

  it("asks before unlinking, and promises nothing is deleted", async () => {
    const unlink = vi.spyOn(api, "unlink").mockResolvedValue({});
    show();
    await userEvent.click(screen.getByRole("button", { name: "Unlink" }));
    expect(await screen.findByText("Nothing is deleted.")).toBeInTheDocument();
    expect(unlink).not.toHaveBeenCalled();
    const confirms = screen.getAllByRole("button", { name: "Unlink" });
    await userEvent.click(confirms[confirms.length - 1]);
    await waitFor(() => expect(unlink).toHaveBeenCalledWith(1));
  });
});
