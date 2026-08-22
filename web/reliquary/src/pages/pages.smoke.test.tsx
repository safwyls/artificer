import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Route, Routes } from "react-router-dom";
import { api } from "../lib/api";
import { AppShell } from "../components/AppShell";
import { Companion } from "./Companion";
import { AdminArtwork } from "./AdminArtwork";
import { AdminCatalogue } from "./AdminCatalogue";
import { renderWithProviders } from "../test/utils";

const auth = {
  username: "safwyl",
  isAdmin: true,
  canSync: true,
  logout: vi.fn(),
};
vi.mock("../lib/auth", () => ({ useAuth: () => auth }));
// The shell opens the custody stream on mount; the pages under test don't
// care whether it connects.
vi.mock("../lib/live", () => ({ useCustodyStream: () => ({ live: true }), POLL_MS: 20_000 }));

beforeEach(() => {
  vi.spyOn(api, "version").mockResolvedValue({ version: "v1.4.2" });
});
afterEach(() => vi.restoreAllMocks());

describe("AppShell", () => {
  it("names the build and the signed-in account, and shows the stream is live", async () => {
    renderWithProviders(
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/" element={<p>content</p>} />
        </Route>
      </Routes>,
    );
    // The shell renders its chrome twice — the phone header and the desktop
    // sidebar. The full "reliquary vX" string is the desktop footer's; the
    // phone header carries the bare build beside the wordmark.
    expect(await screen.findByText("reliquary v1.4.2")).toBeInTheDocument();
    expect(screen.getByText("v1.4.2")).toBeInTheDocument();
    expect(screen.getAllByText("safwyl")).toHaveLength(2);
    expect(screen.getByText("live")).toBeInTheDocument();
  });

  it("renders the admin destinations only for admins", async () => {
    const { unmount } = renderWithProviders(
      <Routes>
        <Route element={<AppShell />}>
          <Route path="/" element={<p>content</p>} />
        </Route>
      </Routes>,
    );
    expect(screen.getAllByRole("link", { name: /Users/ })).toHaveLength(2);
    unmount();

    auth.isAdmin = false;
    try {
      renderWithProviders(
        <Routes>
          <Route element={<AppShell />}>
            <Route path="/" element={<p>content</p>} />
          </Route>
        </Routes>,
      );
      expect(screen.queryByRole("link", { name: /Users/ })).not.toBeInTheDocument();
      expect(screen.queryByRole("link", { name: /Cover art/ })).not.toBeInTheDocument();
      expect(screen.getAllByRole("link", { name: /Worlds/ }).length).toBeGreaterThan(0);
    } finally {
      auth.isAdmin = true;
    }
  });
});

describe("Companion", () => {
  it("offers to mint when there is no token", async () => {
    vi.spyOn(api, "syncToken").mockResolvedValue({});
    renderWithProviders(<Companion />);
    expect(await screen.findByRole("button", { name: "Mint a token" })).toBeInTheDocument();
  });

  it("shows the token, its download link, and warns that reminting revokes", async () => {
    vi.spyOn(api, "syncToken").mockResolvedValue({ token: "s3cr3t" });
    renderWithProviders(<Companion />);
    expect(await screen.findByText("s3cr3t")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Download the companion" })).toHaveAttribute(
      "href",
      "/api/public/sync/s3cr3t/companion/download",
    );
    expect(screen.getByText(/minting a new one revokes the old everywhere/)).toBeInTheDocument();
  });

  it("names the bundled companion build, so a download can be told apart", async () => {
    vi.spyOn(api, "syncToken").mockResolvedValue({ token: "s3cr3t" });
    renderWithProviders(<Companion />);
    expect(await screen.findByText("v1.4.2")).toBeInTheDocument();
  });

  it("asks before revoking", async () => {
    const revoke = vi.spyOn(api, "revokeSyncToken").mockResolvedValue(undefined);
    vi.spyOn(api, "syncToken").mockResolvedValue({ token: "s3cr3t" });
    renderWithProviders(<Companion />);
    await userEvent.click(await screen.findByRole("button", { name: "Revoke" }));
    expect(await screen.findByText(/stops being able to reach the vault/)).toBeInTheDocument();
    expect(revoke).not.toHaveBeenCalled();
  });
});

describe("AdminArtwork", () => {
  it("distinguishes credentials saved here from the environment's", async () => {
    vi.spyOn(api, "artworkSettings").mockResolvedValue({
      status: { configured: true, clientId: "abc", lookups: 214, hits: 198, misses: 16, cached: 190 },
      stored: true,
      envConfigured: true,
    });
    renderWithProviders(<AdminArtwork />);
    expect(await screen.findByText(/saved here/)).toBeInTheDocument();
    expect(screen.getByText(/IGDB_CLIENT_ID\/SECRET are set in the environment/)).toBeInTheDocument();
    expect(screen.getByText(/214 asked · 198 matched/)).toBeInTheDocument();
  });

  // The panel exists to answer one question the shelf cannot: is a missing
  // cover IGDB not knowing the game, or this deployment's credentials?
  it("surfaces the last error, which is the whole point of the panel", async () => {
    vi.spyOn(api, "artworkSettings").mockResolvedValue({
      status: { configured: true, lastError: "401 from twitch" },
    });
    renderWithProviders(<AdminArtwork />);
    expect(await screen.findByText("401 from twitch")).toBeInTheDocument();
  });

  it("reports a test that IGDB refused", async () => {
    vi.spyOn(api, "artworkSettings").mockResolvedValue({ status: { configured: true } });
    vi.spyOn(api, "testArtwork").mockResolvedValue({ test: { ok: false, error: "no answer" } });
    renderWithProviders(<AdminArtwork />);
    await userEvent.click(await screen.findByRole("button", { name: "Test" }));
    expect(await screen.findByText("no answer")).toBeInTheDocument();
  });
});

describe("AdminCatalogue", () => {
  it("counts the catalogue when it is loaded", async () => {
    vi.spyOn(api, "saveHintsStatus").mockResolvedValue({
      status: { loaded: true, games: 38412, steamIds: 21006, url: "https://example.test/manifest" },
    });
    renderWithProviders(<AdminCatalogue />);
    expect(await screen.findByText("38,412 games")).toBeInTheDocument();
    expect(screen.getByText(/21,006 Steam ids/)).toBeInTheDocument();
  });

  it("says it is not loaded rather than showing a zero", async () => {
    vi.spyOn(api, "saveHintsStatus").mockResolvedValue({ status: { loaded: false } });
    renderWithProviders(<AdminCatalogue />);
    expect(await screen.findByText("not loaded")).toBeInTheDocument();
  });
});
