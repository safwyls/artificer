import { describe, expect, it } from "vitest";
import type { Pal } from "./api";
import { partnerSkill, partnerTags, workProfile } from "./partner";
import {
  TEAM_TARGETS,
  TEAM_WEIGHTS,
  analyzeTeam,
  fillTeam,
  rankCandidates,
  scoreCandidate,
  suggestBuilds,
  teamTargetById,
  bestPower,
  type TeamPal,
} from "./team";

/** A live-save pal record with sane defaults, for the engine's stats read. */
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
    baseId: "",
    ...over,
  };
}

function tp(characterId: string, over: Partial<Pal> = {}): TeamPal {
  const pal = makePal(characterId, over);
  return { key: pal.instanceId, characterId, pal };
}

describe("partner catalog", () => {
  it("carries the tags the engine reasons from", () => {
    expect(partnerSkill("Anubis")?.ac).toBe("Earth");
    expect(partnerSkill("JetDragon")?.m).toBe("fly");
    expect(partnerSkill("Alpaca")?.pb).toEqual(["kingalpaca", "Defense"]);
    expect(partnerSkill("LazyCatfish")?.eb).toEqual(["Earth", "attack"]);
  });

  it("folds capture decorations onto the species", () => {
    expect(partnerSkill("BOSS_Alpaca")?.n).toBe(partnerSkill("Alpaca")?.n);
  });

  it("keeps descriptions free of markup and unresolved numbers", () => {
    for (const id of ["Anubis", "CaptainPenguin", "LazyCatfish"]) {
      const d = partnerSkill(id)?.d ?? "";
      expect(d).not.toMatch(/[{}[\]]/);
    }
  });

  it("knows mount speeds for the ranking tiebreak", () => {
    const jet = workProfile("JetDragon")?.r ?? 0;
    const nite = workProfile("HawkBird")?.r ?? 0;
    expect(jet).toBeGreaterThan(nite);
  });
});

