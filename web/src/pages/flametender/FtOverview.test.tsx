import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen } from "@testing-library/react";
import { Route, Routes } from "react-router-dom";
import { FtOverview } from "./FtOverview";
import { api, type ServerInfo } from "../../lib/api";
import { makeServer, renderWithProviders } from "../../test/utils";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("../../lib/auth", () => ({
  useAuth: () => ({
    username: "admin",
    isAdmin: true,
    can: () => false,
    logout: vi.fn(),
  }),
}));

// What the Overview claims about a live server, now that two sources
// describe one: the game's own Steam query and the log tail. The three
// facts under test are the ones the query brought — the real slot count,
// the running build, and the difference between a process that is up and
// one that will let anyone in.

function info(over: Partial<ServerInfo> = {}): ServerInfo {
  return {
    servername: "Grimwood Bastion",
    version: "",
    playerCount: 0,
    transport: "agent",
    ...over,
  };
}

function renderOverview() {
  return renderWithProviders(
    <Routes>
      <Route path="/servers/:serverID" element={<FtOverview />} />
    </Routes>,
    { route: "/servers/1" },
  );
}

function stub(
  overrides: { info?: ServerInfo | Error; maxplayernum?: number } = {},
) {
  vi.spyOn(api, "getServer").mockResolvedValue(
    makeServer({ game: "enshrouded", gamePort: 15637 }),
  );
  if (overrides.info instanceof Error) {
    vi.spyOn(api, "serverInfo").mockRejectedValue(overrides.info);
  } else {
    vi.spyOn(api, "serverInfo").mockResolvedValue(overrides.info ?? info());
  }
  vi.spyOn(api, "serverPlayers").mockResolvedValue([]);
  vi.spyOn(api, "serverMetrics").mockResolvedValue({
    serverfps: 0,
    serverframetime: 0,
    currentplayernum: 0,
    maxplayernum: overrides.maxplayernum ?? 16,
    uptime: 3600,
    days: 1,
  });
  vi.spyOn(api, "serverActivity").mockResolvedValue({
    events: [],
    hours: 24,
    intervalSeconds: 30,
  });
  vi.spyOn(api, "listBackups").mockResolvedValue({
    snapshots: [],
    intervalHours: 0,
    keep: 3,
    running: false,
    available: true,
    totalBytes: 0,
    lastFailure: null,
  });
}

describe("FtOverview", () => {
  beforeEach(() => vi.restoreAllMocks());

  // A booting server binds its port and reads "running" well before it
  // accepts a connection. Calling that Online sends a friend to an error.
  it("separates a server that is starting from one that is accepting joins", async () => {
    stub({ info: info({ readiness: "starting" }) });
    renderOverview();

    expect(await screen.findByText("Starting")).toBeInTheDocument();
    expect(screen.queryByText("Online")).not.toBeInTheDocument();
    expect(
      screen.getByText(/won't accept joins until it finishes/i),
    ).toBeInTheDocument();
  });

  it("says Online once the server has logged that it is up", async () => {
    stub({ info: info({ readiness: "ready" }) });
    renderOverview();

    expect(await screen.findByText("Online")).toBeInTheDocument();
    expect(screen.queryByText("Starting")).not.toBeInTheDocument();
  });

  // The console can't always tell — the readiness marker is logged once
  // at boot and scrolls out of the agent's ring. Unknown must read as
  // Online, not as a server perpetually starting.
  it("does not claim a long-running server is still starting", async () => {
    stub({ info: info({ readiness: "" }) });
    renderOverview();

    expect(await screen.findByText("Online")).toBeInTheDocument();
    expect(screen.queryByText("Starting")).not.toBeInTheDocument();
  });

  // The slot count is the fact only the query carries. Drawing against
  // the 16-slot hard cap made a full 4-slot server look quarter-empty.
  it("uses the server's own slot count rather than the game's cap", async () => {
    stub({ info: info({ playerCount: 3 }), maxplayernum: 4 });
    renderOverview();

    expect(await screen.findByText("4 slots")).toBeInTheDocument();
    expect(screen.getByText("/ 4")).toBeInTheDocument();
    expect(screen.getByText("1 slots open")).toBeInTheDocument();
  });

  it("falls back to naming the cap when no slot count was reported", async () => {
    stub({ maxplayernum: 16 });
    renderOverview();

    expect(await screen.findByText("16-slot cap")).toBeInTheDocument();
  });

  // The build is what turns a friend's version-mismatch join error from a
  // mystery into a comparison.
  it("shows the running build when the game reports one", async () => {
    stub({ info: info({ version: "1024233" }) });
    renderOverview();

    expect(await screen.findByText("build 1024233")).toBeInTheDocument();
  });

  it("shows no build chip when nothing reported one", async () => {
    stub();
    renderOverview();

    await screen.findByText("Online");
    expect(screen.queryByText(/^build /)).not.toBeInTheDocument();
  });

  it("reads the player count off the server rather than the roster", async () => {
    // The log names nobody; the game says three people are on. The count
    // is the game's.
    stub({
      info: info({ playerCount: 3, transport: "agent+a2s" }),
      maxplayernum: 16,
    });
    renderOverview();

    await screen.findByText("Online");
    expect(screen.getByText("13 slots open")).toBeInTheDocument();
  });

  it("says the flame is out when the server is unreachable", async () => {
    stub({ info: new Error("server process is stopped") });
    renderOverview();

    expect(await screen.findByText("Offline")).toBeInTheDocument();
    expect(screen.getByText(/flame is out/i)).toBeInTheDocument();
  });
});
