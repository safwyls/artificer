import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { api } from "../lib/api";
import { LinkGameDialog, byHandGame } from "./LinkGameDialog";
import { makeGame, makeLink, makeState, makeSyncWorld, renderWithProviders } from "../test/utils";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn(), loading: vi.fn() },
}));

const CAND = "C:\\Users\\you\\AppData\\Roaming\\Enshrouded\\savegame";

const stateWith = (worlds = [makeSyncWorld()]) =>
  makeState({ sync: { configured: true, username: "safwyl", busy: false, worlds } });

const show = (game = makeGame(), state = stateWith(), onClose = vi.fn()) => {
  const r = renderWithProviders(
    <LinkGameDialog game={game} state={state} art={{}} onClose={onClose} />,
  );
  return { ...r, onClose };
};

beforeEach(() => {
  vi.spyOn(api, "splitSavePath").mockResolvedValue({ split: null });
  vi.spyOn(api, "resolveSavePath").mockResolvedValue({ dir: "", exists: false });
});
afterEach(() => vi.restoreAllMocks());

describe("LinkGameDialog", () => {
  it("offers the candidate folders with why each was found, and fills the first", () => {
    show();
    expect(screen.getByDisplayValue(CAND)).toBeInTheDocument();
    expect(screen.getByText(`${CAND} — save-location catalogue`)).toBeInTheDocument();
  });

  // "No save folder found" is a job for the player, not a footnote.
  it("asks for a folder outright when discovery found none", () => {
    show(makeGame({ saveDirs: [] }));
    expect(screen.getByText(/No save folder was found for this game\./)).toBeInTheDocument();
  });

  // Regression: caught here rather than at the service — the answer is a
  // folder the player has to supply, and the round trip only delays the ask.
  it("refuses to link without a folder, without a round trip", async () => {
    const create = vi.spyOn(api, "createWorld").mockResolvedValue({});
    show(makeGame({ saveDirs: [] }));
    await userEvent.click(screen.getByRole("button", { name: "Link" }));
    expect(
      await screen.findByText(
        "This needs the game's save folder before it can link. Paste the folder that holds the save files.",
      ),
    ).toBeInTheDocument();
    expect(create).not.toHaveBeenCalled();
  });

  it("refuses to create a nameless world", async () => {
    const create = vi.spyOn(api, "createWorld").mockResolvedValue({});
    show(byHandGame());
    await userEvent.type(screen.getByLabelText("Save folder"), "C:\\somewhere");
    await userEvent.click(screen.getByRole("button", { name: "Link" }));
    expect(await screen.findByText("A new world needs a name.")).toBeInTheDocument();
    expect(create).not.toHaveBeenCalled();
  });

  // Regression: submitting used to close unconditionally and report the
  // failure to a status line in a panel *behind* this box, so a refused
  // link looked exactly like a successful one that did nothing.
  it("keeps the dialog open and shows the service's refusal inside it", async () => {
    vi.spyOn(api, "createWorld").mockRejectedValue(new Error("that folder holds no save files"));
    const { onClose } = show();
    await userEvent.click(screen.getByRole("button", { name: "Link" }));
    expect(await screen.findByText("that folder holds no save files")).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("closes only once the link actually happened", async () => {
    const create = vi.spyOn(api, "createWorld").mockResolvedValue({});
    const { onClose } = show();
    await userEvent.click(screen.getByRole("button", { name: "Link" }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({ name: "Enshrouded", dir: CAND, seed: true }),
    );
  });

  // Joining a world whose folder is an opaque id: the player supplies the
  // half they can know, and the companion makes the rest underneath.
  it("resolves the world's own folder beneath the player's root when joining", async () => {
    const world = makeSyncWorld();
    world.world.savePath = "3fd2c1a09b";
    vi.spyOn(api, "resolveSavePath").mockResolvedValue({
      dir: `${CAND}\\3fd2c1a09b`,
      exists: false,
    });
    const add = vi.spyOn(api, "addLink").mockResolvedValue({});
    show(makeGame(), stateWith([world]));
    await userEvent.selectOptions(screen.getByLabelText("World on the service"), "1");
    // The explainer says what will happen before anything is created.
    expect(await screen.findByText("3fd2c1a09b")).toBeInTheDocument();
    expect(screen.getByText(/linking will/)).toBeInTheDocument();
    expect(screen.getByText(/It does not exist yet/)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Link" }));
    await waitFor(() =>
      // create:true only on submit — the preview never makes a folder.
      expect(api.resolveSavePath).toHaveBeenLastCalledWith(CAND, "3fd2c1a09b", true),
    );
    expect(add).toHaveBeenCalledWith(expect.objectContaining({ worldId: 1, dir: `${CAND}\\3fd2c1a09b` }));
  });

  it("records the leaf when creating a world, so joiners get it made for them", async () => {
    vi.spyOn(api, "splitSavePath").mockResolvedValue({
      split: { root: CAND, leaf: "3fd2c1a09b", why: "the folder below the save root" },
    });
    const create = vi.spyOn(api, "createWorld").mockResolvedValue({});
    show();
    expect(await screen.findByText(/will be recorded as the folder/)).toBeInTheDocument();
    expect(screen.getByText(/they never need to know it/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Link" }));
    await waitFor(() =>
      expect(create).toHaveBeenCalledWith(expect.objectContaining({ savePath: "3fd2c1a09b" })),
    );
  });

  it("hides the new-world fields once an existing world is chosen", async () => {
    show();
    expect(screen.getByLabelText("New world's name")).toBeInTheDocument();
    await userEvent.selectOptions(screen.getByLabelText("World on the service"), "1");
    expect(screen.queryByLabelText("New world's name")).not.toBeInTheDocument();
  });

  it("does not offer a world this machine has already linked", () => {
    const state = makeState({
      sync: { configured: true, username: "safwyl", busy: false, worlds: [makeSyncWorld()] },
      links: [makeLink({ worldId: 1 })],
    });
    show(makeGame(), state);
    expect(screen.queryByRole("option", { name: /Embervale/ })).not.toBeInTheDocument();
  });

  // Regression, the jsattr class of bug: the old page interpolated paths
  // into inline handlers, so a quote or a backslash closed the attribute
  // and the handler silently never ran. JSX passes values; this proves
  // the path survives to the API byte for byte.
  it("carries a path with quotes and backslashes through intact", async () => {
    const create = vi.spyOn(api, "createWorld").mockResolvedValue({});
    const hostile = `C:\\Users\\o'brien\\My "Games"\\Saved`;
    show(makeGame({ saveDirs: [] }));
    const field = screen.getByLabelText("Save folder");
    await userEvent.click(field);
    await userEvent.paste(hostile);
    expect(field).toHaveValue(hostile);
    await userEvent.click(screen.getByRole("button", { name: "Link" }));
    await waitFor(() => expect(create).toHaveBeenCalledWith(expect.objectContaining({ dir: hostile })));
  });

  it("offers to hide a real shelf entry, but not a by-hand folder", () => {
    const { unmount } = show();
    expect(screen.getByRole("button", { name: "Hide from shelf" })).toBeInTheDocument();
    unmount();
    show(byHandGame());
    expect(screen.queryByRole("button", { name: "Hide from shelf" })).not.toBeInTheDocument();
  });

  it("fills the folder from the browser and clears the error with it", async () => {
    vi.spyOn(api, "browse").mockResolvedValue({
      browse: {
        path: "C:\\Users\\you\\Saved Games",
        entries: [{ name: "Enshrouded", path: "C:\\Users\\you\\Saved Games\\Enshrouded", saveish: true }],
        roots: [],
      },
    });
    show(makeGame({ saveDirs: [] }));
    await userEvent.click(screen.getByRole("button", { name: "Link" }));
    await screen.findByText(/This needs the game's save folder/);
    await userEvent.click(screen.getByRole("button", { name: "Browse this computer…" }));
    await userEvent.click(await screen.findByRole("button", { name: "Use this folder" }));
    expect(screen.getByLabelText("Save folder")).toHaveValue("C:\\Users\\you\\Saved Games");
    expect(screen.queryByText(/This needs the game's save folder/)).not.toBeInTheDocument();
  });
});
