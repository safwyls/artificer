import { describe, expect, it, vi, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { api } from "../lib/api";
import { LinkedGameDialog } from "./LinkedGameDialog";
import { makeGame, makeLink, makeSyncWorld, renderWithProviders } from "../test/utils";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn(), loading: vi.fn() },
}));

const show = (link = makeLink()) =>
  renderWithProviders(
    <LinkedGameDialog
      game={makeGame()}
      link={link}
      world={makeSyncWorld()}
      art={{}}
      onClose={vi.fn()}
    />,
  );

afterEach(() => vi.restoreAllMocks());

describe("LinkedGameDialog", () => {
  it("points at the world, and at where custody actually lives", () => {
    show();
    expect(screen.getByText("Embervale")).toBeInTheDocument();
    expect(screen.getByText(/check it out and in from/)).toBeInTheDocument();
  });

  // A Steam game answers for itself: the field is empty and the note
  // says what will be opened anyway.
  it("says what a Steam game will open, without needing an override", () => {
    show();
    expect(screen.getByLabelText(/Launch target/)).toHaveValue("");
    expect(screen.getByText("steam://rungameid/1203620")).toBeInTheDocument();
  });

  it("admits when nothing says what starts the game", () => {
    show(makeLink({ appId: "" }));
    expect(screen.getByText(/leave the game to you/)).toBeInTheDocument();
    expect(screen.getByText(/Not a command line/)).toBeInTheDocument();
  });

  it("previews the override as it is typed, before it is saved", async () => {
    show(makeLink({ appId: "" }));
    await userEvent.type(screen.getByLabelText(/Launch target/), "D:\\Games\\thegame.exe");
    expect(screen.getByText("D:\\Games\\thegame.exe")).toBeInTheDocument();
  });

  it("saves an override, and only once it differs from what is stored", async () => {
    const update = vi.spyOn(api, "updateLink").mockResolvedValue({});
    show();
    const save = screen.getByRole("button", { name: "Save launch target" });
    expect(save).toBeDisabled();
    await userEvent.type(screen.getByLabelText(/Launch target/), "D:\\Games\\modded.lnk");
    expect(save).toBeEnabled();
    await userEvent.click(save);
    await waitFor(() =>
      expect(update).toHaveBeenCalledWith(1, { launchTarget: "D:\\Games\\modded.lnk" }),
    );
  });

  it("lets an override be cleared back to Steam's own", async () => {
    const update = vi.spyOn(api, "updateLink").mockResolvedValue({});
    show(makeLink({ launchTarget: "D:\\Games\\modded.lnk" }));
    await userEvent.clear(screen.getByLabelText(/Launch target/));
    // With the override gone, the app id answers again.
    expect(screen.getByText("steam://rungameid/1203620")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Save launch target" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith(1, { launchTarget: "" }));
  });

  it("asks before unlinking, and promises nothing is deleted", async () => {
    const unlink = vi.spyOn(api, "unlink").mockResolvedValue({});
    show();
    await userEvent.click(screen.getByRole("button", { name: "Unlink" }));
    expect(await screen.findByText("Nothing is deleted.")).toBeInTheDocument();
    expect(unlink).not.toHaveBeenCalled();
  });
});
