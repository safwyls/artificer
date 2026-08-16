import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen } from "@testing-library/react";
import { Route, Routes } from "react-router-dom";
import { FtSaves } from "./FtSaves";
import { api, type BackupsResult } from "../../lib/api";
import { renderWithProviders } from "../../test/utils";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("../../lib/auth", () => ({
  useAuth: () => ({ username: "admin", isAdmin: true, can: () => true, logout: vi.fn() }),
}));

// The gap this covers: a snapshot runs detached from the click that starts
// it, so when it fails there is no request left to carry the reason. The
// page showed the running flag clearing and no new file, which reads as
// "nothing happened" — the symptom that hid a backup path broken since the
// transplant.

function backups(over: Partial<BackupsResult> = {}): BackupsResult {
  return {
    available: true,
    running: false,
    intervalHours: 0,
    keep: 3,
    snapshots: [],
    totalBytes: 0,
    lastFailure: null,
    ...over,
  };
}

function renderSaves() {
  return renderWithProviders(
    <Routes>
      <Route path="/servers/:serverID" element={<FtSaves />} />
    </Routes>,
    { route: "/servers/1" },
  );
}

describe("FtSaves", () => {
  beforeEach(() => vi.restoreAllMocks());

  it("says why the last snapshot wrote no file", async () => {
    vi.spyOn(api, "listBackups").mockResolvedValue(
      backups({
        lastFailure: {
          error: "the save directory holds no world files — has the server saved yet?",
          at: "2026-08-16T10:00:00Z",
        },
      }),
    );
    renderSaves();

    expect(await screen.findByText(/the last snapshot failed/i)).toBeInTheDocument();
    expect(screen.getByText(/holds no world files/i)).toBeInTheDocument();
  });

  // While one is in flight, the reason on screen belongs to the previous
  // attempt; showing it next to "in progress" would read as this one having
  // already failed.
  it("holds the reason back while a snapshot is running", async () => {
    vi.spyOn(api, "listBackups").mockResolvedValue(
      backups({ running: true, lastFailure: { error: "an older failure", at: "2026-08-16T10:00:00Z" } }),
    );
    renderSaves();

    expect(await screen.findByText(/Snapshot in progress/i)).toBeInTheDocument();
    expect(screen.queryByText(/an older failure/)).not.toBeInTheDocument();
  });

  it("shows no failure banner when the last snapshot worked", async () => {
    vi.spyOn(api, "listBackups").mockResolvedValue(
      backups({ snapshots: [{ name: "20260816-100000.zip", ts: "2026-08-16T10:00:00Z", bytes: 4096 }] }),
    );
    renderSaves();

    expect(await screen.findByText("20260816-100000.zip")).toBeInTheDocument();
    expect(screen.queryByText(/the last snapshot failed/i)).not.toBeInTheDocument();
  });
});
