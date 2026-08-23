import { describe, expect, it } from "vitest";
import {
  artFor,
  custodyOf,
  gameKey,
  holdIsPressing,
  holdLeft,
  launchTargetOf,
  launchable,
} from "./types";
import { makeLink, makeSyncWorld } from "../test/utils";

// One identity for a game, shared by the artwork map, the hidden list and
// the service. If these three ever key differently, a game gets a cover
// under one name and is hidden under another.
describe("gameKey", () => {
  it("prefers the Steam app id", () => {
    expect(gameKey({ appId: "1203620", name: "Enshrouded" })).toBe("app:1203620");
  });

  it("falls back to a trimmed, lowercased name", () => {
    expect(gameKey({ name: "  Enshrouded " })).toBe("name:enshrouded");
    expect(gameKey({})).toBe("name:");
  });
});

describe("artFor", () => {
  const art = { "app:1203620": { cover: "a" }, "name:enshrouded": { cover: "b" } };

  it("resolves by app id first", () => {
    expect(artFor(art, { appId: "1203620", name: "Enshrouded" }).cover).toBe("a");
  });

  // A link made before app ids were recorded still matches by title.
  it("falls back to the name for a game with no app id", () => {
    expect(artFor(art, { name: "Enshrouded" }).cover).toBe("b");
    expect(artFor(art, { appId: "999", name: "Enshrouded" }).cover).toBe("b");
  });

  it("answers with an empty cover rather than undefined", () => {
    expect(artFor({}, { name: "Valheim" })).toEqual({});
  });
});

describe("custodyOf", () => {
  const holder = (o: Record<string, unknown> = {}) => ({
    sessionId: 7,
    username: "mira",
    expiresAt: "2026-08-22T23:00:00Z",
    claimable: false,
    ...o,
  });

  it("calls a world with no holder free", () => {
    expect(custodyOf(makeLink(), makeSyncWorld(), "safwyl", true).state).toBe("free");
  });

  it("is mine only when this machine holds the session", () => {
    const world = makeSyncWorld({ holder: holder({ username: "safwyl", sessionId: 7 }) });
    expect(custodyOf(makeLink({ sessionId: 7 }), world, "safwyl", true).state).toBe("mine");
  });

  // The account holds it but this machine has no session for it: another
  // machine of theirs took it, or the download is still on its way here.
  // Offering "Check in" there would check in a folder that has not
  // received the save yet.
  it("is fetching when the hold is this account's but another session's", () => {
    const world = makeSyncWorld({ holder: holder({ username: "safwyl", sessionId: 9 }) });
    expect(custodyOf(makeLink({ sessionId: 7 }), world, "safwyl", true).state).toBe("fetching");
  });

  it("distinguishes someone else's live hold from an expired one", () => {
    expect(custodyOf(makeLink(), makeSyncWorld({ holder: holder() }), "safwyl", true).state).toBe("held");
    expect(
      custodyOf(makeLink(), makeSyncWorld({ holder: holder({ claimable: true }) }), "safwyl", true)
        .state,
    ).toBe("expired");
  });

  // A link to a world the service no longer has is a real state, and the
  // player has to be told rather than shown a row that does nothing.
  it("reports a world the service no longer knows, but only when connected", () => {
    expect(custodyOf(makeLink(), undefined, "safwyl", true).state).toBe("gone");
    expect(custodyOf(makeLink(), undefined, undefined, false).state).toBe("free");
  });
});

// Mirrors launchTarget() in launch.go. The companion is still the one
// that decides what to open; this only labels the button, and the two
// must not disagree about whether there is anything to open at all.
describe("launchTargetOf", () => {
  it("builds Steam's run URI from the app id", () => {
    expect(launchTargetOf(makeLink({ appId: "1203620" }))).toBe("steam://rungameid/1203620");
  });

  it("prefers the player's own override", () => {
    expect(launchTargetOf(makeLink({ appId: "1203620", launchTarget: "D:\\g.lnk" }))).toBe("D:\\g.lnk");
  });

  it("has nothing to open for a folder linked by hand", () => {
    expect(launchTargetOf(makeLink({ appId: "" }))).toBe("");
    expect(launchable(makeLink({ appId: "" }))).toBe(false);
    expect(launchable(makeLink({ appId: "", launchTarget: "   " }))).toBe(false);
    expect(launchable(makeLink())).toBe(true);
  });
});

// Both the chip and the row's one primary action read this single value.
// Carrying the holder and the expiry on it is what stops a second lookup
// from disagreeing with the first.
describe("the custody record", () => {
  const soon = (ms: number) => new Date(Date.now() + ms).toISOString();

  it("carries who holds it, until when, and who is queued behind them", () => {
    const world = makeSyncWorld({
      holder: { sessionId: 7, username: "mira", expiresAt: soon(3_600_000), claimable: false },
      claimedBy: "torv",
    });
    const custody = custodyOf(makeLink(), world, "safwyl", true);
    expect(custody.holder).toBe("mira");
    expect(custody.claimedBy).toBe("torv");
    expect(holdLeft(custody)).toBeGreaterThan(3_500_000);
  });

  // Holds last 48 hours. A timer ticking for two days is noise; a timer
  // under three hours is the reason to go and check the world in.
  it("calls a hold pressing only under three hours", () => {
    const pressing = custodyOf(
      makeLink(),
      makeSyncWorld({
        holder: { sessionId: 7, username: "mira", expiresAt: soon(2 * 3_600_000), claimable: false },
      }),
      "safwyl",
      true,
    );
    const calm = custodyOf(
      makeLink(),
      makeSyncWorld({
        holder: { sessionId: 7, username: "mira", expiresAt: soon(40 * 3_600_000), claimable: false },
      }),
      "safwyl",
      true,
    );
    expect(holdIsPressing(pressing)).toBe(true);
    expect(holdIsPressing(calm)).toBe(false);
  });

  it("has nothing to count down for a free world or a lapsed hold", () => {
    expect(holdIsPressing({ state: "free" })).toBe(false);
    expect(holdIsPressing({ state: "held", expiresAt: new Date(Date.now() - 1000).toISOString() })).toBe(
      false,
    );
    expect(Number.isNaN(holdLeft({ state: "free" }))).toBe(true);
  });
});
