import { describe, expect, it, vi } from "vitest";
import { at, worldActions, type Actor, type WorldHandlers } from "./worldActions";
import { makeHolder, makeStatus, makeVersion, makeWorld } from "../test/utils";

const handlers = (): WorldHandlers => ({
  checkout: vi.fn(),
  renew: vi.fn(),
  claim: vi.fn(),
  unclaim: vi.fn(),
  requestHandback: vi.fn(),
  release: vi.fn(),
  serverGive: vi.fn(),
  serverTake: vi.fn(),
  remove: vi.fn(),
});

const player: Actor = { username: "safwyl", isAdmin: false, canSync: true };
const admin: Actor = { username: "safwyl", isAdmin: true, canSync: true };
const reader: Actor = { username: "guest", isAdmin: false, canSync: false };

const ids = (status = makeStatus(), me = player) =>
  worldActions(status, me, handlers()).map((a) => a.id);

describe("worldActions — the permission model", () => {
  it("offers exactly one primary per custody state", () => {
    for (const status of [
      makeStatus(),
      makeStatus({ holder: makeHolder({ username: "safwyl" }) }),
      makeStatus({ holder: makeHolder({ claimable: true }) }),
    ]) {
      const primary = at(worldActions(status, admin, handlers()), "card", "primary");
      expect(primary).toHaveLength(1);
    }
  });

  it("a free world offers check-out and import", () => {
    expect(ids()).toContain("checkout");
    expect(ids()).toContain("import");
  });

  it("the holder gets check-in and renew, and nobody else does", () => {
    const mine = makeStatus({ holder: makeHolder({ username: "safwyl" }) });
    expect(ids(mine)).toEqual(expect.arrayContaining(["checkin", "renew"]));
    const theirs = makeStatus({ holder: makeHolder() });
    expect(ids(theirs)).not.toContain("checkin");
    expect(ids(theirs)).not.toContain("renew");
  });

  it("an expired hold is takeable by someone else, not by its own holder", () => {
    expect(ids(makeStatus({ holder: makeHolder({ claimable: true }) }))).toContain("takeover");
    expect(
      ids(makeStatus({ holder: makeHolder({ username: "safwyl", claimable: true }) })),
    ).not.toContain("takeover");
  });

  it("claiming next is offered once, and withdrawing only to the claimant", () => {
    const held = makeStatus({ holder: makeHolder() });
    expect(ids(held)).toContain("claim");
    // Someone already holds the claim: no second claim.
    expect(ids({ ...held, claimedBy: "torv" })).not.toContain("claim");
    expect(ids({ ...held, claimedBy: "torv" })).not.toContain("unclaim");
    expect(ids({ ...held, claimedBy: "safwyl" })).toContain("unclaim");
  });

  it("hides every mutating verb from an account without world custody", () => {
    const held = makeStatus({ holder: makeHolder(), head: undefined });
    const offered = ids(held, reader);
    // Not disabled — absent. Only looking is left.
    expect(offered).toEqual(["history"]);
  });

  it("still lets a read-only account download the head", () => {
    // Seeing the worlds and downloading a version is the whole of what an
    // account without custody may do — so this one must survive.
    expect(ids(makeStatus({ head: makeVersion() }), reader)).toContain("download-head");
  });

  it("keeps the admin verbs to admins", () => {
    const held = makeStatus({ holder: makeHolder() });
    for (const id of ["release", "delete", "settings", "request-checkin"]) {
      expect(ids(held, player)).not.toContain(id);
      expect(ids(held, admin)).toContain(id);
    }
  });

  it("offers a checkpoint request only where the world keeps checkpoints", () => {
    const held = makeStatus({ holder: makeHolder() });
    expect(ids(held, admin)).not.toContain("request-checkpoint");
    const withCheckpoints = makeStatus({
      world: makeWorld({ checkpoints: true }),
      holder: makeHolder(),
    });
    expect(ids(withCheckpoints, admin)).toContain("request-checkpoint");
  });

  it("swaps asking for the world back with withdrawing the request", () => {
    const asked = makeStatus({ holder: makeHolder({ requestedKind: "checkin" }) });
    expect(ids(asked, admin)).toContain("request-withdraw");
    expect(ids(asked, admin)).not.toContain("request-checkin");
  });

  it("never asks the dedicated server to hand back through a companion", () => {
    // A server-held world has no companion to poll; give/take is the pair.
    const served = makeStatus({
      world: makeWorld({ agentUrl: "http://host:8420" }),
      holder: makeHolder({ serverHeld: true }),
    });
    expect(ids(served, admin)).not.toContain("request-checkin");
    expect(ids(served, admin)).toContain("server-take");
  });

  it("only offers to host on a server that exists, for a world with a head", () => {
    const head = makeStatus({ head: makeVersion() });
    expect(ids(head, admin)).not.toContain("server-give");
    const linked = { ...head, world: makeWorld({ agentUrl: "http://host:8420" }) };
    expect(ids(linked, admin)).toContain("server-give");
  });

  it("puts the destructive verbs behind a confirmation", () => {
    const held = makeStatus({ holder: makeHolder() });
    const actions = worldActions(held, admin, handlers());
    for (const id of ["release", "delete", "takeover"]) {
      const action = actions.find((a) => a.id === id);
      if (action) expect(action.confirm).toBeTruthy();
    }
  });
});

