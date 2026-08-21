import { describe, expect, it } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithProviders } from "../../test/utils";
import { WkCharacters } from "./WkCharacters";
import type { SaveCharacter } from "../../lib/api";

const aldra: SaveCharacter = {
  charGuid: "1D77A8A24F4A9E5B36D5CB921AC1F2E3",
  charName: "Aldra",
  saveCount: 12,
  playtimeHours: 2,
  health: 87.5,
  stamina: 100,
  sustenance: 23.7,
  hydration: 55,
  endurance: 37.2,
  progression: { questsCompleted: 8, questsInProgress: 5, recipes: 286, spells: 18, buildings: 459, journal: 324 },
  position: { x: -96222.633, y: -3299.294, z: 8697.63 },
  skills: [
    // Real skill ids — the vendored map must resolve them, including the
    // two the game added after the first catalog (Agility, Fishing).
    { id: "4zYUGF5u_0KbMLkWJmmBbQ", xp: 13363 },
    { id: "jqX0Gh6QI0GFFPCDFK_CJQ", xp: 388 },
    { id: "pJggvotwOkuoc98igUn7xA", xp: 10280 },
    { id: "vwY5IkQJJDwb2PKEfoc8MQ", xp: 273 },
    { id: "not-a-known-skill", xp: 1 },
  ],
  inventory: [
    { slot: 0, id: "P3_Aq0nAXu5dlFuBNGgyaw", count: 1, durability: 1211 },
    { slot: 7, id: "unknown-item-id", count: 42 },
  ],
  equipment: [{ slot: 1, id: "ewbJ37oeTkypaVfRgI_GPg", count: 1, durability: 88 }],
};

describe("WkCharacters", () => {
  it("renders a character with resolved skill and item names", async () => {
    renderWithProviders(<WkCharacters players={[aldra]} available loading={false} />);

    expect(screen.getByText("Aldra")).toBeInTheDocument();
    // Skill ids resolve through the vendored map — including Agility and
    // Fishing, added by the game after the first catalog; unknown ids
    // stay raw.
    expect(screen.getByText("Woodcutting")).toBeInTheDocument();
    expect(screen.getByText("Mining")).toBeInTheDocument();
    expect(screen.getByText("Agility")).toBeInTheDocument();
    expect(screen.getByText("Fishing")).toBeInTheDocument();
    expect(screen.getByText("not-a-known-skill")).toBeInTheDocument();
    // The sheet: survival meters and progression counts.
    expect(screen.getByText("Sustenance")).toBeInTheDocument();
    expect(screen.getByText("Hydration")).toBeInTheDocument();
    expect(screen.getByText("Endurance")).toBeInTheDocument();
    expect(screen.getByText(/quests 8 done · 5 underway/)).toBeInTheDocument();
    // Levels on the game's calibrated curve (classic RS formula ÷ 10):
    // 13,363 XP is level 38, 388 is level 9 — with the raw XP on hover.
    expect(screen.getByTitle(/^13,363 XP/)).toHaveTextContent("38");
    expect(screen.getByTitle(/^388 XP/)).toHaveTextContent("9");
    // Position in metres.
    expect(screen.getByText("-962, -33 m")).toBeInTheDocument();

    // Items sit behind the toggle: names resolve, unknown ids stay raw.
    await userEvent.click(screen.getByText(/Show carried items \(3\)/));
    expect(screen.getByText("Abyssal Whip")).toBeInTheDocument();
    expect(screen.getByText("Adventurer's Leggings")).toBeInTheDocument();
    expect(screen.getByText("unknown-item-id")).toBeInTheDocument();
    expect(screen.getByText("×42")).toBeInTheDocument();
  });

  it("presents a transform-only character without inventing vitals", () => {
    const transformOnly: SaveCharacter = {
      charGuid: "044F259443215BB8B575B6ACAA2A1D8D",
      charName: "",
      saveCount: 0,
      playtimeHours: 0,
      health: 0,
      stamina: 0,
      position: { x: 46522, y: 176842, z: -4000 },
      lastUpdated: 186648,
      skills: [],
      inventory: [],
      equipment: [],
    };
    renderWithProviders(<WkCharacters players={[transformOnly]} available loading={false} />);
    expect(screen.getByText("Unnamed adventurer")).toBeInTheDocument();
    expect(screen.getByText("465, 1,768 m")).toBeInTheDocument();
    // No fabricated zero facts for a record the save doesn't carry.
    expect(screen.queryByText("Health")).not.toBeInTheDocument();
    expect(screen.queryByText("Saves")).not.toBeInTheDocument();
  });

  it("marks a remembered sheet with when it was true", () => {
    // The server caches a sheet only while its player is connected, so an
    // offline adventurer's sheet is a memory — said plainly, beside a
    // position that is still current.
    renderWithProviders(
      <WkCharacters
        players={[{ ...aldra, sharedAt: undefined, seenAt: new Date(Date.now() - 3_600_000).toISOString() }]}
        available
        loading={false}
      />,
    );
    expect(screen.getByText(/sheet as of/)).toBeInTheDocument();
    // The sheet itself is still shown in full.
    expect(screen.getByText("Woodcutting")).toBeInTheDocument();
    expect(screen.getByText("-962, -33 m")).toBeInTheDocument();
  });

  it("tells the empty world honestly", () => {
    renderWithProviders(<WkCharacters players={[]} available loading={false} />);
    expect(screen.getByText(/nobody has joined this world/)).toBeInTheDocument();
  });

  it("says when there is no save to read", () => {
    renderWithProviders(<WkCharacters players={[]} available={false} loading={false} />);
    expect(screen.getByText(/No save to read/)).toBeInTheDocument();
  });

  it("surfaces a read error instead of hiding it", () => {
    renderWithProviders(
      <WkCharacters players={[]} available={false} loading={false} error="no .sav file in /saves" />,
    );
    expect(screen.getByText(/no \.sav file in \/saves/)).toBeInTheDocument();
  });
});
