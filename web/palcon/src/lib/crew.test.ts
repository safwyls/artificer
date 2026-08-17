import { describe, expect, it } from "vitest";
import type { Pal } from "./api";
import {
  WORK_TYPES,
  appetite,
  crewReport,
  dedupeSpecies,
  isNocturnal,
  topWork,
  workBreakdown,
  workLevel,
  type CrewPal,
} from "./crew";

function makePal(characterId: string, over: Partial<Pal> = {}): Pal {
  return {
    instanceId: `${characterId}-${over.nickname ?? ""}`,
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
    baseId: "base-1",
    ...over,
  };
}

function cp(characterId: string, over: Partial<Pal> = {}): CrewPal {
  const pal = makePal(characterId, over);
  return { key: pal.instanceId, characterId, pal, playerUid: "u1", playerName: "Aster", where: "At base" };
}

describe("work catalog reads", () => {
  it("knows the levels the game gives a species", () => {
    // Anubis: a Handiwork/Mining specialist; Teafant waters and nothing else.
    expect(workLevel("Anubis", "Handcraft")).toBeGreaterThanOrEqual(4);
    expect(workLevel("Anubis", "Watering")).toBe(0);
  });

  it("folds capture decorations like every other catalog read", () => {
    expect(workLevel("BOSS_Anubis", "Handcraft")).toBe(workLevel("Anubis", "Handcraft"));
  });

  it("reads nocturnal and appetite", () => {
    expect(isNocturnal("BlackPuppy")).toBe(true);
    expect(isNocturnal("ChickenPal")).toBe(false);
    expect(appetite("Anubis")).toBeGreaterThan(appetite("ChickenPal"));
  });

  it("lists a pal's suitabilities best first", () => {
    const top = topWork(makePal("Anubis"));
    expect(top[0].type === "Handcraft" || top[0].type === "Mining").toBe(true);
    expect(top.every((w) => w.level > 0)).toBe(true);
  });
});

describe("workBreakdown", () => {
  it("is just the species table for a fresh catch", () => {
    const w = workBreakdown(makePal("Penguin"), "Watering");
    expect(w).toEqual({ base: 1, books: 0, condensed: 0, level: 1, off: false });
  });

  it("adds the save's work books", () => {
    const fed = makePal("Anubis", { workAdds: { Handcraft: 2 } });
    const w = workBreakdown(fed, "Handcraft");
    expect(w.books).toBe(2);
    expect(w.level).toBe(w.base + 2);
  });

  it("books can't conjure a suitability the species lacks", () => {
    // Anubis can't water; a stray add for it must not invent a level.
    const odd = makePal("Anubis", { workAdds: { Watering: 1 } });
    expect(workBreakdown(odd, "Watering").level).toBeGreaterThanOrEqual(0);
  });

  // The three condensation ladders below are real in-game readings from a
  // live save (2026-08-06) — every pal verified book-free, so the numbers
  // isolate the star bonus exactly. If a patch changes condensation,
  // re-read the ladders in game and recalibrate condenseAdds.

  it("Pengullet ladder: stars walk the tied suitabilities in sheet order", () => {
    // In game: 1111 / 2111 / 2211 / 2221 / 3332 across 0–4 stars, listed
    // as Watering · Handiwork · Cooling · Transporting.
    const sheet = ["Watering", "Handcraft", "Cool", "Transport"];
    const ladder = [
      [1, 1, 1, 1],
      [2, 1, 1, 1],
      [2, 2, 1, 1],
      [2, 2, 2, 1],
      [3, 3, 3, 2],
    ];
    ladder.forEach((expected, stars) => {
      const pal = makePal("Penguin", { rank: stars + 1 });
      expect(sheet.map((t) => workBreakdown(pal, t).level)).toEqual(expected);
    });
    // Never a suitability the species lacks.
    expect(workBreakdown(makePal("Penguin", { rank: 5 }), "Mining").level).toBe(0);
  });

  it("Jormuntide Ignis ladder: a specialist stacks every star on its one job", () => {
    // In game: Kindling 7 / 8 / 9 / 10 / 10 — maxed out at three stars.
    [7, 8, 9, 10, 10].forEach((expected, stars) => {
      expect(workBreakdown(makePal("Umihebi_Fire", { rank: stars + 1 }), "EmitFlame").level).toBe(expected);
    });
  });

  it("Eidrolon Ignis ladder: the third star wraps back to the best", () => {
    // In game: 6·6 / 7·6 / 7·7 / 8·7 / 9·8 for Kindling · Transporting.
    const ladder = [
      [6, 6],
      [7, 6],
      [7, 7],
      [8, 7],
      [9, 8],
    ];
    ladder.forEach((expected, stars) => {
      const pal = makePal("GhostDragon_Fire", { rank: stars + 1 });
      expect([workBreakdown(pal, "EmitFlame").level, workBreakdown(pal, "Transport").level]).toEqual(expected);
    });
  });

  it("caps the total at the game's ceiling of 10", () => {
    const stacked = makePal("Anubis", { rank: 5, workAdds: { Handcraft: 9 } });
    expect(workBreakdown(stacked, "Handcraft").level).toBe(10);
  });

  it("reads the player's off-toggle", () => {
    const benched = makePal("Anubis", { workOff: ["Mining"] });
    expect(workBreakdown(benched, "Mining").off).toBe(true);
    expect(workBreakdown(benched, "Handcraft").off).toBe(false);
  });
});

