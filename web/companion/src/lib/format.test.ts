import { describe, expect, it } from "vitest";
import { freshness, fmtBytes, fmtSpan, fmtTime, fmtWhen, plural } from "./format";

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

// The mono head-meta line — "v41 · 1.8 GB · 18:02" — is three machine
// facts, and each is only worth the width it takes.
describe("the head meta", () => {
  const now = new Date("2026-08-23T18:30:00Z").getTime();
  const ago = (mins: number) => new Date(now - mins * 60_000).toISOString();

  it("says minutes for the last hour and a clock time for today", () => {
    expect(fmtWhen(ago(0.5), now)).toBe("just now");
    expect(fmtWhen(ago(4), now)).toBe("4 min ago");
    expect(fmtWhen(ago(300), now)).toMatch(/\d/);
  });

  it("falls back to a weekday, then to a date", () => {
    expect(fmtWhen(ago(60 * 48), now)).toMatch(/day$/);
    expect(fmtWhen(ago(60 * 24 * 30), now)).toMatch(/\d/);
    expect(fmtWhen(undefined, now)).toBe("");
    expect(fmtWhen("not a date", now)).toBe("");
  });

  // Decimal units, because that is what the folder's own properties
  // dialog says — a save reported as 1.8 GB there must not read 1.7 here.
  it("sizes a save the way the OS does", () => {
    expect(fmtBytes(1_800_000_000)).toBe("1.8 GB");
    expect(fmtBytes(240_000_000)).toBe("240 MB");
    expect(fmtBytes(512)).toBe("512 B");
    expect(fmtBytes(0)).toBe("");
    expect(fmtBytes(undefined)).toBe("");
  });
});

// A hold lasts 48 hours, so hours and minutes are the whole scale: days
// would round the pressure away, seconds would be a stopwatch.
describe("fmtSpan", () => {
  it("drops the minutes once a hold has hours to spare", () => {
    expect(fmtSpan(47 * 3_600_000)).toBe("47h");
    expect(fmtSpan(2 * 3_600_000 + 12 * 60_000)).toBe("2h 12m");
    expect(fmtSpan(8 * 60_000)).toBe("8m");
  });

  it("has nothing to say about a span that has already run out", () => {
    expect(fmtSpan(0)).toBe("");
    expect(fmtSpan(-1)).toBe("");
    expect(fmtSpan(NaN)).toBe("");
  });
});
