// Which shell is this page running in, and what can it therefore do.
//
// The same bundle is served two ways:
//
//   1. `cmd/companion` — the browser build. The page is served by the
//      companion itself, so `/api/...` is same-origin and there is no
//      token: whoever can reach the loopback port is already inside.
//   2. The Electron shell (`companion-desktop`). `companiond` binds an
//      ephemeral port and requires a bearer token, and the preload script
//      hands both to the renderer over `contextBridge` as
//      `window.companion` (see companion-desktop/src/ipc-contract.ts).
//      That object also carries the privileged operations the page cannot
//      do itself — a native folder picker, opening a save folder in the
//      file manager, the OS autostart toggle.
//
// Everything shell-shaped is detected here, once, so no component has to
// ask "am I in Electron". A component asks for a capability and gets
// either the native one or the web fallback; `nativeFolders` is the one
// question a component may ask directly, because the *web* fallback is a
// different piece of UI (the in-app folder browser) rather than a
// different implementation of the same call.

/** What `contextBridge.exposeInMainWorld("companion", …)` puts on the
 * window. Mirrors `CompanionBridge` in the shell's ipc-contract.ts —
 * duplicated rather than imported because the two are separate builds,
 * and drift shows up as a type error the first time either changes. */
export interface CompanionBridge {
  baseUrl: string;
  token: string;
  pickFolder(startDir?: string): Promise<string | null>;
  openPath(path: string): Promise<void>;
  setAutostart(enabled: boolean): Promise<void>;
  getAutostart(): Promise<boolean>;
}

declare global {
  interface Window {
    companion?: Partial<CompanionBridge>;
  }
}

/**
 * The bridge, if a shell installed one. Validated rather than trusted:
 * a half-built preload that exposed only `baseUrl` would otherwise send
 * every request unauthenticated to a daemon that rejects it, and the
 * failure would read as "the companion is not answering" rather than as
 * a broken shell.
 */
export function bridge(): CompanionBridge | undefined {
  const b = typeof window === "undefined" ? undefined : window.companion;
  if (!b) return undefined;
  if (typeof b.baseUrl !== "string" || typeof b.token !== "string") return undefined;
  if (typeof b.pickFolder !== "function" || typeof b.openPath !== "function") return undefined;
  return b as CompanionBridge;
}

/** Where a request goes. Same-origin in the browser build; the daemon's
 * ephemeral address under the shell. */
export function apiUrl(path: string): string {
  const b = bridge();
  if (!b) return path;
  return b.baseUrl.replace(/\/+$/, "") + path;
}

/** The credential, when there is one. The browser build sends no
 * Authorization header at all — `Routes()` has no token configured and
 * an empty bearer is worse than none, since it looks like an attempt. */
export function authHeaders(): Record<string, string> {
  const b = bridge();
  return b && b.token ? { Authorization: `Bearer ${b.token}` } : {};
}

/** True when the shell can open a real OS folder picker. Components
 * branch on this because the fallback is different UI, not a different
 * implementation: without it they draw the in-app folder browser. */
export const nativeFolders = () => Boolean(bridge());

/** Ask the OS for a folder. Resolves to null when cancelled, and when
 * there is no shell to ask. */
export async function pickFolder(startDir?: string): Promise<string | null> {
  const b = bridge();
  if (!b) return null;
  try {
    return await b.pickFolder(startDir);
  } catch {
    // A picker that failed is a cancelled picker as far as the page is
    // concerned — there is nothing useful to say about an IPC error.
    return null;
  }
}

/** Show a path in the file manager. A no-op without a shell: the browser
 * build cannot open the player's own folders and must not pretend to. */
export async function openPath(path: string): Promise<boolean> {
  const b = bridge();
  if (!b) return false;
  try {
    await b.openPath(path);
    return true;
  } catch {
    return false;
  }
}

/** The OS autostart toggle, owned entirely by the shell — the daemon
 * never had it. `undefined` means "this build cannot answer", which is
 * what keeps the setting off the screen in the browser build rather than
 * showing a switch that does nothing. */
export async function getAutostart(): Promise<boolean | undefined> {
  const b = bridge();
  if (!b || typeof b.getAutostart !== "function") return undefined;
  try {
    return await b.getAutostart();
  } catch {
    return undefined;
  }
}

export async function setAutostart(enabled: boolean): Promise<boolean> {
  const b = bridge();
  if (!b || typeof b.setAutostart !== "function") return false;
  try {
    await b.setAutostart(enabled);
    return true;
  } catch {
    return false;
  }
}
