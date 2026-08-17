import { describe, expect, it, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { api } from "../lib/api";
import { renderWithProviders } from "../test/utils";
import { ContainerLogsDialog } from "./ContainerLogsDialog";

const LINES = [
  "[2026-08-05 19:33:50] [LOG] Fenwick joined the server.",
  "[2026-08-05 19:33:54] [LOG] REST accessed endpoint /v1/api/players OK",
  "[2026-08-05 19:33:59] [LOG] REST accessed endpoint /v1/api/metrics OK",
  "[2026-08-05 19:34:02] [LOG] REST accessed endpoint /v1/api/save 401",
  "[2026-08-05 19:34:10] [LOG] Autosave complete.",
];

function open() {
  return renderWithProviders(
    <ContainerLogsDialog serverId={1} containerName="palworld" open onOpenChange={() => {}} />,
  );
}

describe("ContainerLogsDialog", () => {
  beforeEach(() => {
    vi.spyOn(api, "containerLogs").mockResolvedValue({ lines: LINES });
  });

  it("hides successful REST polling by default, and says how much", async () => {
    open();
    await waitFor(() => expect(screen.getByText(/Autosave complete/)).toBeInTheDocument());
    expect(screen.queryByText(/\/v1\/api\/players OK/)).not.toBeInTheDocument();
    // The failing REST access is signal, not noise — it stays.
    expect(screen.getByText(/\/v1\/api\/save 401/)).toBeInTheDocument();
    expect(screen.getByText("· 2 hidden")).toBeInTheDocument();
  });

  it("shows everything when the toggle is turned off", async () => {
    const user = userEvent.setup();
    open();
    await waitFor(() => expect(screen.getByText(/Autosave complete/)).toBeInTheDocument());
    await user.click(screen.getByRole("checkbox", { name: /Hide REST polling/ }));
    expect(screen.getByText(/\/v1\/api\/players OK/)).toBeInTheDocument();
    expect(screen.queryByText("· 2 hidden")).not.toBeInTheDocument();
  });
});
