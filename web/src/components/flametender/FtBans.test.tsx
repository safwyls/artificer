import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FtBans } from "./FtBans";
import { api, ApiError, type BansResult } from "../../lib/api";
import { renderWithProviders } from "../../test/utils";

const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("sonner", () => ({
  toast: {
    success: (...args: unknown[]) => toastSuccess(...args),
    error: (...args: unknown[]) => toastError(...args),
  },
}));

const A = "76561198000000001";
const B = "76561198000000002";

function bans(over: Partial<BansResult> = {}): BansResult {
  return {
    bans: [{ index: 0, id: A }],
    path: "enshrouded_server.json",
    writable: true,
    objectShape: false,
    unreadable: 0,
    running: false,
    ...over,
  };
}

describe("FtBans", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    toastSuccess.mockClear();
    toastError.mockClear();
  });

  it("lists the banned ids and adds one, whole-list", async () => {
    vi.spyOn(api, "serverBans").mockResolvedValue(bans());
    const update = vi.spyOn(api, "updateServerBans").mockResolvedValue(bans());
    renderWithProviders(<FtBans serverId={1} />);

    expect(await screen.findByText(A)).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText("Account to ban"), B);
    await userEvent.click(screen.getByRole("button", { name: /Add/ }));
    await userEvent.click(screen.getByRole("button", { name: "Save ban list" }));

    await waitFor(() => expect(update).toHaveBeenCalled());
    expect(update.mock.calls[0][1].map((b) => b.id)).toEqual([A, B]);
    // A new entry carries -1 so the backend writes it fresh rather than
    // merging it onto some existing element.
    expect(update.mock.calls[0][1][1].index).toBe(-1);
  });

  it("refuses a name pasted where an id belongs, before anything is sent", async () => {
    vi.spyOn(api, "serverBans").mockResolvedValue(bans());
    const update = vi.spyOn(api, "updateServerBans").mockResolvedValue(bans());
    renderWithProviders(<FtBans serverId={1} />);

    await userEvent.type(await screen.findByLabelText("Account to ban"), "Griefer");
    await userEvent.click(screen.getByRole("button", { name: /Add/ }));

    expect(toastError).toHaveBeenCalledWith("That doesn't look like a SteamID64", expect.anything());
    expect(update).not.toHaveBeenCalled();
  });

  it("lets a lift be taken back before it's saved", async () => {
    vi.spyOn(api, "serverBans").mockResolvedValue(bans());
    const update = vi.spyOn(api, "updateServerBans").mockResolvedValue(bans());
    renderWithProviders(<FtBans serverId={1} />);

    await userEvent.click(await screen.findByRole("button", { name: "Lift" }));
    expect(screen.queryByText(A)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(await screen.findByText(A)).toBeInTheDocument();
    expect(update).not.toHaveBeenCalled();
  });

  // The unverified behaviour that could silently undo an edit: the running
  // game owns this file too. Stated as a risk, on screen, only while it
  // applies.
  it("warns that a live server can overwrite the edit, and only while it's live", async () => {
    vi.spyOn(api, "serverBans").mockResolvedValue(bans({ running: true }));
    const { unmount } = renderWithProviders(<FtBans serverId={1} />);
    expect(await screen.findByText(/can be overwritten/i)).toBeInTheDocument();
    unmount();

    vi.spyOn(api, "serverBans").mockResolvedValue(bans({ running: false }));
    renderWithProviders(<FtBans serverId={1} />);
    await screen.findByRole("button", { name: "Save ban list" });
    expect(screen.queryByText(/can be overwritten/i)).not.toBeInTheDocument();
  });

  // Entries the backend couldn't parse are preserved rather than dropped,
  // and the panel has to say so — otherwise the list looks shorter than
  // the file and a save looks like it lifted somebody.
  it("accounts for entries it can't read", async () => {
    vi.spyOn(api, "serverBans").mockResolvedValue(bans({ unreadable: 2 }));
    renderWithProviders(<FtBans serverId={1} />);

    expect(await screen.findByText(/2 entries in this file aren't in a form/i)).toBeInTheDocument();
  });

  it("says plainly that a ban here doesn't eject anyone", async () => {
    vi.spyOn(api, "serverBans").mockResolvedValue(bans());
    renderWithProviders(<FtBans serverId={1} />);

    expect(await screen.findByText(/does not remove anyone who is in the world now/i)).toBeInTheDocument();
  });

  it("renders nothing when the server has no config path", async () => {
    vi.spyOn(api, "serverBans").mockRejectedValue(new ApiError(400, "no config path configured"));
    renderWithProviders(<FtBans serverId={1} />);

    await waitFor(() => expect(screen.queryByText("Banned accounts")).not.toBeInTheDocument());
  });
});
