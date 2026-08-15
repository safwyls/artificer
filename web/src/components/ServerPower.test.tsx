import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ServerPower } from "./ServerPower";
import { api, ApiError } from "../lib/api";
import { renderWithProviders } from "../test/utils";

// The component reads only `can` from auth; grant everything so the save
// and power controls both render.
vi.mock("../lib/auth", () => ({
  useAuth: () => ({
    username: "admin",
    isAdmin: true,
    can: () => true,
    logout: vi.fn(),
  }),
}));

const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("sonner", () => ({
  toast: {
    success: (...args: unknown[]) => toastSuccess(...args),
    error: (...args: unknown[]) => toastError(...args),
  },
}));

const runningState = {
  name: "flameagent-main",
  status: "running",
  running: true,
  startedAt: "2026-08-11T00:00:00Z",
  exitCode: 0,
};

function renderPower() {
  return renderWithProviders(<ServerPower serverId={1} />);
}

describe("ServerPower launch mode", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    toastSuccess.mockClear();
    toastError.mockClear();
    vi.spyOn(api, "containerStatus").mockResolvedValue(runningState);
    vi.spyOn(api, "steamUpdateStatus").mockResolvedValue({
      job: null,
      agent: { version: "test", apiVersion: 1, mode: "supervisor", installDirOk: true, diskFreeBytes: 0 },
    });
  });

  const wineLaunch = {
    profile: "wine",
    label: "Windows build under Wine",
    installed: true,
    runnable: true,
    available: ["wine"],
    pendingRestart: false,
    configPath: "enshrouded_server.json",
  };

  function renderWithAgent() {
    return renderWithProviders(<ServerPower serverId={1} agentUrl="http://agent:8811" />);
  }

  it("describes the one build and offers no switcher", async () => {
    vi.spyOn(api, "serverLaunch").mockResolvedValue(wineLaunch);
    renderWithAgent();

    await screen.findByText("Launch mode");
    expect(screen.getByText(/only build/i)).toBeInTheDocument();
    // One selectable profile means nothing to switch between — a
    // single-button "chooser" would be furniture.
    expect(screen.queryByRole("button", { name: /Windows under Wine/i })).not.toBeInTheDocument();
  });

  it("points at Update server when the game isn't downloaded yet", async () => {
    vi.spyOn(api, "serverLaunch").mockResolvedValue({ ...wineLaunch, installed: false });
    renderWithAgent();

    expect(await screen.findByText(/isn't downloaded yet/i)).toBeInTheDocument();
  });

  it("offers to rebuild the agent when its image cannot run the game", async () => {
    // Wine missing from the agent image — the state a TrueNAS operator
    // lands in when a container was provisioned from a stale image and
    // nothing in their apps view can change it, so the console has to
    // offer the rebuild itself.
    vi.spyOn(api, "serverLaunch").mockResolvedValue({ ...wineLaunch, runnable: false });
    const rebuild = vi.spyOn(api, "recreateAgent").mockResolvedValue({
      container: "flameagent-emberhold",
      image: "ghcr.io/safwyls/flameagent:latest",
      previousImage: "ghcr.io/safwyls/flameagent:old",
    });
    renderWithAgent();

    expect(await screen.findByText(/cannot run the game/i)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /Rebuild the agent/i }));

    // Removing and recreating a container is worth confirming, and the
    // dialog has to say the world survives it.
    expect(await screen.findByText(/not touched/i)).toBeInTheDocument();
    expect(rebuild).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: "Rebuild agent" }));
    await waitFor(() => expect(rebuild).toHaveBeenCalledWith(1, "latest"));
    expect(toastSuccess).toHaveBeenCalledWith("Agent rebuilt", expect.anything());
  });

  it("does not offer a rebuild when the image can already run the game", async () => {
    vi.spyOn(api, "serverLaunch").mockResolvedValue(wineLaunch);
    renderWithAgent();

    await screen.findByText("Launch mode");
    expect(screen.queryByRole("button", { name: /Rebuild/i })).not.toBeInTheDocument();
  });

  it("reports a custom command honestly", async () => {
    vi.spyOn(api, "serverLaunch").mockResolvedValue({
      ...wineLaunch,
      profile: "custom",
      label: "Custom command",
      available: [],
    });
    renderWithAgent();

    expect(await screen.findByText(/Custom launch command/i)).toBeInTheDocument();
  });

  it("shows no launch row for an agent that doesn't run the game", async () => {
    // Companion mode answers 400 — there is nothing being launched, so the
    // control should be absent rather than broken.
    vi.spyOn(api, "serverLaunch").mockRejectedValue(new ApiError(400, "this agent does not run the game"));
    renderWithAgent();

    await screen.findByRole("button", { name: "Stop" });
    expect(screen.queryByText("Launch mode")).not.toBeInTheDocument();
  });
});

