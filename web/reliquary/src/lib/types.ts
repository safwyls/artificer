// The wire shapes reliquary's API actually returns (core/api/savesync.go,
// core/store/savesync.go). Reliquary is game-blind: nothing here names a
// game — gameTitle/gameMeta/saveHint are whatever a companion reported.

export interface Me {
  username: string;
  role: string;
  isAdmin: boolean;
  permissions: string[];
  /** True when this session began at Cloudflare Access. */
  sso?: boolean;
  /** Where to send the browser to end the *Access* session. Clearing only
   * reliquary's cookie would leave the next person at a shared machine
   * holding the previous identity. */
  ssoLogoutURL?: string;
}

export interface AppUser {
  id: number;
  username: string;
  role: string;
  permissions: string[];
  disabled: boolean;
}

export interface World {
  id: number;
  name: string;
  /** Game metadata as the companion that discovered it reported: a
   * display title, where the save lived on that machine, and free-form
   * JSON the reporter shaped. The server never interprets these, and
   * neither does anything here but lib/art.ts. */
  gameTitle: string;
  saveHint: string;
  gameMeta: string;
  /** The world's folder inside each player's own save root — the opaque
   * leaf an Unreal game generates, shared by every player of the world. */
  savePath: string;
  agentUrl: string;
  hasAgentToken: boolean;
  leaseHours: number;
  maxBytes: number;
  keepVersions: number;
  checkpoints: boolean;
  webhookUrl: string;
  headVersion?: number;
  nextHolder?: number;
  createdAt: string;
}

export interface Version {
  id: number;
  worldId: number;
  sessionId?: number;
  parentId?: number;
  /** "checkin" | "checkpoint" | "import" | … — displayed, never switched on
   * except for the CHECKPOINT badge. */
  kind: string;
  /** Checked in from a hold that could no longer move the head: an admin
   * picks a head to resolve it. */
  conflict: boolean;
  bytes: number;
  sha256: string;
  uploaderId?: number;
  createdAt: string;
}

export interface Holder {
  sessionId: number;
  userId: number;
  username: string;
  /** The dedicated server holds it, not a person. */
  serverHeld: boolean;
  expiresAt: string;
  /** A standing request for this holder's companion, waiting to be picked
   * up on its next poll: "checkpoint" or "checkin". */
  requestedKind?: string;
  requestedAt?: string;
  /** The hold is past its lease, so a takeover checkout would succeed. */
  claimable: boolean;
}

export interface WorldStatus {
  world: World;
  holder?: Holder;
  claimedBy?: string;
  head?: Version;
}

export interface WorldDetail {
  status: WorldStatus;
  versions: Version[];
  /** Uploader id (as a string key) to username, resolved server-side. */
  uploaders: Record<string, string>;
}

/** One game's cover, keyed as lib/art.ts keys it. */
export interface Artwork {
  cover?: string;
  [k: string]: unknown;
}

export interface ArtworkStatus {
  configured?: boolean;
  clientId?: string;
  lastOkAt?: string;
  lastError?: string;
  lookups?: number;
  hits?: number;
  misses?: number;
  cached?: number;
  filter?: string;
}

export interface ArtworkTest {
  ok: boolean;
  error?: string;
}

export interface ArtworkSettings {
  status?: ArtworkStatus;
  /** A credential pair saved through this panel, which wins over the
   * environment's. */
  stored?: boolean;
  envConfigured?: boolean;
  test?: ArtworkTest;
}

export interface SaveHintsStatus {
  loaded?: boolean;
  refreshing?: boolean;
  games?: number;
  steamIds?: number;
  fetchedAt?: string;
  lastError?: string;
  url?: string;
}

/** The custody state a world is in, as the chip and the primary action
 * both read it. */
export type Custody = "free" | "held" | "expired";

export function custodyOf(st: WorldStatus): Custody {
  if (!st.holder) return "free";
  return st.holder.claimable ? "expired" : "held";
}
