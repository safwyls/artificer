import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithProviders } from "../../test/utils";
import { WkCompanionPanel } from "./WkCompanionPanel";

const getCompanion = vi.fn();
const setCompanion = vi.fn();
vi.mock("../../lib/api", async (importOriginal) => {
  const mod = await importOriginal<typeof import("../../lib/api")>();
  return {
    ...mod,
    api: {
      ...mod.api,
      getCompanion: (...a: unknown[]) => getCompanion(...a),
      setCompanion: (...a: unknown[]) => setCompanion(...a),
    },
  };
});

describe("WkCompanionPanel", () => {
  beforeEach(() => {
    getCompanion.mockReset();
    setCompanion.mockReset();
  });

  it("shows the token and shared count when enabled", async () => {
    getCompanion.mockResolvedValue({ enabled: true, token: "aabbccddeeff", shared: 2 });
    renderWithProviders(<WkCompanionPanel serverId={1} />);
    expect(await screen.findByText("aabbccddeeff")).toBeInTheDocument();
    expect(screen.getByText("2 characters shared")).toBeInTheDocument();
    expect(screen.getByText("Disable sharing")).toBeInTheDocument();
  });

  it("enables sharing on request", async () => {
    getCompanion.mockResolvedValue({ enabled: false, token: "", shared: 0 });
    setCompanion.mockResolvedValue({ enabled: true, token: "fresh" });
    renderWithProviders(<WkCompanionPanel serverId={7} />);
    await userEvent.click(await screen.findByText("Enable sharing"));
    await waitFor(() => expect(setCompanion).toHaveBeenCalledWith(7, true));
  });
});
