import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { api } from "../lib/api";
import { AdminUsers } from "./AdminUsers";
import { makeUser, renderWithProviders } from "../test/utils";

vi.mock("../lib/auth", () => ({ useAuth: () => ({ username: "safwyl", isAdmin: true, canSync: true }) }));

describe("AdminUsers", () => {
  beforeEach(() => {
    vi.spyOn(api, "users").mockResolvedValue([
      makeUser({ id: 1, username: "safwyl", role: "admin", permissions: [] }),
      makeUser({ id: 2, username: "mira", permissions: ["savesync"] }),
      makeUser({ id: 3, username: "torv", permissions: [], disabled: true }),
    ]);
    vi.spyOn(api, "updateUser").mockResolvedValue(undefined);
  });
  afterEach(() => vi.restoreAllMocks());

  it("shows an admin's custody as implied, and everyone else's as granted or not", async () => {
    renderWithProviders(<AdminUsers />);
    expect(await screen.findByText("via admin")).toBeInTheDocument();
    expect(screen.getByText("yes")).toBeInTheDocument();
    expect(screen.getByText("no")).toBeInTheDocument();
    expect(screen.getByText("disabled")).toBeInTheDocument();
  });

  // Regression: disabling used to send only `disabled`, and the API — which
  // replaces the record — cleared role and permissions with it.
  it("disabling a user keeps their role and their custody grant", async () => {
    renderWithProviders(<AdminUsers />);
    await screen.findByText("mira");
    const rows = screen.getAllByRole("row");
    const mira = rows.find((r) => r.textContent?.includes("mira"))!;
    await userEvent.click(within(mira, "Disable"));
    await waitFor(() =>
      expect(api.updateUser).toHaveBeenCalledWith(2, {
        role: "user",
        permissions: ["savesync"],
        disabled: true,
      }),
    );
  });

  it("granting custody leaves the role and the disabled flag alone", async () => {
    renderWithProviders(<AdminUsers />);
    await screen.findByText("torv");
    const torv = screen.getAllByRole("row").find((r) => r.textContent?.includes("torv"))!;
    await userEvent.click(within(torv, "Grant custody"));
    await waitFor(() =>
      expect(api.updateUser).toHaveBeenCalledWith(3, {
        role: "user",
        permissions: ["savesync"],
        disabled: true,
      }),
    );
  });

  it("a demotion carries the custody grant with it", async () => {
    renderWithProviders(<AdminUsers />);
    await screen.findByText("safwyl");
    await userEvent.click(screen.getByRole("button", { name: "Make ordinary user" }));
    // It asks first, and says what the demoted account keeps.
    expect(await screen.findByText(/They keep world custody/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Demote" }));
    await waitFor(() =>
      expect(api.updateUser).toHaveBeenCalledWith(1, {
        role: "user",
        permissions: ["savesync"],
        disabled: false,
      }),
    );
  });

  it("asks before deleting, and says what survives", async () => {
    const remove = vi.spyOn(api, "deleteUser").mockResolvedValue(undefined);
    renderWithProviders(<AdminUsers />);
    await screen.findByText("mira");
    const mira = screen.getAllByRole("row").find((r) => r.textContent?.includes("mira"))!;
    await userEvent.click(within(mira, "Delete"));
    expect(await screen.findByText("Their worlds and versions stay.")).toBeInTheDocument();
    expect(remove).not.toHaveBeenCalled();
    const confirms = screen.getAllByRole("button", { name: "Delete" });
    await userEvent.click(confirms[confirms.length - 1]);
    await waitFor(() => expect(remove).toHaveBeenCalledWith(2));
  });

  it("new accounts arrive with world custody", async () => {
    const create = vi.spyOn(api, "createUser").mockResolvedValue(undefined);
    renderWithProviders(<AdminUsers />);
    await userEvent.type(await screen.findByLabelText("Username"), "kes");
    await userEvent.type(screen.getByLabelText("Password"), "opensesame");
    await userEvent.click(screen.getByRole("button", { name: "Add user" }));
    await waitFor(() => expect(create).toHaveBeenCalledWith("kes", "opensesame", ""));
  });
});

/** The button with this label inside one row — the table repeats every verb
 * once per user, so a bare getByRole would be ambiguous. */
function within(row: HTMLElement, label: string): HTMLElement {
  const found = [...row.querySelectorAll("button")].find((b) => b.textContent?.trim() === label);
  if (!found) throw new Error(`no "${label}" button in that row`);
  return found;
}
