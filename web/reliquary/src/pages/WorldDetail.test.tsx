import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Route, Routes } from "react-router-dom";
import { api } from "../lib/api";
import { resetArtCache } from "../lib/art";
import { WorldDetail } from "./WorldDetail";
import {
  makeHolder,
  makeStatus,
  makeVersion,
  makeWorld,
  renderWithProviders,
} from "../test/utils";

const auth = { username: "safwyl", isAdmin: true, canSync: true };
vi.mock("../lib/auth", () => ({ useAuth: () => auth }));

// The toasts are the app's whole status readout; the Toaster itself lives in
// the shell, so a page test watches the calls rather than the DOM.
const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
  loading: vi.fn(() => "id"),
}));
vi.mock("sonner", () => ({ toast }));

const show = (route = "/worlds/1") =>
  renderWithProviders(
    <Routes>
      <Route path="/worlds/:worldID" element={<WorldDetail />} />
    </Routes>,
    { route },
  );

describe("WorldDetail", () => {
  beforeEach(() => {
    resetArtCache();
    vi.spyOn(api, "artwork").mockResolvedValue({ art: {} });
    vi.spyOn(api, "world").mockResolvedValue({
      status: makeStatus({
        world: makeWorld({ headVersion: 41, gameTitle: "Palworld", agentUrl: "http://host:8420" }),
        holder: makeHolder({ username: "safwyl" }),
        head: makeVersion({ id: 41 }),
      }),
      versions: [
        makeVersion({ id: 41 }),
        makeVersion({ id: 40, conflict: true, uploaderId: 2 }),
        makeVersion({ id: 39, kind: "checkpoint" }),
      ],
      uploaders: { "2": "torv" },
    });
  });
  afterEach(() => vi.restoreAllMocks());

  it("badges the head, the conflict and the checkpoint", async () => {
    show();
    expect(await screen.findByText("HEAD")).toBeInTheDocument();
    expect(screen.getByText("CONFLICT")).toBeInTheDocument();
    expect(screen.getByText("CHECKPOINT")).toBeInTheDocument();
  });

  it("names the uploader the server resolved, and admits when it can't", async () => {
    show();
    expect(await screen.findByText("torv")).toBeInTheDocument();
    expect(screen.getAllByText("unknown").length).toBeGreaterThan(0);
  });

  it("offers Make head only for versions that aren't already it", async () => {
    show();
    await screen.findByText("HEAD");
    // Three versions, one of them the head.
    expect(screen.getAllByRole("button", { name: "Make head" })).toHaveLength(2);
  });

  it("moves the head only after the confirmation", async () => {
    const setHead = vi.spyOn(api, "setHead").mockResolvedValue(undefined);
    show();
    await screen.findByText("HEAD");
    await userEvent.click(screen.getAllByRole("button", { name: "Make head" })[0]);
    expect(setHead).not.toHaveBeenCalled();
    await userEvent.click(await screen.findByText("Make v40 the canonical head?"));
    const confirms = screen.getAllByRole("button", { name: "Make head" });
    await userEvent.click(confirms[confirms.length - 1]);
    await waitFor(() => expect(setHead).toHaveBeenCalledWith(1, 40));
  });

  // Regression: the settings API replaces the whole record, so a save that
  // omitted a field would clear it. Every tab sends all of it.
  it("sends the whole settings record, including the fields the tab doesn't show", async () => {
    const update = vi.spyOn(api, "updateWorld").mockResolvedValue(undefined as never);
    show("/worlds/1?tab=settings");
    const name = await screen.findByDisplayValue("Emberfall");
    await userEvent.clear(name);
    await userEvent.type(name, "Embervale");
    await userEvent.click(screen.getByRole("button", { name: "Save settings" }));
    await waitFor(() => expect(update).toHaveBeenCalled());
    expect(update.mock.calls[0][1]).toEqual({
      name: "Embervale",
      leaseHours: 6,
      maxBytes: 1 << 30,
      keepVersions: 20,
      checkpoints: false,
      savePath: "",
      webhookUrl: "",
      // The server link is not on this tab and must survive the save.
      agentUrl: "http://host:8420",
      // Never returned by the API; empty keeps what is stored.
      agentToken: "",
    });
  });

  it("keeps the server link's own fields on the server tab", async () => {
    show("/worlds/1?tab=server");
    expect(await screen.findByDisplayValue("http://host:8420")).toBeInTheDocument();
    // The token is never returned, so the field starts empty and says that
    // leaving it empty keeps what is stored.
    expect(screen.getByLabelText(/Agent token/)).toHaveValue("");
  });

  it("offers to take the world back when the dedicated server is holding it", async () => {
    vi.spyOn(api, "world").mockResolvedValue({
      status: makeStatus({
        world: makeWorld({ headVersion: 41, agentUrl: "http://host:8420" }),
        holder: makeHolder({ username: "the server", serverHeld: true }),
        head: makeVersion({ id: 41 }),
      }),
      versions: [],
      uploaders: {},
    });
    show("/worlds/1?tab=server");
    expect(
      await screen.findByRole("button", { name: "Take back from server" }),
    ).toBeInTheDocument();
  });

  // Regression: a check-in against a hold that can no longer move the head
  // is accepted and *flagged*, not refused. Saying only "uploaded" would
  // let the uploader walk away believing their save is the head.
  it("says so when a check-in is flagged as a conflict", async () => {
    vi.spyOn(api, "checkin").mockResolvedValue({ version: makeVersion({ conflict: true }) });
    show();
    await screen.findByText("HEAD");
    const file = new File(["tar"], "save.tar", { type: "application/x-tar" });
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await userEvent.upload(input, file);
    await waitFor(() =>
      expect(toast.warning).toHaveBeenCalledWith(
        "checked in, but flagged as a conflict — pick a head in the history",
        expect.anything(),
      ),
    );
  });

  it("checks in against the holder's own session", async () => {
    const checkin = vi.spyOn(api, "checkin").mockResolvedValue({});
    show();
    await screen.findByText("HEAD");
    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await userEvent.upload(input, new File(["tar"], "save.tar"));
    await waitFor(() => expect(checkin).toHaveBeenCalledWith(7, expect.any(File)));
  });

  it("hides the admin tabs from an ordinary holder", async () => {
    auth.isAdmin = false;
    try {
      show();
      await screen.findByText("HEAD");
      expect(screen.queryByRole("tab", { name: "Settings" })).not.toBeInTheDocument();
      expect(screen.queryByRole("tab", { name: "Server link" })).not.toBeInTheDocument();
      expect(screen.getByRole("tab", { name: "History" })).toBeInTheDocument();
    } finally {
      auth.isAdmin = true;
    }
  });
});
