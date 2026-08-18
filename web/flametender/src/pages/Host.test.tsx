import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { api, type HostOverview } from "../lib/api";
import { renderWithProviders } from "../test/utils";
import { Host, containerKind, formatBytes } from "./Host";

/** The fixture's container names are arbitrary — the page is game-
 * agnostic, and registered/orphan/foreign come from the API's own booleans,
 * not from which console this test happens to live in. */
const OVERVIEW: HostOverview = {
  available: true,
  anvilURL: "http://anvil:8410",
  health: {
    service: "anvil",
    version: "1.4.0",
    client: "flametender",
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
  ],
  images: [
    {
      id: "sha256:pal1",
      tags: ["ghcr.io/safwyls/palagent:latest"],
      size: 900_000_000,
      created: 1_755_000_000,
      containers: ["palagent-palhalla", "palagent-lost"],
    },
    // Untagged but pinned: a managed container was created from it before
    // the :latest tag moved on.
    { id: "sha256:old1", tags: [], size: 870_000_000, created: 1_754_000_000, containers: ["wkagent-ashenfall"] },
  ],
};

describe("Host", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  // Container names also appear in the images table's "Used by" column, so
  // container-row assertions scope themselves to the first table.
  const containersTable = () => within(screen.getAllByRole("table")[0]);

  it("shows the managed containers and links registered ones to their server", async () => {
    vi.spyOn(api, "hostOverview").mockResolvedValue(OVERVIEW);
    renderWithProviders(<Host />);

    const link = await screen.findByRole("link", { name: "Palhalla" });
    expect(link).toHaveAttribute("href", "/servers/5");

    // The orphan is flagged, not silently listed — surfacing it is the
    // page's reason to exist.
    expect(screen.getByText(/not registered — adoptable/)).toBeInTheDocument();
    // Docker's status sentence carries the exit code and age.
    expect(screen.getByText("Exited (137) 2 days ago")).toBeInTheDocument();
    // Another console's managed row is visible context.
    expect(containersTable().getByText("wkagent-ashenfall")).toBeInTheDocument();
    expect(containersTable().getByText("wildskeeper")).toBeInTheDocument();
  });

  it("filters the container list by name, image or server", async () => {
    vi.spyOn(api, "hostOverview").mockResolvedValue(OVERVIEW);
    const user = userEvent.setup();
    renderWithProviders(<Host />);

    await screen.findByRole("link", { name: "Palhalla" });
    await user.type(screen.getByPlaceholderText("Filter containers…"), "ashen");
    expect(containersTable().getByText("wkagent-ashenfall")).toBeInTheDocument();
    expect(containersTable().queryByText("palagent-palhalla")).not.toBeInTheDocument();

    // Server name matches too, so an operator can search what they see.
    await user.clear(screen.getByPlaceholderText("Filter containers…"));
    await user.type(screen.getByPlaceholderText("Filter containers…"), "palhalla");
    expect(containersTable().getByText("palagent-palhalla")).toBeInTheDocument();
    expect(containersTable().queryByText("wkagent-ashenfall")).not.toBeInTheDocument();
  });

  it("sorts containers by state on demand, running first", async () => {
    vi.spyOn(api, "hostOverview").mockResolvedValue(OVERVIEW);
    const user = userEvent.setup();
    renderWithProviders(<Host />);

    await screen.findByRole("link", { name: "Palhalla" });
    await user.click(screen.getByRole("button", { name: /^State/ }));
    const names = containersTable()
      .getAllByRole("row")
      .slice(1) // header row
      .map((r) => within(r).getAllByRole("cell")[0].textContent);
    // Running rows lead; the exited orphan lands last.
    expect(names[names.length - 1]).toContain("palagent-lost");
  });

  it("shows images with their disk cost and flags the untagged one", async () => {
    vi.spyOn(api, "hostOverview").mockResolvedValue(OVERVIEW);
    renderWithProviders(<Host />);

    await waitFor(() => expect(screen.getByText("untagged")).toBeInTheDocument());
    expect(screen.getByText("858 MB")).toBeInTheDocument(); // sha256:pal1
    // Summary tile: total bytes across both images.
    expect(screen.getByText("1.6 GB")).toBeInTheDocument();
    expect(screen.getByText("2 images")).toBeInTheDocument();
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
      health: { service: "anvil", version: "1.0.0", client: "flametender", dataRoot: "/d", dockerOk: true },
      containers: OVERVIEW.containers,
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
