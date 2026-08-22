import { describe, expect, it } from "vitest";
import { ApiError } from "./api";
import { hintFor } from "./auth";

// The three distinguishable SSO hints. A silent fall-through to the
// password box is indistinguishable from Access being broken, and each of
// these has a different fix — so each keeps its own answer.
describe("hintFor", () => {
  it("calls a 404 'not configured on this server'", () => {
    expect(hintFor(new ApiError(404, "not found"))).toEqual({ kind: "unconfigured" });
  });

  it("calls a 401 'no assertion arrived with this request'", () => {
    expect(hintFor(new ApiError(401, "unauthorized"))).toEqual({ kind: "no-assertion" });
  });

  it("passes anything else through with the server's own words", () => {
    // A 403 is Access recognising someone this vault refuses — a disabled
    // account — which must be said, not swallowed.
    expect(hintFor(new ApiError(403, "this account is disabled"))).toEqual({
      kind: "error",
      message: "this account is disabled",
    });
  });

  it("says nothing about SSO for a failure that isn't an API answer", () => {
    expect(hintFor(new TypeError("network down"))).toEqual({ kind: "none" });
  });
});
