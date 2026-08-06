import { describe, expect, it } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Pal } from "../lib/api";
import { renderWithProviders } from "../test/utils";
import { PalDetailDialog } from "./PalDetailDialog";

function makePal(characterId: string, over: Partial<Pal> = {}): Pal {
  return {
    instanceId: `${characterId}-test`,
    characterId,
    nickname: "",
    level: 30,
    gender: "male",
    isBoss: false,
    isLucky: false,
    rank: 1,
    talentHp: 50,
    talentShot: 50,
    talentDefense: 50,
    passives: [],
    exp: 0,
    skills: [],
    hp: 1,
    sanity: 100,
    stomach: 100,
    friendship: 0,
    sick: "",
    souls: {},
    slotIndex: 0,
    baseId: "",
    ...over,
  };
}

function open(pal: Pal) {
  return renderWithProviders(<PalDetailDialog pal={pal} location="Palbox" onClose={() => {}} />);
}

describe("PalDetailDialog", () => {
  it("shows the partner skill with its effect tags on the overview", () => {
    open(makePal("Anubis"));
    expect(screen.getByText("Guardian of the Desert")).toBeInTheDocument();
    expect(screen.getByText("arms you with Earth")).toBeInTheDocument();
  });

  it("draws the condenser rank as stars, empty row included", () => {
    open(makePal("Anubis", { rank: 3 }));
    expect(screen.getByLabelText("Condenser: 2 of 4 stars")).toBeInTheDocument();
  });

  it("switches to the work sheet: all 12 types, unsuited ones dashed", async () => {
    const user = userEvent.setup();
    open(makePal("Anubis"));
    await user.click(screen.getByRole("tab", { name: "Work" }));
    // Anubis: a Handiwork/Mining specialist that can't water.
    expect(screen.getByText("Handiwork")).toBeInTheDocument();
    expect(screen.getByText("Watering")).toBeInTheDocument();
    expect(screen.getByRole("tabpanel").querySelectorAll(".grid > div")).toHaveLength(12);
    // Overview content is gone while the work sheet shows.
    expect(screen.queryByText("Guardian of the Desert")).not.toBeInTheDocument();
  });

  it("shows effective levels with the earned part called out", async () => {
    const user = userEvent.setup();
    // 4-star Anubis with two Handiwork books: the star cycle hits each of
    // its three suitabilities once, the fourth star again — Handiwork gets
    // species 6 + books 2 + condensed 2, landing exactly on the cap of 10.
    open(makePal("Anubis", { rank: 5, workAdds: { Handcraft: 2 } }));
    await user.click(screen.getByRole("tab", { name: "Work" }));
    const tile = screen.getByText("Handiwork").closest("div")!;
    expect(tile).toHaveTextContent("+4");
    expect(tile).toHaveTextContent("10");
    expect(tile).toHaveAttribute("title", "species 6 · books +2 · condensed +2");
  });

  it("flags a job the player switched off", async () => {
    const user = userEvent.setup();
    open(makePal("Anubis", { workOff: ["Mining"] }));
    await user.click(screen.getByRole("tab", { name: "Work" }));
    expect(screen.getByText("Mining").closest("div")).toHaveTextContent("off");
  });

  it("shows the working traits — night shift and appetite", async () => {
    const user = userEvent.setup();
    open(makePal("BlackPuppy"));
    await user.click(screen.getByRole("tab", { name: "Work" }));
    expect(screen.getByText("works through the night")).toBeInTheDocument();
    expect(screen.getByText(/^eats \d+$/)).toBeInTheDocument();
  });

  it("opens on the caller's tab — the crew planner asks for Work", () => {
    renderWithProviders(
      <PalDetailDialog pal={makePal("Anubis")} location="At base" onClose={() => {}} initialTab="work" />,
    );
    expect(screen.getByRole("tab", { name: "Work" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("Handiwork")).toBeInTheDocument();
  });

  it("says so when a species has no vendored work data", async () => {
    const user = userEvent.setup();
    open(makePal("SomethingBrandNew"));
    await user.click(screen.getByRole("tab", { name: "Work" }));
    expect(screen.getByText(/No work data vendored/)).toBeInTheDocument();
  });
});
