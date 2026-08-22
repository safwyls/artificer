import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { api } from "./lib/api";
import { App } from "./App";
import { makeGame, makeLink, makeState, makeSyncWorld, renderWithProviders } from "./test/utils";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn(), loading: vi.fn() },
}));

beforeEach(() => {
  vi.spyOn(api, "artwork").mockResolvedValue({ art: {} });
  vi.spyOn(api, "saveHints").mockResolvedValue({ available: true, known: 5 });
  vi.spyOn(api, "splitSavePath").mockResolvedValue({ split: null });
});
afterEach(() => vi.restoreAllMocks());

describe("App — setup", () => {
  it("shows the connect card and the game-finding card side by side, not an empty page", async () => {
    vi.spyOn(api, "state").mockResolvedValue(
      makeState({
        config: { serverUrl: "", tokenSet: false, steamDirs: [] },
        sync: { configured: false, busy: false },
      }),
    );
    renderWithProviders(<App />);
    expect(await screen.findByText("Connect to your vault")).toBeInTheDocument();
    expect(screen.getByText("Finding your games")).toBeInTheDocument();
    expect(screen.getByText("Not connected")).toBeInTheDocument();
    // Nothing leaves this machine until it is connected — said out loud.
    expect(screen.getByText(/Nothing leaves this machine until you connect/)).toBeInTheDocument();
  });

  it("saves the URL and token together, and never sends an empty token", async () => {
    vi.spyOn(api, "state").mockResolvedValue(
      makeState({
        config: { serverUrl: "", tokenSet: false, steamDirs: [] },
        sync: { configured: false, busy: false },
      }),
    );
    const setConfig = vi.spyOn(api, "setConfig").mockResolvedValue({});
    renderWithProviders(<App />);
    await userEvent.type(await screen.findByLabelText("Save-sync service URL"), "https://vault.example.test");
    await userEvent.type(screen.getByLabelText("Your sync token"), "tok123");
    await userEvent.click(screen.getByRole("button", { name: "Save & connect" }));
    await waitFor(() =>
      expect(setConfig).toHaveBeenCalledWith({
        serverUrl: "https://vault.example.test",
        token: "tok123",
      }),
    );
  });

  // The Steam card and the connect card save independently: the config
  // endpoint writes only the fields a request carries, so a saved token
  // survives a Steam-folder change.
  it("saves the Steam folder without touching the connection", async () => {
    vi.spyOn(api, "state").mockResolvedValue(
      makeState({
        config: { serverUrl: "", tokenSet: true, steamDirs: [] },
        sync: { configured: false, busy: false },
      }),
    );
    const setConfig = vi.spyOn(api, "setConfig").mockResolvedValue({});
    renderWithProviders(<App />);
    await userEvent.type(await screen.findByLabelText(/Steam folder/), "D:\\SteamLibrary");
    await userEvent.click(screen.getByRole("button", { name: "Save folder & rescan" }));
    await waitFor(() => expect(setConfig).toHaveBeenCalledWith({ steamDirs: ["D:\\SteamLibrary"] }));
  });

  it("clears the override rather than saving a blank folder", async () => {
    vi.spyOn(api, "state").mockResolvedValue(
      makeState({
        config: { serverUrl: "", tokenSet: true, steamDirs: ["D:\\Old"] },
        sync: { configured: false, busy: false },
      }),
    );
    const setConfig = vi.spyOn(api, "setConfig").mockResolvedValue({});
    renderWithProviders(<App />);
    await userEvent.clear(await screen.findByLabelText(/Steam folder/));
    await userEvent.click(screen.getByRole("button", { name: "Save folder & rescan" }));
    await waitFor(() => expect(setConfig).toHaveBeenCalledWith({ steamDirs: [] }));
  });
});

describe("App — connected", () => {
  const connected = () =>
    makeState({
      links: [makeLink()],
      discovered: { games: [makeGame()], probes: [] },
      sync: { configured: true, username: "safwyl", busy: false, worlds: [makeSyncWorld()] },
    });

  it("names who it is connected as, and both builds in the footer", async () => {
    vi.spyOn(api, "state").mockResolvedValue({
      ...connected(),
      sync: { ...connected().sync, serverVersion: "v1.4.2" },
    });
    renderWithProviders(<App />);
    expect(await screen.findByText("safwyl")).toBeInTheDocument();
    expect(screen.getByText("companion v1.4.2 · service v1.4.2")).toBeInTheDocument();
  });

  // A save-sync report that names one half names nothing — so an unknown
  // service version is said, not omitted.
  it("admits when it does not know the service's build", async () => {
    vi.spyOn(api, "state").mockResolvedValue(connected());
    renderWithProviders(<App />);
    expect(await screen.findByText("companion v1.4.2 · service version unknown")).toBeInTheDocument();
  });

  it("says what to do when nothing is linked yet", async () => {
    vi.spyOn(api, "state").mockResolvedValue({ ...connected(), links: [] });
    renderWithProviders(<App />);
    expect(await screen.findByText(/Nothing linked yet/)).toBeInTheDocument();
  });

  // Regression: form state must never be clobbered by the five-second
  // background refresh. The old page guaranteed this by keeping forms
  // only in a modal; here the dialog owns its own state.
  it("keeps a half-filled link form across a poll", async () => {
    vi.spyOn(api, "state").mockResolvedValue({ ...connected(), links: [] });
    const { queryClient } = renderWithProviders(<App />);
    await userEvent.click(await screen.findByRole("button", { name: "Link a folder by hand…" }));
    const field = await screen.findByLabelText("Save folder");
    await userEvent.type(field, "C:\\half-typed");
    await userEvent.type(screen.getByLabelText("New world's name"), "Newworld");

    // The poll lands, with fresh objects for everything.
    await queryClient.refetchQueries({ queryKey: ["state"] });
    await new Promise((r) => setTimeout(r, 10));

    expect(screen.getByLabelText("Save folder")).toHaveValue("C:\\half-typed");
    expect(screen.getByLabelText("New world's name")).toHaveValue("Newworld");
  });

  it("opens what a linked tile points at, rather than the link form", async () => {
    vi.spyOn(api, "state").mockResolvedValue(connected());
    renderWithProviders(<App />);
    await userEvent.click(await screen.findByRole("button", { name: /Enshrouded/ }));
    expect(await screen.findByText(/check it out and in from/)).toBeInTheDocument();
    expect(screen.queryByLabelText("World on the service")).not.toBeInTheDocument();
  });

  it("says plainly when the companion itself is not answering", async () => {
    vi.spyOn(api, "state").mockRejectedValue(new Error("connection refused"));
    renderWithProviders(<App />);
    expect(await screen.findByText(/companion is not answering on this machine/)).toBeInTheDocument();
  });
});