describe("TEAM_TARGETS", () => {
  it("lists story bosses with their elements and both difficulty levels", () => {
    const grizzbolt = teamTargetById("BOSS_BATTLE_NAME_GrassBoss");
    expect(grizzbolt?.elements).toEqual(["Electricity"]);
    expect(grizzbolt?.level).toBe(10);
    expect(grizzbolt?.levelHard).toBe(72);
  });

  it("skips the Forbidden Laboratory — one element line would lie", () => {
    expect(teamTargetById("BOSS_BATTLE_NAME_WorldTreeMiddleBoss3")).toBeUndefined();
  });

  it("dedupes field bosses to one entry per species at its highest level", () => {
    const ids = TEAM_TARGETS.filter((t) => t.group === "field").map((t) => t.characterId);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("offers all nine plain elements", () => {
    expect(TEAM_TARGETS.filter((t) => t.group === "element")).toHaveLength(9);
  });
});

describe("scoreCandidate", () => {
  const fire = teamTargetById("element:Fire")!;

  it("rewards the element that answers the target", () => {
    const water = scoreCandidate(tp("BluePlatypus"), { target: fire, team: [] }, 0);
    expect(water.reasons.some((r) => r.kind === "edge" && r.label === "beats Fire")).toBe(true);
  });

  it("penalizes walking a weakness into the fight", () => {
    const leaf = scoreCandidate(tp("CloverFairy"), { target: fire, team: [] }, 0);
    expect(leaf.warnings.some((w) => w.label === "takes super-effective hits")).toBe(true);
    expect(leaf.score).toBeLessThan(0);
  });

  it("counts arming the player on top of the pal's own element", () => {
    // Anubis turns the player's attacks to Earth; Electric is weak to Earth.
    // That stacks with "beats Electric" — the pal's damage and the player's
    // are separate — which is what puts Anubis above a plain Earth pal here.
    const vsElectric = teamTargetById("element:Electricity")!;
    const anubis = scoreCandidate(tp("Anubis"), { target: vsElectric, team: [] }, 0);
    expect(anubis.reasons.some((r) => r.kind === "partner" && r.label === "Earth attacks for you")).toBe(true);
    expect(anubis.reasons.some((r) => r.kind === "edge" && r.label === "beats Electric")).toBe(true);
  });

  it("values an element buff by the allies it reaches", () => {
    const catfish = tp("LazyCatfish");
    const alone = scoreCandidate(catfish, { target: null, team: [] }, 0);
    const withEarth = scoreCandidate(catfish, { target: null, team: [tp("Anubis")] }, 0);
    expect(alone.reasons.some((r) => r.label.startsWith("arms"))).toBe(false);
    expect(withEarth.reasons.some((r) => r.label === "arms Earth allies ×1")).toBe(true);
  });

  it("scores the two real pair bonds, both ways round", () => {
    const melpaca = scoreCandidate(tp("Alpaca"), { target: null, team: [tp("KingAlpaca")] }, 0);
    expect(melpaca.reasons.some((r) => r.kind === "pair" && r.label === "pairs with Kingpaca")).toBe(true);
    const kingpaca = scoreCandidate(tp("KingAlpaca"), { target: null, team: [tp("Alpaca")] }, 0);
    expect(kingpaca.reasons.some((r) => r.kind === "pair" && r.label === "boosted by Melpaca")).toBe(true);
  });

  it("spends the mount bonus once", () => {
    const jet = tp("JetDragon");
    const first = scoreCandidate(jet, { target: null, team: [] }, 0);
    const second = scoreCandidate(jet, { target: null, team: [tp("HawkBird")] }, 0);
    expect(first.reasons.some((r) => r.kind === "mount")).toBe(true);
    expect(second.reasons.some((r) => r.kind === "mount")).toBe(false);
  });

  it("measures strength against the pool's best", () => {
    const strong = tp("JetDragon", { level: 60 });
    const yardstick = bestPower([strong, tp("ChickenPal", { level: 10 })]);
    const scored = scoreCandidate(strong, { target: null, team: [] }, yardstick);
    const power = scored.reasons.find((r) => r.kind === "power");
    expect(power?.value).toBe(40);
  });
});

describe("fillTeam", () => {
  const pool = [
    tp("JetDragon", { nickname: "a" }),
    tp("JetDragon", { nickname: "b" }),
    tp("BluePlatypus", { nickname: "c" }),
    tp("Anubis", { nickname: "d" }),
    tp("CloverFairy", { nickname: "e" }),
    tp("ChickenPal", { nickname: "f" }),
    tp("Alpaca", { nickname: "g" }),
  ];

  it("keeps locked picks and never invents a duplicate species", () => {
    const locked = [pool[0]];
    const team = fillTeam(pool, locked, teamTargetById("element:Fire")!);
    expect(team).toHaveLength(5);
    expect(team[0]).toBe(pool[0]);
    const species = team.map((p) => p.characterId.toLowerCase());
    expect(new Set(species).size).toBe(species.length);
  });

  it("fills from an empty rail", () => {
    expect(fillTeam(pool, [], null)).toHaveLength(5);
  });

  it("stops at the pool when it runs dry", () => {
    expect(fillTeam(pool.slice(0, 3), [], null).length).toBeLessThanOrEqual(3);
  });
});

describe("rankCandidates", () => {
  it("excludes pals already on the team and sorts best first", () => {
    const team = [tp("JetDragon", { nickname: "picked" })];
    const pool = [...team, tp("BluePlatypus"), tp("CloverFairy")];
    const ranked = rankCandidates(pool, { target: teamTargetById("element:Fire")!, team });
    expect(ranked.map((r) => r.pal.characterId)).not.toContain("JetDragon");
    expect(ranked[0].pal.characterId).toBe("BluePlatypus");
  });
});

describe("partnerTags", () => {
  it("speaks the same vocabulary as the team builder's reasons", () => {
    expect(partnerTags(partnerSkill("JetDragon")!).map((t) => t.label)).toContain("flying mount");
    expect(partnerTags(partnerSkill("Anubis")!)).toContainEqual({ label: "arms you with Earth", element: "Earth" });
    expect(partnerTags(partnerSkill("LazyCatfish")!).map((t) => t.label)).toContain("arms Earth pals");
    expect(partnerTags(partnerSkill("Alpaca")!)).toContainEqual({ label: "boosts Kingpaca", bond: true });
    expect(partnerTags(partnerSkill("Alpaca")!).map((t) => t.label)).toContain("ranch drops");
  });
});

describe("scoreCandidate weights", () => {
  it("reweighs factors without touching the labels", () => {
    const fire = teamTargetById("element:Fire")!;
    const heavy = { ...TEAM_WEIGHTS, takes: -60 };
    const base = scoreCandidate(tp("CloverFairy"), { target: fire, team: [] }, 0);
    const timid = scoreCandidate(tp("CloverFairy"), { target: fire, team: [] }, 0, heavy);
    expect(timid.score).toBeLessThan(base.score);
    expect(timid.warnings[0].label).toBe(base.warnings[0].label);
  });

  it("a zero mount weight silences the factor entirely", () => {
    const scored = scoreCandidate(tp("JetDragon"), { target: null, team: [] }, 0, { ...TEAM_WEIGHTS, mount: 0 });
    expect(scored.reasons.some((r) => r.kind === "mount")).toBe(false);
  });
});

describe("suggestBuilds", () => {
  const pool = [
    tp("JetDragon", { nickname: "a" }),
    tp("BluePlatypus", { nickname: "b" }),
    tp("Anubis", { nickname: "c" }),
    tp("CloverFairy", { nickname: "d" }),
    tp("ChickenPal", { nickname: "e" }),
    tp("Alpaca", { nickname: "f" }),
    tp("KingAlpaca", { nickname: "g" }),
    tp("LazyCatfish", { nickname: "h" }),
  ];
  const fire = teamTargetById("element:Fire")!;

  it("proposes distinct full parties, each with a summary", () => {
    const builds = suggestBuilds(pool, [], fire);
    expect(builds.length).toBeGreaterThanOrEqual(1);
    expect(builds.length).toBeLessThanOrEqual(3);
    const signatures = builds.map((b) =>
      b.team
        .map((t) => t.key)
        .sort()
        .join("|"),
    );
    expect(new Set(signatures).size).toBe(signatures.length);
    for (const b of builds) {
      expect(b.team).toHaveLength(5);
      expect(b.summary.avgLevel).toBeGreaterThan(0);
    }
  });

  it("keeps locked picks in every proposal", () => {
    const locked = [pool[3]]; // a Leaf pal — nothing would pick it into Fire
    for (const b of suggestBuilds(pool, locked, fire)) {
      expect(b.team.map((t) => t.key)).toContain(pool[3].key);
    }
  });

  it("the safe build carries no more exposure than the default", () => {
    const builds = suggestBuilds(pool, [], fire);
    const best = builds.find((b) => b.id === "best");
    const safe = builds.find((b) => b.id === "safe");
    if (best && safe) expect(safe.summary.exposed).toBeLessThanOrEqual(best.summary.exposed);
  });

  it("never proposes an excluded pal — exclusion is a pool filter", () => {
    const banned = pool[0].key;
    for (const b of suggestBuilds(pool.filter((p) => p.key !== banned), [], fire)) {
      expect(b.team.map((t) => t.key)).not.toContain(banned);
    }
  });
});

describe("analyzeTeam", () => {
  it("reports coverage, synergies and gaps for a real lineup", () => {
    const fire = teamTargetById("element:Fire")!;
    const team = [tp("Anubis"), tp("LazyCatfish"), tp("Alpaca"), tp("KingAlpaca")];
    const a = analyzeTeam(team, fire);
    // Anubis is Earth; Earth answers Electricity.
    expect(a.coverage.has("Electricity")).toBe(true);
    expect(a.synergies.some((s) => s.label.includes("arms 1 Earth ally"))).toBe(true);
    expect(a.synergies.some((s) => s.label === "Melpaca boosts Kingpaca")).toBe(true);
    // Dumud is Earth/Water, and Water answers Fire — no offense gap. Melpaca
    // can be ridden, so no mount gap either.
    expect(a.gaps.some((g) => g.label.includes("hits"))).toBe(false);
    expect(a.gaps.some((g) => g.label === "no mount")).toBe(false);
  });

  it("calls out a lineup with no answer to the target", () => {
    // Earth and Normal only, into a Fire fight that wants Water.
    const a = analyzeTeam([tp("Anubis"), tp("Alpaca"), tp("KingAlpaca")], teamTargetById("element:Fire")!);
    expect(a.gaps.some((g) => g.label === "nobody hits Fire hard")).toBe(true);
  });

  it("stays quiet for an empty rail", () => {
    const a = analyzeTeam([], null);
    expect(a.synergies).toHaveLength(0);
    expect(a.gaps).toHaveLength(0);
  });
});
