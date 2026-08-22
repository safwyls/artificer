import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
import { api } from "../lib/api";
import { resetArtCache } from "../lib/art";
import { Worlds } from "./Worlds";
import { makeStatus, makeWorld, renderWithProviders } from "../test/utils";

const auth = { username: "guest", isAdmin: false, canSync: false };
vi.mock("../lib/auth", () => ({ useAuth: () => auth }));

describe("Worlds — the read-only shelf", () => {
  beforeEach(() => {
    resetArtCache();
    vi.spyOn(api, "artwork").mockResolvedValue({ art: {} });
    vi.spyOn(api, "worlds").mockResolvedValue({
      worlds: [makeStatus({ world: makeWorld({ name: "Emberfall" }) })],
    });
  });
  afterEach(() => vi.restoreAllMocks());

  it("tells an account without custody what it can and cannot do", async () => {
    renderWithProviders(<Worlds />);
    expect(await screen.findByText(/but not hold one/)).toBeInTheDocument();
  });

  it("hides every mutating affordance rather than disabling it", async () => {
    renderWithProviders(<Worlds />);
    await screen.findAllByText("Emberfall");
    expect(screen.queryByRole("button", { name: "+ New world" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Check out" })).not.toBeInTheDocument();
    // The companion strip is for people who can hold a world.
    expect(screen.queryByRole("link", { name: "Open Companion" })).not.toBeInTheDocument();
  });

  it("says the shelf is empty rather than showing nothing at all", async () => {
    vi.spyOn(api, "worlds").mockResolvedValue({ worlds: [] });
    renderWithProviders(<Worlds />);
    expect(await screen.findByText(/No worlds yet/)).toBeInTheDocument();
  });
});

describe("Worlds — with world custody", () => {
  beforeEach(() => {
    auth.canSync = true;
    resetArtCache();
    vi.spyOn(api, "artwork").mockResolvedValue({ art: {} });
    vi.spyOn(api, "worlds").mockResolvedValue({
      worlds: [makeStatus({ world: makeWorld({ name: "Emberfall" }) })],
    });
    vi.spyOn(api, "syncToken").mockResolvedValue({ token: "abc" });
  });
  afterEach(() => {
    auth.canSync = false;
    vi.restoreAllMocks();
  });

  it("offers the new-world form and points at the Companion page", async () => {
    renderWithProviders(<Worlds />);
    expect(await screen.findByRole("button", { name: "+ New world" })).toBeInTheDocument();
    expect(await screen.findByText("Your companion token is minted.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open Companion" })).toHaveAttribute(
      "href",
      "/companion",
    );
  });

  it("notices when no token has been minted yet", async () => {
    vi.spyOn(api, "syncToken").mockResolvedValue({});
    renderWithProviders(<Worlds />);
    expect(await screen.findByText("You have no companion token yet.")).toBeInTheDocument();
  });
});