describe("ServerPower on-demand save", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    toastSuccess.mockClear();
    toastError.mockClear();
    vi.spyOn(api, "containerStatus").mockResolvedValue(runningState);
  });

  it("saves the world from the power row while capability is unknown", async () => {
    const save = vi.spyOn(api, "save").mockResolvedValue(undefined);
    renderPower();

    await userEvent.click(await screen.findByRole("button", { name: "Save world" }));

    await waitFor(() => expect(save).toHaveBeenCalledWith(1));
    expect(toastSuccess).toHaveBeenCalledWith("World saved");
  });

  it("relays the game's own reason when the save is refused", async () => {
    const reason = "the server autosaves every 10 minutes and saves on shutdown";
    vi.spyOn(api, "save").mockRejectedValue(new ApiError(501, reason));
    renderPower();

    await userEvent.click(await screen.findByRole("button", { name: "Save world" }));

    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith("Save failed", { description: reason }),
    );
  });

  it("offers save-then-stop in the stop dialog, stopping only after the save lands", async () => {
    const save = vi.spyOn(api, "save").mockResolvedValue(undefined);
    const act = vi.spyOn(api, "containerAction").mockResolvedValue(runningState);
    renderPower();

    await userEvent.click(await screen.findByRole("button", { name: "Stop" }));
    await userEvent.click(await screen.findByRole("button", { name: "Save world, then stop" }));

    await waitFor(() => expect(act).toHaveBeenCalledWith(1, "stop"));
    expect(save).toHaveBeenCalledWith(1);
    // The save must land before the stop is even asked for.
    expect(save.mock.invocationCallOrder[0]).toBeLessThan(act.mock.invocationCallOrder[0]);
  });

  it("does not stop when the pre-stop save fails", async () => {
    vi.spyOn(api, "save").mockRejectedValue(new ApiError(502, "agent unreachable"));
    const act = vi.spyOn(api, "containerAction").mockResolvedValue(runningState);
    renderPower();

    await userEvent.click(await screen.findByRole("button", { name: "Stop" }));
    await userEvent.click(await screen.findByRole("button", { name: "Save world, then stop" }));

    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(act).not.toHaveBeenCalled();
    // The dialog stays open so plain Stop is still available.
    expect(screen.getByRole("button", { name: "Save world, then stop" })).toBeInTheDocument();
  });

  it("hides Save world when the probe says this server can never save", async () => {
    // Enshrouded's stable answer: no on-demand save exists, and the game
    // saves on shutdown anyway. A permanently disabled button would be
    // furniture; the honest rendering is absence.
    const reason = "the server autosaves every 10 minutes and saves on shutdown";
    vi.spyOn(api, "serverCapabilities").mockResolvedValue({
      probed: true,
      commands: { save: { supported: false, reason } },
    });
    const save = vi.spyOn(api, "save").mockResolvedValue(undefined);
    renderPower();

    await screen.findByRole("button", { name: "Stop" });
    await waitFor(() => expect(screen.queryByRole("button", { name: "Save world" })).not.toBeInTheDocument());

    // And the stop dialog stops offering a save-first path that could only
    // ever fail.
    await userEvent.click(screen.getByRole("button", { name: "Stop" }));
    await screen.findByText(/Stop the server\?/i);
    expect(screen.queryByRole("button", { name: /Save world, then/ })).not.toBeInTheDocument();
    expect(save).not.toHaveBeenCalled();
  });

  it("stays optimistic while the capability answer is unknown", async () => {
    // A game that can't be probed, which is what every game was before the
    // probe existed. Hiding the control would be the wrong call: the command
    // explains itself if it turns out to be unavailable.
    vi.spyOn(api, "serverCapabilities").mockResolvedValue({ probed: false, commands: {} });
    const save = vi.spyOn(api, "save").mockResolvedValue(undefined);
    renderPower();

    await userEvent.click(await screen.findByRole("button", { name: "Save world" }));
    await waitFor(() => expect(save).toHaveBeenCalledWith(1));
  });

  it("keeps the save control for agent-managed servers whose docker power is off", async () => {
    vi.spyOn(api, "containerStatus").mockRejectedValue(
      new ApiError(400, "docker power control not configured"),
    );
    vi.spyOn(api, "steamUpdateStatus").mockResolvedValue({
      job: null,
      agent: { version: "test", apiVersion: 1, mode: "supervisor", installDirOk: true, diskFreeBytes: 0 },
    });
    renderWithProviders(<ServerPower serverId={1} agentUrl="http://agent:8811" />);

    expect(await screen.findByRole("button", { name: "Save world" })).toBeInTheDocument();
    // The docker power buttons stay hidden — save is the only lever here.
    expect(screen.queryByRole("button", { name: "Stop" })).not.toBeInTheDocument();
  });
});
