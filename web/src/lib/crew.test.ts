import { describe, expect, it } from "vitest";
import type { Pal } from "./api";
import { WORK_TYPES, appetite, crewReport, dedupeSpecies, isNocturnal, topWork, workLevel, type CrewPal } from "./crew";

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
    const top = topWork("Anubis");
    expect(top[0].type === "Handcraft" || top[0].type === "Mining").toBe(true);
    expect(top.every((w) => w.level > 0)).toBe(true);
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
