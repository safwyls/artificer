import type {
  AppUser,
  Artwork,
  ArtworkSettings,
  ArtworkTest,
  Me,
  SaveHintsStatus,
  Version,
  World,
  WorldDetail,
  WorldStatus,
} from "./types";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

/** Registered by the auth provider. A 401 from any endpoint means the
 * session expired (or was revoked) mid-use; clearing auth state there lets
 * RequireAuth bounce to /login once instead of every query surfacing its
 * own scattered error. */
let onUnauthorized: (() => void) | null = null;

export function setUnauthorizedHandler(handler: (() => void) | null) {
  onUnauthorized = handler;
}

/** The old page's `j()`, typed: one fetch wrapper that speaks JSON, keeps
 * the session cookie, treats 204 as an empty body, and turns a non-2xx into
 * an ApiError carrying the server's own `error` string — which is what
 * every custody refusal explains itself in. */
async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api${path}`, {
    method,
    credentials: "include",
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return undefined as T;
  const out = await res.json().catch(() => ({}));
  if (!res.ok) {
    // /login's own 401 is just a wrong password, not an expired session,
    // and /login/cloudflare's is "no assertion on this request" — the
    // login page says both out loud itself.
    if (res.status === 401 && !path.startsWith("/login")) onUnauthorized?.();
    const message =
      out && typeof out === "object" && "error" in out ? String(out.error) : res.statusText;
    throw new ApiError(res.status, message);
  }
  return out as T;
}

/** Upload a save bundle: a raw `.tar` POST, outside the JSON body cap.
 * Both check-in and import take this shape. */
async function upload(path: string, file: File): Promise<{ version?: Version }> {
  const res = await fetch(`/api${path}`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/x-tar" },
    body: file,
  });
  const out = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new ApiError(res.status, out?.error ? String(out.error) : res.statusText);
  }
  return out as { version?: Version };
}

/** The server's explanation for a failed request — custody refusals name
 * who holds the world and whether a takeover would work. */
export function errorDetail(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return "something went wrong";
}

/** Download URLs are plain links, not fetches: the browser's own download
 * carries the session cookie and streams gigabytes we never want in JS. */
export const downloadURL = (worldID: number, versionID: number) =>
  `/api/sync/worlds/${worldID}/versions/${versionID}/download`;

/** The companion download is token-gated rather than cookie-gated, so each
 * player's link carries their own credential. */
export const companionURL = (token: string) =>
  `/api/public/sync/${encodeURIComponent(token)}/companion/download`;

export interface WorldWriteInput {
  name: string;
  leaseHours: number;
  maxBytes: number;
  keepVersions: number;
  checkpoints: boolean;
  savePath: string;
  webhookUrl: string;
  agentUrl: string;
  /** Empty keeps the saved one — the API never returns it. */
  agentToken: string;
}

export interface UserWriteInput {
  role: string;
  permissions: string[];
  disabled: boolean;
}

export const api = {
  version: () => request<{ version: string }>("GET", "/version"),

  // --- auth ---
  me: () => request<Me>("GET", "/me"),
  login: (username: string, password: string) =>
    request<void>("POST", "/login", { username, password }),
  loginCloudflare: () => request<void>("POST", "/login/cloudflare"),
  logout: () => request<void>("POST", "/logout"),

  // --- worlds and custody ---
  worlds: () => request<{ worlds: WorldStatus[] }>("GET", "/sync/worlds"),
  world: (id: number) => request<WorldDetail>("GET", `/sync/worlds/${id}`),
  createWorld: (name: string) => request<{ world: World }>("POST", "/sync/worlds", { name }),
  updateWorld: (id: number, input: WorldWriteInput) =>
    request<{ world: World }>("PUT", `/sync/worlds/${id}`, input),
  deleteWorld: (id: number) => request<void>("DELETE", `/sync/worlds/${id}`),
  checkout: (id: number, takeover: boolean) =>
    request<unknown>("POST", `/sync/worlds/${id}/checkout`, { takeover }),
  claim: (id: number) => request<unknown>("POST", `/sync/worlds/${id}/claim`),
  unclaim: (id: number) => request<unknown>("DELETE", `/sync/worlds/${id}/claim`),
  renew: (sessionID: number) => request<unknown>("POST", `/sync/sessions/${sessionID}/renew`),
  checkin: (sessionID: number, file: File) => upload(`/sync/sessions/${sessionID}/checkin`, file),
  importSave: (worldID: number, file: File) => upload(`/sync/worlds/${worldID}/import`, file),
  /** kind: "checkin" | "checkpoint", or "" to withdraw the request. */
  requestHandback: (id: number, kind: string) =>
    request<unknown>("POST", `/sync/worlds/${id}/request`, { kind }),
  release: (id: number) => request<unknown>("POST", `/sync/worlds/${id}/release`),
  setHead: (id: number, versionId: number) =>
    request<unknown>("POST", `/sync/worlds/${id}/head`, { versionId }),
  serverGive: (id: number) => request<unknown>("POST", `/sync/worlds/${id}/server/give`),
  serverTake: (id: number) => request<unknown>("POST", `/sync/worlds/${id}/server/take`),

  // --- companion token ---
  syncToken: () => request<{ token?: string }>("GET", "/me/sync-token"),
  mintSyncToken: () => request<{ token?: string }>("POST", "/me/sync-token"),
  revokeSyncToken: () => request<unknown>("DELETE", "/me/sync-token"),

  // --- users (admin) ---
  users: () => request<AppUser[]>("GET", "/users"),
  createUser: (username: string, password: string, role: string) =>
    request<unknown>("POST", "/users", { username, password, role, permissions: ["savesync"] }),
  /** The API *replaces* role, permissions and disabled together, so every
   * caller sends the whole record — see saveUser in lib/users.ts. */
  updateUser: (id: number, input: UserWriteInput) =>
    request<unknown>("PUT", `/users/${id}`, input),
  deleteUser: (id: number) => request<void>("DELETE", `/users/${id}`),

  // --- cover art ---
  artwork: (games: { appId: string; name: string }[]) =>
    request<{ art?: Record<string, Artwork> }>("POST", "/sync/artwork", { games }),
  artworkSettings: () => request<ArtworkSettings>("GET", "/sync/artwork/settings"),
  saveArtworkSettings: (clientId: string, clientSecret: string) =>
    request<{ test?: ArtworkTest }>("PUT", "/sync/artwork/settings", { clientId, clientSecret }),
  clearArtworkSettings: () => request<unknown>("DELETE", "/sync/artwork/settings"),
  testArtwork: () => request<{ test?: ArtworkTest }>("POST", "/sync/artwork/test"),

  // --- save-location catalogue ---
  saveHintsStatus: () => request<{ status?: SaveHintsStatus }>("GET", "/sync/savehints/status"),
  refreshSaveHints: () =>
    request<{ refreshed?: boolean; error?: string; status?: SaveHintsStatus }>(
      "POST",
      "/sync/savehints/refresh",
    ),
};