describe("crewReport", () => {
  const crew = [
    cp("FlameBuffalo", { nickname: "a" }), // Kindling 3, Lumbering 2
    cp("Penguin", { nickname: "b" }), // Watering/Cooling/Handiwork/Transport 1
    cp("BlackPuppy", { nickname: "c" }), // Gathering 2, nocturnal
  ];

  it("builds all 12 rows in the game's order", () => {
    const r = crewReport(crew, []);
    expect(r.rows.map((x) => x.type)).toEqual(WORK_TYPES.map((w) => w.id));
  });

  it("ranks hands and reads the best level per row", () => {
    const r = crewReport(crew, []);
    const kindling = r.rows.find((x) => x.type === "EmitFlame")!;
    expect(kindling.best).toBe(3);
    expect(kindling.hands[0].characterId).toBe("FlameBuffalo");
    const mining = r.rows.find((x) => x.type === "Mining")!;
    expect(mining.best).toBe(0);
    expect(mining.hands).toHaveLength(0);
  });

  it("marks a row's night shift by who covers it", () => {
    const r = crewReport(crew, []);
    expect(r.rows.find((x) => x.type === "Collection")!.night).toBe(true);
    expect(r.rows.find((x) => x.type === "EmitFlame")!.night).toBe(false);
    expect(r.nightHands).toBe(1);
  });

  it("suggests a box pal only when it beats the base's best", () => {
    const boxes = [cp("Anubis", { nickname: "boxed", baseId: "" })];
    const r = crewReport(crew, boxes);
    const handiwork = r.rows.find((x) => x.type === "Handcraft")!;
    // Pengullet's Handiwork 1 is beaten by Anubis.
    expect(handiwork.upgrade?.pal.characterId).toBe("Anubis");
    expect(handiwork.upgrade?.over).toBe(1);
    // Nobody in the box waters better than Pengullet's 1 — Anubis can't.
    expect(r.rows.find((x) => x.type === "Watering")!.upgrade).toBeUndefined();
  });

  it("uses effective levels: a condensed hand raises the base's best", () => {
    // A 4-star Arsox (Kindling 3, Lumbering 2): the star cycle lands on
    // Kindling twice and Lumbering once, then the fourth star adds one
    // more everywhere — Kindling 6, Lumbering 4 on the board.
    const maxed = [cp("FlameBuffalo", { nickname: "maxed", rank: 5 })];
    const r = crewReport(maxed, []);
    expect(r.rows.find((x) => x.type === "EmitFlame")!.best).toBe(6);
    expect(r.rows.find((x) => x.type === "Deforest")!.best).toBe(4);
  });

  it("a deployed work buffer gives every other hand a flat +1", () => {
    // Wumpo raises Transporting base-wide; Pengullet's Transport 1 reads
    // 2 on the board, while Wumpo's own level stays unbuffed.
    const wumpo = cp("Yeti", { nickname: "buffer" });
    const r = crewReport([wumpo, cp("Penguin", { nickname: "hauler" })], []);
    const row = r.rows.find((x) => x.type === "Transport")!;
    const wumpoOwn = workBreakdown(wumpo.pal, "Transport").level;
    expect(row.buff?.pal.pal.nickname).toBe("buffer");
    expect(row.hands.length).toBe(2);
    // Pengullet: 1 + 1 aura = 2; the row's best is whoever ends higher.
    expect(row.best).toBe(Math.max(2, wumpoOwn));
    // A sick buffer isn't working, so the aura is gone.
    const sick = crewReport([cp("Yeti", { nickname: "ill", sick: "Cold" }), cp("Penguin", { nickname: "hauler" })], []);
    expect(sick.rows.find((x) => x.type === "Transport")!.buff).toBeUndefined();
  });

  it("a switched-off job is neither a hand nor a suggestion", () => {
    const offCrew = [cp("FlameBuffalo", { nickname: "off", workOff: ["EmitFlame"] })];
    const r = crewReport(offCrew, [cp("Anubis", { nickname: "boxed", baseId: "", workOff: ["Handcraft"] })]);
    const kindling = r.rows.find((x) => x.type === "EmitFlame")!;
    // The only kindler has the job switched off: no hands, level 0 —
    // Lumbering still counts it, since only EmitFlame was toggled.
    expect(kindling.hands).toHaveLength(0);
    expect(kindling.best).toBe(0);
    expect(r.rows.find((x) => x.type === "Deforest")!.hands).toHaveLength(1);
    // And the boxed Anubis with Handiwork off is never suggested for it.
    expect(r.rows.find((x) => x.type === "Handcraft")!.upgrade).toBeUndefined();
  });

  it("breaks a level tie by combat level when appetites match", () => {
    // Chikipi and Cattiva both gather at 1 and both eat f=1, so the
    // stronger pal wins the suggestion — it defends the base it works.
    const a = cp("ChickenPal", { nickname: "low", level: 10, baseId: "" });
    const b = cp("Carbunclo", { nickname: "high", level: 50, baseId: "" });
    const r = crewReport([], [a, b]);
    expect(r.rows.find((x) => x.type === "Collection")!.upgrade?.pal.pal.nickname).toBe("high");
  });

  it("flags the sick and the idle, and sums the appetite", () => {
    const withSick = [...crew, cp("FlameBuffalo", { nickname: "ill", sick: "Cold" })];
    const r = crewReport(withSick, []);
    expect(r.sick.map((p) => p.pal.nickname)).toEqual(["ill"]);
    expect(r.appetite).toBe(3 + 1 + 3 + 3);
    expect(r.idle).toHaveLength(0);
  });

  it("finds base-wide work buffs and ranch producers", () => {
    // Grintale's partner skill raises Cooling base-wide; Chikipi lays eggs.
    const r = crewReport([cp("BlackPuppy_Ice", { nickname: "buff" }), cp("ChickenPal", { nickname: "hen" })], []);
    expect(r.buffs.some((b) => b.pal.pal.nickname === "buff")).toBe(true);
    expect(r.ranchers.some((p) => p.pal.nickname === "hen")).toBe(true);
  });
});

describe("dedupeSpecies", () => {
  it("collapses a monoculture crew to one entry with a count", () => {
    const six = Array.from({ length: 6 }, (_, i) => cp("Anubis", { nickname: `a${i}` }));
    const out = dedupeSpecies([...six, cp("Penguin", { nickname: "p" })]);
    expect(out).toHaveLength(2);
    expect(out.find((e) => e.pal.characterId === "Anubis")?.count).toBe(6);
  });
});
