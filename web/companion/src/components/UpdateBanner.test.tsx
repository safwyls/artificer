import { describe, expect, it, vi, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { api } from "../lib/api";
import { UpdateBanner } from "./UpdateBanner";
import { renderWithProviders } from "../test/utils";
import type { UpdateState } from "../lib/types";
import type { ShellUpdate } from "../lib/runtime";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn(), loading: vi.fn() },
}));

const update = (o: Partial<UpdateState> = {}): UpdateState => ({
  available: true,
  version: "abcdef123456",
  supported: true,
  ...o,
});

afterEach(() => {
  vi.restoreAllMocks();
  delete window.companion;
});

/**
 * A shell whose updater is wired, with a status the test controls.
 *
 * The installed app does not poll the daemon for updates — the daemon
 * does not watch for them at all, because what gets replaced is the
 * application it lives inside. Status is pushed from the shell, so the
 * fake has to push too.
 */
function shellWithUpdate(initial: ShellUpdate) {
  let push: ((s: ShellUpdate) => void) | undefined;
  const bridge = {
    baseUrl: "http://127.0.0.1:41234",
    token: "tok",
    pickFolder: vi.fn(async () => null),
    openPath: vi.fn(async () => {}),
    updateStatus: vi.fn(async () => initial),
    checkForUpdate: vi.fn(async () => initial),
    downloadUpdate: vi.fn(async () => {}),
    installUpdate: vi.fn(async () => {}),
    onUpdateStatus: vi.fn((fn: (s: ShellUpdate) => void) => {
      push = fn;
      return () => {
        push = undefined;
      };
    }),
  };
  window.companion = bridge;
  return { bridge, push: (s: ShellUpdate) => push?.(s) };
}

