import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import { api, type HostOverview } from "../lib/api";
import { renderWithProviders } from "../test/utils";
import { Host, containerKind, formatBytes } from "./Host";

const OVERVIEW: HostOverview = {
  available: true,
  anvilURL: "http://anvil:8410",
  health: {
    service: "anvil",
    version: "1.4.0",
    client: "palcon",
    dataRoot: "/mnt/tank/pal",
    dockerOk: true,
  },
  containers: [
    {
      name: "palagent-palhalla",
      image: "ghcr.io/safwyls/palagent:latest",
      running: true,
      state: "running",
      status: "Up 3 hours",
      managed: true,
      mine: true,
      slug: "palhalla",
      dataDir: "/mnt/tank/pal/palhalla",
      ports: [{ host: 8211, container: 8211, proto: "udp" }],
      serverId: 5,
      serverName: "Palhalla",
    },
    {
      // Ours per Anvil, registered nowhere: the orphan case.
      name: "palagent-lost",
      image: "ghcr.io/safwyls/palagent:latest",
      running: false,
      state: "exited",
      status: "Exited (137) 2 days ago",
      managed: true,
      mine: true,
      slug: "lost",
    },
    {
      name: "wkagent-ashenfall",
      image: "ghcr.io/safwyls/wkagent:latest",
      running: true,
      state: "running",
      managed: true,
      mine: false,
      owner: "wildskeeper",
    },
    {
      name: "nginx",
      image: "nginx:latest",
      running: true,
      state: "running",
      managed: false,
      mine: false,
    },
  ],
  ports: [{ port: 8211, proto: "udp", container: "palagent-palhalla" }],
  images: [
    {
      id: "sha256:pal1",
      tags: ["ghcr.io/safwyls/palagent:latest"],
      size: 900_000_000,
      created: 1_755_000_000,
      containers: ["palagent-palhalla", "palagent-lost"],
    },
    { id: "sha256:old1", tags: [], size: 870_000_000, created: 1_754_000_000, containers: [] },
  ],
};

describe("Host", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("shows every container on the host and links registered ones to their server", async () => {
    vi.spyOn(api, "hostOverview").mockResolvedValue(OVERVIEW);
    renderWithProviders(<Host />);

    const link = await screen.findByRole("link", { name: "Palhalla" });
    expect(link).toHaveAttribute("href", "/servers/5");

    // The orphan is flagged, not silently listed — surfacing it is the
    // page's reason to exist.
    expect(screen.getByText(/not registered — adoptable/)).toBeInTheDocument();
    // Docker's status sentence carries the exit code and age.
    expect(screen.getByText("Exited (137) 2 days ago")).toBeInTheDocument();
    // Foreign and unmanaged rows are visible context.
    expect(screen.getByText("wkagent-ashenfall")).toBeInTheDocument();
    expect(screen.getByText("wildskeeper")).toBeInTheDocument();
    expect(screen.getByText("not managed by Anvil")).toBeInTheDocument();
  });

  it("shows images with their disk cost and flags the dangling one", async () => {
    vi.spyOn(api, "hostOverview").mockResolvedValue(OVERVIEW);
    renderWithProviders(<Host />);

    await waitFor(() => expect(screen.getByText("dangling")).toBeInTheDocument());
    expect(screen.getByText("858 MB")).toBeInTheDocument(); // sha256:pal1
    // Summary tile: total bytes across both images, with the dangling count.
    expect(screen.getByText("1.6 GB")).toBeInTheDocument();
    expect(screen.getByText("2 images, 1 dangling")).toBeInTheDocument();
  });

  it("says why the dashboard is unavailable instead of rendering an empty host", async () => {
    vi.spyOn(api, "hostOverview").mockResolvedValue({
      available: false,
      reason: "this console is not connected to an Anvil host service",
    });
    renderWithProviders(<Host />);

    await waitFor(() =>
      expect(screen.getByText(/not connected to an Anvil host service/)).toBeInTheDocument(),
    );
    expect(screen.queryByText("Containers")).not.toBeInTheDocument();
  });

  it("keeps the sections that worked when one read fails", async () => {
    vi.spyOn(api, "hostOverview").mockResolvedValue({
      available: true,
      anvilURL: "http://anvil:8410",
      health: { service: "anvil", version: "1.0.0", client: "palcon", dataRoot: "/d", dockerOk: true },
      containers: OVERVIEW.containers,
      ports: [],
      imagesError: "this Anvil does not report images yet — upgrade it",
    });
    renderWithProviders(<Host />);

    await screen.findByRole("link", { name: "Palhalla" });
    expect(screen.getByText(/does not report images yet/)).toBeInTheDocument();
  });
});

describe("containerKind", () => {
  const base = { name: "x", image: "i", running: true, managed: true, mine: true };
  it("tells registered, orphan, foreign and unmanaged apart", () => {
    expect(containerKind({ ...base, serverId: 1, serverName: "S" })).toBe("registered");
    expect(containerKind(base)).toBe("orphan");
    expect(containerKind({ ...base, mine: false, owner: "wildskeeper" })).toBe("foreign");
    expect(containerKind({ ...base, managed: false, mine: false })).toBe("unmanaged");
  });
});

describe("formatBytes", () => {
  it("scales to a readable unit", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(512)).toBe("512 B");
    expect(formatBytes(900_000_000)).toBe("858 MB");
    expect(formatBytes(1_770_000_000)).toBe("1.6 GB");
  });
});
