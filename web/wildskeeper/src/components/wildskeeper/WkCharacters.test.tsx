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
  position: { x: -96222.633, y: -3299.294, z: 8697.63 },
  skills: [
    // Real skill ids — the vendored map must resolve them.
    { id: "4zYUGF5u_0KbMLkWJmmBbQ", xp: 13363 },
    { id: "jqX0Gh6QI0GFFPCDFK_CJQ", xp: 388 },
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
    // Skill ids resolve through the vendored map; unknown ids stay raw.
    expect(screen.getByText("Woodcutting")).toBeInTheDocument();
    expect(screen.getByText("Mining")).toBeInTheDocument();
    expect(screen.getByText("not-a-known-skill")).toBeInTheDocument();
    // Levels derived on the RuneScape curve — 13,363 XP is exactly level
    // 30, 388 exactly level 5 — with the raw XP kept on hover.
    expect(screen.getByTitle(/^13,363 XP/)).toHaveTextContent("30");
    expect(screen.getByTitle(/^388 XP/)).toHaveTextContent("5");
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
