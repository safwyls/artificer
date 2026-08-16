import { describe, expect, it } from "vitest";
import { runAdvisorTool, describeToolCall, ADVISOR_TOOLS } from "./advisor-tools";
import { BREEDABLE, breedChild } from "./breeding";
import { palName } from "./paldex";

const idOf = (name: string) => BREEDABLE.find((b) => b.name === name)!.id;

describe("advisor tools", () => {
  it("breeds by display name and agrees with the breeding table", async () => {
    // Whatever the table says Relaxaurus × Sparkit produces, the tool must
    // say the same — it's the same lookup, addressed by player names.
    const expected = palName(breedChild(idOf("Relaxaurus"), idOf("Sparkit"))!.childId);
    const out = await runAdvisorTool("breed_child", { parentA: "Relaxaurus", parentB: "Sparkit" });
    expect(out).toContain(expected);
  });

  it("says so plainly when a species is unknown", async () => {
    expect(await runAdvisorTool("breed_child", { parentA: "Notarealpal", parentB: "Sparkit" })).toContain("unknown");
    expect(await runAdvisorTool("parents_for", { child: "Notarealpal" })).toContain("Check the spelling");
  });

  it("lists parent pairs for a special pal, unique combos first", async () => {
    const out = await runAdvisorTool("parents_for", { child: "Frostallion Noct" });
    expect(out).toContain("Frostallion Noct");
    expect(out).toMatch(/\d+ pair\(s\) breed/);
    // Frostallion Noct is a unique-combo pal; the listing marks those.
    expect(out).toContain("unique combo");
  });

  it("computes inheritance odds and rejects impossible asks", async () => {
    const out = await runAdvisorTool("inheritance_odds", { parentPassiveCount: 4, desiredCount: 2 });
    expect(out).toContain("%");
    expect(out).toContain("eggs");
    expect(await runAdvisorTool("inheritance_odds", { parentPassiveCount: 2, desiredCount: 5 })).toContain("Invalid");
  });

  it("estimates stats by display name", async () => {
    const out = await runAdvisorTool("estimate_stats", { species: "Anubis", level: 50 });
    expect(out).toMatch(/Anubis at level 50: about \d+ HP, \d+ Attack, \d+ Defense/);
  });

  it("answers unknown tools and bad input as text, never by throwing", async () => {
    expect(await runAdvisorTool("nonsense", {})).toContain("Unknown tool");
    expect(await runAdvisorTool("estimate_stats", {})).toMatch(/check the spelling/i);
  });

  it("describes calls in player terms and defines every runner", async () => {
    expect(describeToolCall({ name: "breed_child", args: { parentA: "A", parentB: "B" } })).toBe("checked A × B");
    for (const tool of ADVISOR_TOOLS) {
      // Every advertised tool must actually run — a definition without a
      // runner would send the model calls nobody answers.
      expect(await runAdvisorTool(tool.name, {})).not.toContain("Unknown tool");
    }
  });
});
