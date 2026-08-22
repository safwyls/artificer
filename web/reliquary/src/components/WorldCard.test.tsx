import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { api } from "../lib/api";
import { resetArtCache } from "../lib/art";
import { WorldCard } from "./WorldCard";
import { makeHolder, makeStatus, makeVersion, makeWorld, renderWithProviders } from "../test/utils";

const auth = { username: "safwyl", isAdmin: true, canSync: true };
vi.mock("../lib/auth", () => ({ useAuth: () => auth }));

describe("WorldCard", () => {
  beforeEach(() => {
    resetArtCache();
    vi.spyOn(api, "artwork").mockResolvedValue({ art: {} });
  });
  afterEach(() => vi.restoreAllMocks());

  it("shows the custody chip and the one action it calls for", () => {
    renderWithProviders(<WorldCard status={makeStatus({ head: makeVersion() })} />);
    expect(screen.getByText("Free")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Check out" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Check in…" })).not.toBeInTheDocument();
  });

  it("names the holder, the expiry and the queued next player", () => {
    renderWithProviders(
      <WorldCard status={makeStatus({ holder: makeHolder(), claimedBy: "torv" })} />,
    );
    expect(screen.getByText("Held")).toBeInTheDocument();
    expect(screen.getByText(/held by mira/)).toBeInTheDocument();
    expect(screen.getByText(/next claim: torv/)).toBeInTheDocument();
  });

  it("says what has been asked of a quiet holder, and how long ago", () => {
    const status = makeStatus({
      holder: makeHolder({ requestedKind: "checkin", requestedAt: "2026-08-21T10:00:00Z" }),
    });
    renderWithProviders(<WorldCard status={status} />);
    expect(screen.getByText(/waiting for mira's companion to check in and release/)).toBeInTheDocument();
  });

  it("calls the world 'held by you' rather than by your own name", () => {
    renderWithProviders(
      <WorldCard status={makeStatus({ holder: makeHolder({ username: "safwyl" }) })} />,
    );
    expect(screen.getByText(/held by you/)).toBeInTheDocument();
  });

  // Regression: the page this replaces built its handlers by interpolating
  // values into HTML attributes, so a name (or a permission) containing a
  // quote closed the attribute early and the button silently stopped
  // working. React passes values, not markup — this asserts both halves:
  // the name renders literally, and the verb behind it still fires.
  it("renders a world named with quotes and angle brackets, and still deletes it", async () => {
    const remove = vi.spyOn(api, "deleteWorld").mockResolvedValue(undefined);
    const hostile = `The "Great" <script>alert('x')</script> World`;
    renderWithProviders(<WorldCard status={makeStatus({ world: makeWorld({ name: hostile }) })} />);
    // Both links to the world (the cover and the title) carry the name.
    expect(screen.getAllByRole("link", { name: hostile })).toHaveLength(2);

    await userEvent.click(screen.getByRole("button", { name: `More actions for ${hostile}` }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Delete" }));
    // The confirm text carries the name too, and reads as the name.
    expect(await screen.findByText(`Delete ${hostile}?`)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Delete the world" }));
    await waitFor(() => expect(remove).toHaveBeenCalledWith(1));
  });

  it("asks before force-releasing, and says what it costs", async () => {
    const release = vi.spyOn(api, "release").mockResolvedValue(undefined);
    renderWithProviders(<WorldCard status={makeStatus({ holder: makeHolder() })} />);
    await userEvent.click(screen.getByRole("button", { name: /More actions/ }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Force release" }));
    expect(
      await screen.findByText("Anything they have not sent is left on their machine."),
    ).toBeInTheDocument();
    expect(release).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Force release" }));
    await waitFor(() => expect(release).toHaveBeenCalledWith(1));
  });

  it("renews the hold against the holder's own session", async () => {
    const renew = vi.spyOn(api, "renew").mockResolvedValue(undefined);
    renderWithProviders(
      <WorldCard status={makeStatus({ holder: makeHolder({ username: "safwyl", sessionId: 99 }) })} />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Renew hold" }));
    await waitFor(() => expect(renew).toHaveBeenCalledWith(99));
  });

  it("points the head download at the head version", () => {
    renderWithProviders(<WorldCard status={makeStatus({ head: makeVersion({ id: 41 }) })} />);
    expect(screen.getByRole("link", { name: "Download head" })).toHaveAttribute(
      "href",
      "/api/sync/worlds/1/versions/41/download",
    );
  });
});
