import type { Browse, CompanionState, Artwork, SplitInfo } from "./types";

/**
 * The companion's API answers `{ok: false, error}` rather than HTTP
 * statuses — it is a local process talking to its own page, and the page
 * shows the sentence. `call` turns that into a thrown Error so every
 * caller handles failure the same way.
 */
export async function call<T = Record<string, unknown>>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const out = (await res.json().catch(() => ({}))) as { ok?: boolean; error?: string };
  if (out.ok === false && out.error) throw new Error(out.error);
  if (!res.ok && !out.error) throw new Error(res.statusText || "the companion did not answer");
  return out as T;
}

export function errorText(err: unknown): string {
  return err instanceof Error && err.message ? err.message : "something went wrong";
}

export interface ConfigInput {
  serverUrl?: string;
  token?: string;
  steamDirs?: string[];
}

export interface LinkInput {
  worldId: number;
  gameTitle: string;
  dir: string;
  meta: string;
  appId: string;
}

export interface CreateWorldInput {
  name: string;
  gameTitle: string;
  dir: string;
  meta: string;
  appId: string;
  savePath: string;
  seed: boolean;
}

export const api = {
  state: () => call<CompanionState>("GET", "/api/state"),
  /** Only the fields present are written — the connect card and the Steam
   * card save independently, and absent means "keep what is stored". */
  setConfig: (input: ConfigInput) => call("PUT", "/api/config", input),
  discover: () => call<{ found: number }>("POST", "/api/discover"),
  syncNow: () => call<{ worlds: number }>("POST", "/api/sync/refresh"),
  artwork: () =>
    call<{ art?: Record<string, Artwork>; asked?: boolean; error?: string }>("GET", "/api/artwork"),
  saveHints: () =>
    call<{ available?: boolean; known?: number; error?: string }>("GET", "/api/savehints"),
  browse: (path: string) =>
    call<{ browse: Browse }>("GET", `/api/browse?path=${encodeURIComponent(path || "")}`),
  splitSavePath: (dir: string, appId: string, name: string) =>
    call<{ split: SplitInfo | null }>(
      "GET",
      `/api/savepath/split?${new URLSearchParams({ dir, appId, name }).toString()}`,
    ),
  resolveSavePath: (root: string, leaf: string, create: boolean) =>
    call<{ dir: string; exists: boolean }>("POST", "/api/savepath/resolve", { root, leaf, create }),
  hide: (key: string, hidden: boolean) => call("POST", "/api/hide", { key, hidden }),

  addLink: (input: LinkInput) => call("POST", "/api/links", input),
  createWorld: (input: CreateWorldInput) => call("POST", "/api/links/create", input),
  unlink: (worldID: number) => call("DELETE", `/api/links/${worldID}`),
  checkout: (worldID: number, takeover: boolean) =>
    call("POST", `/api/links/${worldID}/checkout`, { takeover }),
  checkin: (worldID: number) => call("POST", `/api/links/${worldID}/checkin`),
  checkpoint: (worldID: number) => call("POST", `/api/links/${worldID}/checkpoint`),
  renew: (worldID: number) => call("POST", `/api/links/${worldID}/renew`),
  claim: (worldID: number) => call("POST", `/api/links/${worldID}/claim`),
};