describe("UpdateBanner", () => {
  it("stays out of the way when there is nothing to offer", () => {
    const { container } = renderWithProviders(<UpdateBanner update={update({ available: false })} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("says nothing at all before the first check has answered", () => {
    const { container } = renderWithProviders(<UpdateBanner update={undefined} />);
    expect(container).toBeEmptyDOMElement();
  });

  // Every build is stamped with a commit SHA and SHAs have no order, so
  // the honest claim is "different", not "newer".
  it("names the build without claiming it is newer", () => {
    renderWithProviders(<UpdateBanner update={update()} />);
    expect(screen.getByText("A different companion build is available.")).toBeInTheDocument();
    expect(screen.getByText("abcdef123456")).toBeInTheDocument();
    expect(screen.queryByText(/newer/i)).not.toBeInTheDocument();
  });

  // An install that cannot write to its own folder (Program Files
  // without elevation) would fail on the button, so it doesn't get one.
  it("explains rather than offering a button that cannot work", () => {
    renderWithProviders(
      <UpdateBanner
        update={update({ supported: false, why: "this companion lives somewhere it cannot write to" })}
      />,
    );
    expect(screen.getByText(/cannot write to/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Update now" })).not.toBeInTheDocument();
  });

  it("applies on request and says the companion is restarting", async () => {
    const apply = vi.spyOn(api, "applyUpdate").mockResolvedValue({ restarting: true });
    renderWithProviders(<UpdateBanner update={update()} />);
    await userEvent.click(screen.getByRole("button", { name: "Update now" }));
    await waitFor(() => expect(apply).toHaveBeenCalled());
    expect(await screen.findByRole("button", { name: "Updating…" })).toBeInTheDocument();
  });

  // A refused update must leave the button usable — the reason is often
  // "a save transfer is running", which stops being true a minute later.
  it("comes back from a refusal", async () => {
    vi.spyOn(api, "applyUpdate").mockRejectedValue(new Error("a save transfer is running"));
    renderWithProviders(<UpdateBanner update={update()} />);
    await userEvent.click(screen.getByRole("button", { name: "Update now" }));
    expect(await screen.findByRole("button", { name: "Update now" })).toBeEnabled();
  });

  it("shows an apply already running as in progress", () => {
    renderWithProviders(<UpdateBanner update={update({ applying: true })} />);
    expect(screen.getByRole("button", { name: "Updating…" })).toBeDisabled();
  });
});

// The installed app's half. Everything here goes through the shell,
// because electron-updater is what can actually replace an installed
// application — the daemon is one file inside it.
describe("UpdateBanner — the installed app", () => {
  it("says nothing when there is nothing to do about it", async () => {
    shellWithUpdate({ state: "current", version: "abcdef123456" });
    const { container } = renderWithProviders(<UpdateBanner update={undefined} />);
    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });

  // A build that cannot update itself does not need to announce that on
  // every screen — Diagnostics carries it.
  it("stays quiet on a build that cannot update itself", async () => {
    shellWithUpdate({ state: "unsupported", why: "macOS builds are unsigned" });
    const { container } = renderWithProviders(<UpdateBanner update={undefined} />);
    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });

  // Downloading is a separate step from installing, and offered rather
  // than taken: it is ~90MB on a connection this app knows nothing about.
  it("offers the download first, and the install only once it is ready", async () => {
    const { bridge, push } = shellWithUpdate({ state: "available", version: "abcdef123456" });
    renderWithProviders(<UpdateBanner update={undefined} />);

    const download = await screen.findByRole("button", { name: "Download update" });
    await userEvent.click(download);
    expect(bridge.downloadUpdate).toHaveBeenCalled();
    expect(bridge.installUpdate).not.toHaveBeenCalled();

    push({ state: "ready", version: "abcdef123456" });
    const install = await screen.findByRole("button", { name: "Install and reopen" });
    await userEvent.click(install);
    expect(bridge.installUpdate).toHaveBeenCalled();
  });

  // "It felt fast" is not a measurement. The byte counts are shown
  // because they are the only place a differential update is visible: one
  // that pulls the whole ~82MB installer behaves exactly like one that
  // pulls two megabytes.
  it("shows how much of the update is actually being downloaded", async () => {
    const { push } = shellWithUpdate({ state: "available", version: "0.2.2" });
    renderWithProviders(<UpdateBanner update={undefined} />);
    await screen.findByRole("button", { name: "Download update" });

    push({ state: "downloading", version: "0.2.2", percent: 50, transferred: 1_000_000, total: 2_000_000 });
    expect(await screen.findByText(/1(\.0)? MB of 2(\.0)? MB/)).toBeInTheDocument();

    push({ state: "ready", version: "0.2.2", total: 2_000_000 });
    expect(await screen.findByText(/2(\.0)? MB downloaded/)).toBeInTheDocument();
  });

  it("shows progress while it downloads, and does not offer a second click", async () => {
    const { push } = shellWithUpdate({ state: "available", version: "abcdef123456" });
    renderWithProviders(<UpdateBanner update={undefined} />);
    await screen.findByRole("button", { name: "Download update" });

    push({ state: "downloading", version: "abcdef123456", percent: 42 });
    expect(await screen.findByText(/downloading 42%/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Downloading…" })).toBeDisabled();
  });

  // Updates are a convenience and never fatal — but a failure the player
  // was watching for has to be visible, not swallowed.
  it("says why an update they asked for failed", async () => {
    shellWithUpdate({ state: "error", version: "abcdef123456", why: "net::ERR_CONNECTION_RESET" });
    renderWithProviders(<UpdateBanner update={undefined} />);
    expect(await screen.findByText(/net::ERR_CONNECTION_RESET/)).toBeInTheDocument();
  });

  // The daemon's own update state is the *browser* build's, and inside
  // the shell it is not watched at all. Rendering it here would be a
  // second answer to a question with one owner.
  it("ignores the daemon's update state entirely", async () => {
    shellWithUpdate({ state: "current" });
    const { container } = renderWithProviders(
      <UpdateBanner update={{ available: true, version: "deadbeef", supported: true }} />,
    );
    await waitFor(() => expect(container).toBeEmptyDOMElement());
  });
});
