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

  // The join password is a decision, not a requirement: blank means an
  // open server, and the dialog says so where the choice is being made.
  // The raise itself needs only a name (and a host, prefilled here).
  it("carries the join password, and raises without one as an open server", async () => {
    vi.spyOn(api, "provisionDefaults").mockResolvedValue({ available: true, host: "10.0.0.9" });
    const provision = vi.spyOn(api, "provisionServer").mockResolvedValue({
      server: makeServer({ name: "Keep" }),
      adminPassword: "pw",
      agentToken: "tok",
      stack: "services: {}",
      deployed: true,
    });
    const user = userEvent.setup();
    open();

    await waitFor(() => expect(api.provisionDefaults).toHaveBeenCalled());
    expect(screen.getByText(/open server/i)).toBeInTheDocument();
    await user.type(screen.getByPlaceholderText("Emberhold"), "Keep");
    await user.type(screen.getByLabelText(/join password/i), "the-word");

    await user.click(screen.getByRole("button", { name: /raise the server/i }));
    await waitFor(() => expect(provision).toHaveBeenCalled());
    expect(provision.mock.calls[0][0]).toMatchObject({ joinPassword: "the-word", name: "Keep" });
  });

  // One UDP port carries game and query both — the dialog must not imply
  // a second one is being taken (the sibling consoles' games did).
  it("says the single port carries game and query", async () => {
    vi.spyOn(api, "provisionDefaults").mockResolvedValue({ available: true, ports: { game: 15700, agent: 8900 } });
    open();
    await waitFor(() => expect(api.provisionDefaults).toHaveBeenCalled());
    await screen.findByText("15700");
    expect(screen.queryByText("15701")).not.toBeInTheDocument();
    expect(screen.getByText(/game and the/i)).toBeInTheDocument();
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
    await user.type(screen.getByPlaceholderText("Emberhold"), "Keep");
    await user.click(screen.getByRole("button", { name: /raise the server/i }));

    await screen.findByText(/is rising/i);
    expect(screen.getByText("generated-pw")).toBeInTheDocument();
    expect(screen.getByText("/mnt/pool/keep")).toBeInTheDocument();
  });
});
