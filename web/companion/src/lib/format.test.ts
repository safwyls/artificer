import { describe, expect, it } from "vitest";
import { freshness, fmtTime, plural } from "./format";

// Custody is shared state — someone else checking a world in is the whole
// reason this page exists — so the age of what is on screen is said out
// loud rather than left to guess.
describe("freshness", () => {
  const at = "2026-08-22T12:00:00Z";
  const t = (offsetSecs: number) => new Date(at).getTime() + offsetSecs * 1000;

  it("says nothing has been synced yet when nothing has", () => {
    expect(freshness(undefined)).toBe("not synced yet");
    expect(freshness("not a timestamp")).toBe("not synced yet");
  });

  it("calls the last ten seconds up to date", () => {
    expect(freshness(at, t(0))).toBe("up to date");
    expect(freshness(at, t(9))).toBe("up to date");
  });

  it("counts seconds up to a minute and a half, then minutes", () => {
    expect(freshness(at, t(40))).toBe("synced 40s ago");
    expect(freshness(at, t(300))).toBe("synced 5 min ago");
  });

  // A clock that is behind the companion's must not read "synced -3s ago".
  it("never reports a negative age", () => {
    expect(freshness(at, t(-30))).toBe("up to date");
  });
});

describe("fmtTime", () => {
  it("renders nothing for a missing or unparseable timestamp", () => {
    expect(fmtTime(undefined)).toBe("");
    expect(fmtTime("nonsense")).toBe("");
  });
});

describe("plural", () => {
  it("agrees with its count", () => {
    expect(plural(1, "library", "libraries")).toBe("1 library");
    expect(plural(2, "library", "libraries")).toBe("2 libraries");
    expect(plural(0, "path", "paths")).toBe("0 paths");
  });
});
