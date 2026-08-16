import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RaiseServerDialog } from "./RaiseServerDialog";
import { api } from "../../lib/api";
import { makeServer, renderWithProviders } from "../../test/utils";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

function open() {
  return renderWithProviders(<RaiseServerDialog open onOpenChange={() => {}} />);
}

describe("RaiseServerDialog", () => {
  beforeEach(() => vi.restoreAllMocks());

  // The owner id is the whole reason this dialog can't be a plain form:
  // without one the game installs and then refuses to start, so the button
  // must not be reachable until it's filled in.
  it("will not raise a server without an owner id", async () => {
    vi.spyOn(api, "provisionDefaults").mockResolvedValue({ available: true, host: "10.0.0.9" });
    const provision = vi.spyOn(api, "provisionServer");
    const user = userEvent.setup();
    open();

    await waitFor(() => expect(api.provisionDefaults).toHaveBeenCalled());
    await user.type(screen.getByPlaceholderText("Grimwood Bastion"), "Keep");

    const raise = screen.getByRole("button", { name: /raise the server/i });
    expect(raise).toBeDisabled();

    await user.type(screen.getByLabelText(/owner id/i), "owner-abc");
    await waitFor(() => expect(raise).toBeEnabled());
    await user.click(raise);
    await waitFor(() => expect(provision).toHaveBeenCalled());
    expect(provision.mock.calls[0][0]).toMatchObject({ ownerId: "owner-abc", name: "Keep" });
  });

  // The game binds the port above its own, so the dialog has to say so —
  // an operator picking a port needs to know two are being taken.
  it("says which port pair will be published", async () => {
    vi.spyOn(api, "provisionDefaults").mockResolvedValue({ available: true, ports: { game: 7900, agent: 8900 } });
    open();
    await waitFor(() => expect(api.provisionDefaults).toHaveBeenCalled());
    await screen.findByText("7900");
    expect(screen.getByText("7901")).toBeInTheDocument();
  });

  // Without a provisioner there is nothing to deploy onto, so the dialog
  // must ask for a data path and promise a stack rather than a container.
  it("falls back to generating a stack when no provisioner is configured", async () => {
    vi.spyOn(api, "provisionDefaults").mockResolvedValue({ available: false });
    open();
    await screen.findByText(/generates a stack for you to deploy/i);
    expect(screen.getByRole("button", { name: /generate the stack/i })).toBeDisabled();
  });

  // The admin password is shown exactly once; if the result view didn't
  // render it, it would be lost with no way back.
  it("reveals the generated credentials after a deploy", async () => {
    vi.spyOn(api, "provisionDefaults").mockResolvedValue({ available: true, host: "10.0.0.9" });
    vi.spyOn(api, "provisionServer").mockResolvedValue({
      server: makeServer({ name: "Keep" }),
      adminPassword: "generated-pw",
      agentToken: "generated-token",
      stack: "services: {}",
      deployed: true,
      dataDir: "/mnt/pool/keep",
    });
    const user = userEvent.setup();
    open();

    await waitFor(() => expect(api.provisionDefaults).toHaveBeenCalled());
    await user.type(screen.getByPlaceholderText("Grimwood Bastion"), "Keep");
    await user.type(screen.getByLabelText(/owner id/i), "owner-abc");
    await user.click(screen.getByRole("button", { name: /raise the server/i }));

    await screen.findByText(/is rising/i);
    expect(screen.getByText("generated-pw")).toBeInTheDocument();
    expect(screen.getByText("/mnt/pool/keep")).toBeInTheDocument();
  });
});
