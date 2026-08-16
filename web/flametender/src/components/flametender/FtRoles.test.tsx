import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FtRoles } from "./FtRoles";
import { api, ApiError, type RoleGroup } from "../../lib/api";
import { renderWithProviders } from "../../test/utils";

const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("sonner", () => ({
  toast: {
    success: (...args: unknown[]) => toastSuccess(...args),
    error: (...args: unknown[]) => toastError(...args),
  },
}));

function group(over: Partial<RoleGroup> = {}): RoleGroup {
  return {
    index: 0,
    name: "Keepers",
    password: "keeper-pw",
    canKickBan: true,
    canAccessInventories: true,
    canEditBase: true,
    canExtendBase: true,
    canEditWorld: true,
    reservedSlots: 1,
    ...over,
  };
}

const roles = {
  groups: [
    group(),
    group({
      index: 1,
      name: "Friends",
      password: "join-pw",
      canKickBan: false,
      canEditWorld: false,
      reservedSlots: 0,
    }),
  ],
  path: "enshrouded_server.json",
  writable: true,
  restartRequired: true,
};

describe("FtRoles", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    toastSuccess.mockClear();
    toastError.mockClear();
  });

  it("shows each group's capabilities as toggles, with the password hidden", async () => {
    vi.spyOn(api, "serverRoles").mockResolvedValue(roles);
    renderWithProviders(<FtRoles serverId={1} />);

    expect(await screen.findByDisplayValue("Keepers")).toBeInTheDocument();
    // The kick/ban chip carries the answer to "who has admin here", so it
    // has to be readable as on/off without clicking anything.
    const chips = await screen.findAllByRole("switch", { name: "Kick/ban" });
    expect(chips[0]).toHaveAttribute("aria-checked", "true");
    expect(chips[1]).toHaveAttribute("aria-checked", "false");

    // Passwords are on screen because the settings grant already means
    // "may read this file", but not in the clear by default.
    const pw = screen.getByLabelText("Join password for Keepers");
    expect(pw).toHaveAttribute("type", "password");
    await userEvent.click(screen.getAllByLabelText("Reveal password")[0]);
    expect(pw).toHaveAttribute("type", "text");
  });

  it("saves nothing until something changes, then sends the whole list", async () => {
    vi.spyOn(api, "serverRoles").mockResolvedValue(roles);
    const update = vi.spyOn(api, "updateServerRoles").mockResolvedValue(roles);
    renderWithProviders(<FtRoles serverId={1} />);

    const save = await screen.findByRole("button", { name: "Save roles" });
    expect(save).toBeDisabled();

    await userEvent.click(screen.getAllByRole("switch", { name: "Edit world" })[1]);
    expect(save).toBeEnabled();
    await userEvent.click(save);

    await waitFor(() => expect(update).toHaveBeenCalled());
    const sent = update.mock.calls[0][1];
    expect(sent).toHaveLength(2);
    expect(sent[1].canEditWorld).toBe(true);
    // The index is how the backend keeps fields this console doesn't model.
    expect(sent[1].index).toBe(1);
    expect(toastSuccess).toHaveBeenCalledWith("Roles saved — restart to apply");
  });

  it("adds a group that carries no admin rights by default", async () => {
    vi.spyOn(api, "serverRoles").mockResolvedValue(roles);
    const update = vi.spyOn(api, "updateServerRoles").mockResolvedValue(roles);
    renderWithProviders(<FtRoles serverId={1} />);

    await userEvent.click(await screen.findByRole("button", { name: /Add group/ }));
    const names = screen.getAllByLabelText("Group name");
    await userEvent.type(names[names.length - 1], "Builders");
    await userEvent.click(screen.getByRole("button", { name: "Save roles" }));

    await waitFor(() => expect(update).toHaveBeenCalled());
    const added = update.mock.calls[0][1][2];
    expect(added.name).toBe("Builders");
    // A new group is a player role, not a second admin: -1 marks it as new,
    // and kick/ban is off.
    expect(added.index).toBe(-1);
    expect(added.canKickBan).toBe(false);
  });

  it("discards a removal instead of writing it when asked", async () => {
    vi.spyOn(api, "serverRoles").mockResolvedValue(roles);
    const update = vi.spyOn(api, "updateServerRoles").mockResolvedValue(roles);
    renderWithProviders(<FtRoles serverId={1} />);

    await userEvent.click(await screen.findByRole("button", { name: "Remove Friends" }));
    expect(screen.queryByDisplayValue("Friends")).not.toBeInTheDocument();

    // Draft-then-save is what makes a misclicked ✕ recoverable.
    await userEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(await screen.findByDisplayValue("Friends")).toBeInTheDocument();
    expect(update).not.toHaveBeenCalled();
  });

  it("relays the backend's refusal rather than inventing its own wording", async () => {
    vi.spyOn(api, "serverRoles").mockResolvedValue(roles);
    const reason = "keep one group with kick/ban rights — moderation happens in the game's own player list";
    vi.spyOn(api, "updateServerRoles").mockRejectedValue(new ApiError(400, reason));
    renderWithProviders(<FtRoles serverId={1} />);

    await userEvent.click(await screen.findByRole("button", { name: "Remove Keepers" }));
    await userEvent.click(screen.getByRole("button", { name: "Save roles" }));

    await waitFor(() => expect(toastError).toHaveBeenCalledWith("Save failed", { description: reason }));
  });

  it("warns before the save fails when the file is read-only", async () => {
    vi.spyOn(api, "serverRoles").mockResolvedValue({ ...roles, writable: false });
    renderWithProviders(<FtRoles serverId={1} />);

    expect(await screen.findByText(/read-only mount/i)).toBeInTheDocument();
  });

  it("stays quiet when no config path is set — the settings panel says that once", async () => {
    vi.spyOn(api, "serverRoles").mockRejectedValue(new ApiError(400, "no config path configured"));
    const { container } = renderWithProviders(<FtRoles serverId={1} />);

    await waitFor(() => expect(within(container).queryByText(/Reading the file/)).not.toBeInTheDocument());
    expect(screen.queryByText("no config path configured")).not.toBeInTheDocument();
  });
});
