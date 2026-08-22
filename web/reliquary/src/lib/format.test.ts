import { describe, expect, it } from "vitest";
import { fmtBytes, fmtTime } from "./format";

describe("fmtBytes", () => {
  it("names GB, MB and KB the way the version rows do", () => {
    expect(fmtBytes(2 * (1 << 30))).toBe("2.0 GB");
    expect(fmtBytes(191_000_000)).toBe("182.2 MB");
    expect(fmtBytes(4096)).toBe("4 KB");
  });

  it("never calls a save that exists 0 KB", () => {
    expect(fmtBytes(1)).toBe("1 KB");
    expect(fmtBytes(0)).toBe("1 KB");
  });
});

describe("fmtTime", () => {
  it("renders nothing for a missing or unparseable timestamp", () => {
    // A world with no hold has no expiry; the line must not read
    // "until Invalid Date".
    expect(fmtTime(undefined)).toBe("");
    expect(fmtTime("")).toBe("");
    expect(fmtTime("not a time")).toBe("");
  });

  it("renders a real timestamp in the reader's own zone", () => {
    expect(fmtTime("2026-08-20T21:04:00Z")).not.toBe("");
  });
});
