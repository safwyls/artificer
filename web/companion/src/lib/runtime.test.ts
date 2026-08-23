import { afterEach, describe, expect, it, vi } from "vitest";
import { apiUrl, authHeaders, bridge, getAutostart, nativeFolders, openPath, pickFolder, setAutostart } from "./runtime";
import { call } from "./api";
import type { CompanionBridge } from "./runtime";

function install(over: Partial<CompanionBridge> = {}) {
  const b: Partial<CompanionBridge> = {
    baseUrl: "http://127.0.0.1:41234",
    token: "tok",
    pickFolder: vi.fn(async () => "D:\\picked"),
    openPath: vi.fn(async () => {}),
    setAutostart: vi.fn(async () => {}),
    getAutostart: vi.fn(async () => true),
    ...over,
  };
  window.companion = b;
  return b;
}

afterEach(() => {
  delete window.companion;
  vi.restoreAllMocks();
});

describe("runtime — the browser build", () => {
  it("has no bridge, so requests stay same-origin and unauthenticated", () => {
    expect(bridge()).toBeUndefined();
    expect(apiUrl("/api/state")).toBe("/api/state");
    expect(authHeaders()).toEqual({});
    expect(nativeFolders()).toBe(false);
  });

  it("answers 'cannot' rather than pretending, for every native capability", async () => {
    expect(await pickFolder()).toBeNull();
    expect(await openPath("C:\\saves")).toBe(false);
    expect(await getAutostart()).toBeUndefined();
    expect(await setAutostart(true)).toBe(false);
  });
});

describe("runtime — under the Electron shell", () => {
  it("prefixes the daemon's address and carries the bearer token", () => {
    install();
    expect(apiUrl("/api/state")).toBe("http://127.0.0.1:41234/api/state");
    expect(authHeaders()).toEqual({ Authorization: "Bearer tok" });
    expect(nativeFolders()).toBe(true);
  });

  it("does not double the slash when the shell's baseUrl has a trailing one", () => {
    install({ baseUrl: "http://127.0.0.1:41234/" });
    expect(apiUrl("/api/state")).toBe("http://127.0.0.1:41234/api/state");
  });

  // A preload that exposed half a bridge would otherwise send every
  // request to the daemon without the token it requires, and the failure
  // would read as "the companion is not answering".
  it("ignores a half-built bridge rather than half-using it", () => {
    window.companion = { baseUrl: "http://127.0.0.1:1" } as Partial<CompanionBridge>;
    expect(bridge()).toBeUndefined();
    expect(apiUrl("/api/state")).toBe("/api/state");
  });

  it("uses the native picker and the native opener", async () => {
    const b = install();
    expect(await pickFolder("D:\\start")).toBe("D:\\picked");
    expect(b.pickFolder).toHaveBeenCalledWith("D:\\start");
    expect(await openPath("C:\\saves")).toBe(true);
    expect(b.openPath).toHaveBeenCalledWith("C:\\saves");
  });

  it("treats a failing IPC call as a cancelled one", async () => {
    install({
      pickFolder: vi.fn(async () => {
        throw new Error("no window");
      }),
      openPath: vi.fn(async () => {
        throw new Error("no window");
      }),
    });
    expect(await pickFolder()).toBeNull();
    expect(await openPath("x")).toBe(false);
  });

  it("reads and writes the OS autostart toggle", async () => {
    const b = install();
    expect(await getAutostart()).toBe(true);
    expect(await setAutostart(false)).toBe(true);
    expect(b.setAutostart).toHaveBeenCalledWith(false);
  });
});

describe("the api client follows the runtime", () => {
  it("sends the bearer token to the daemon's own address", async () => {
    install();
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }));
    await call("POST", "/api/sync/refresh");
    expect(fetchMock).toHaveBeenCalledWith(
      "http://127.0.0.1:41234/api/sync/refresh",
      expect.objectContaining({ headers: { Authorization: "Bearer tok" } }),
    );
  });

  it("sends no Authorization header at all in the browser build", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }));
    await call("GET", "/api/state");
    expect(fetchMock).toHaveBeenCalledWith("/api/state", expect.objectContaining({ headers: {} }));
  });
});
