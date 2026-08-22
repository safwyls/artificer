import { describe, expect, it, vi, beforeEach } from "vitest";
import { api } from "./api";
import { hasSync, saveUser } from "./users";
import { makeUser } from "../test/utils";

// Regression: the update API *replaces* role, permissions and disabled
// together. A button that sent only its own field silently cleared the
// other two — which is how "Disable" used to be written, and how a user
// could come back from being disabled without world custody.
describe("saveUser sends the whole record", () => {
  beforeEach(() => {
    vi.spyOn(api, "updateUser").mockResolvedValue(undefined);
  });

  it("keeps permissions and role when only disabling", () => {
    const user = makeUser({ role: "user", permissions: ["savesync"], disabled: false });
    saveUser(user, { disabled: true });
    expect(api.updateUser).toHaveBeenCalledWith(user.id, {
      role: "user",
      permissions: ["savesync"],
      disabled: true,
    });
  });

  it("keeps disabled and permissions when only changing the role", () => {
    const user = makeUser({ role: "user", permissions: ["savesync"], disabled: true });
    saveUser(user, { role: "admin" });
    expect(api.updateUser).toHaveBeenCalledWith(user.id, {
      role: "admin",
      permissions: ["savesync"],
      disabled: true,
    });
  });

  it("adds and removes only the custody grant, leaving other permissions alone", () => {
    const user = makeUser({ permissions: ["something-else"] });
    saveUser(user, { sync: true });
    expect(api.updateUser).toHaveBeenCalledWith(user.id, {
      role: "user",
      permissions: ["something-else", "savesync"],
      disabled: false,
    });
    saveUser(makeUser({ permissions: ["something-else", "savesync"] }), { sync: false });
    expect(api.updateUser).toHaveBeenLastCalledWith(2, {
      role: "user",
      permissions: ["something-else"],
      disabled: false,
    });
  });

  it("leaves the grant untouched when the change doesn't mention it", () => {
    saveUser(makeUser({ permissions: [] }), { disabled: true });
    expect(api.updateUser).toHaveBeenCalledWith(2, {
      role: "user",
      permissions: [],
      disabled: true,
    });
  });

  it("reads the grant off the permission list", () => {
    expect(hasSync(makeUser({ permissions: ["savesync"] }))).toBe(true);
    expect(hasSync(makeUser({ permissions: [] }))).toBe(false);
  });
});
