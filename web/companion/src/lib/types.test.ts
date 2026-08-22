import { describe, expect, it } from "vitest";
import { artFor, custodyOf, gameKey, launchTargetOf, launchable } from "./types";
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
    expect(custodyOf(makeLink(), makeSyncWorld(), "safwyl", true)).toBe("free");
  });

  it("is mine only when this machine holds the session", () => {
    const world = makeSyncWorld({ holder: holder({ username: "safwyl", sessionId: 7 }) });
    expect(custodyOf(makeLink({ sessionId: 7 }), world, "safwyl", true)).toBe("mine");
  });

  // The account holds it but this machine has no session for it: another
  // machine of theirs took it, or the download is still on its way here.
  // Offering "Check in" there would check in a folder that has not
  // received the save yet.
  it("is fetching when the hold is this account's but another session's", () => {
    const world = makeSyncWorld({ holder: holder({ username: "safwyl", sessionId: 9 }) });
    expect(custodyOf(makeLink({ sessionId: 7 }), world, "safwyl", true)).toBe("fetching");
  });

  it("distinguishes someone else's live hold from an expired one", () => {
    expect(custodyOf(makeLink(), makeSyncWorld({ holder: holder() }), "safwyl", true)).toBe("held");
    expect(
      custodyOf(makeLink(), makeSyncWorld({ holder: holder({ claimable: true }) }), "safwyl", true),
    ).toBe("expired");
  });

  // A link to a world the service no longer has is a real state, and the
  // player has to be told rather than shown a row that does nothing.
  it("reports a world the service no longer knows, but only when connected", () => {
    expect(custodyOf(makeLink(), undefined, "safwyl", true)).toBe("gone");
    expect(custodyOf(makeLink(), undefined, undefined, false)).toBe("free");
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
