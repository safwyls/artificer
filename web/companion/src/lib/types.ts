// The shapes the companion's own local API returns (cmd/companion:
// server.go, discover.go, savedirs.go, savepath.go, browse.go, sync.go).
// Everything here is local to the player's machine except `sync`, which
// is what the service last said.

/** One save folder this machine offers for a game, with why it was
 * offered — a Steam Cloud hit is exact, the rest are guesses the player
 * confirms. */
export interface SaveCandidate {
  path: string;
  why: string;
}

export interface DiscoveredGame {
  name: string;
  appId?: string;
  installDir?: string;
  saveDirs?: SaveCandidate[];
  /** Filled in when the state is served: the page's view of this game,
   * not a property of the install. */
  key?: string;
  hidden?: boolean;
  /** Set by the by-hand link path — a blank game standing in for a folder
   * discovery never found. Never comes from the server. */
  byHand?: boolean;
}

/** One place the scan looked and what it found, so an empty shelf can
 * explain itself. */
export interface Probe {
  source: string;
  path: string;
  resolved?: string;
  note: string;
}

export interface Discovery {
  games: DiscoveredGame[];
  probes: Probe[];
  libraries?: string[];
}

/** A world this machine has linked to a folder. */
export interface Link {
  worldId: number;
  gameTitle?: string;
  dir: string;
  appId?: string;
  /** Overrides what starts this game. A path or a URI the desktop knows
   * how to open — an .exe, a .lnk, another launcher's URI scheme — never
   * a command line. Empty means Steam's own run URI, from appId. */
  launchTarget?: string;
  sessionId?: number;
  baseVersion?: number;
}

/**
 * What the companion will open to start this world's game, or "" when it
 * has nothing to start — a folder linked by hand carries no app id, and
 * the companion must not guess. Mirrors launchTarget() in launch.go; the
 * companion is still the one that decides, this only labels the button.
 */
export function launchTargetOf(link: Link): string {
  const override = (link.launchTarget ?? "").trim();
  if (override) return override;
  return link.appId ? `steam://rungameid/${link.appId}` : "";
}

export const launchable = (link: Link) => launchTargetOf(link) !== "";

export interface SyncWorld {
  world: {
    id: number;
    name: string;
    gameTitle: string;
    saveHint: string;
    checkpoints: boolean;
    savePath: string;
    headVersion?: number | null;
  };
  holder?: {
    sessionId: number;
    username: string;
    expiresAt: string;
    claimable: boolean;
    requestedKind?: string;
  };
  claimedBy?: string;
  head?: { id: number; bytes: number; createdAt: string };
}

export interface SyncState {
  configured: boolean;
  username?: string;
  worlds?: SyncWorld[];
  busy: boolean;
  lastError?: string;
  lastAction?: string;
  polledAt?: string;
  serverVersion?: string;
}

/** What GitHub last said about the current release (cmd/companion:
 * update.go). Convenience, never custody. */
export interface UpdateState {
  /** The release names a build that is not this one. Deliberately not
   * "newer": every build is stamped with a commit SHA, and SHAs have no
   * order, so identity is the only honest question. */
  available: boolean;
  version?: string;
  checkedAt?: string;
  error?: string;
  applying?: boolean;
  /** False when this install cannot replace itself — no writable
   * directory, or a platform with no published release. */
  supported: boolean;
  why?: string;
}

export interface CompanionState {
  config: {
    serverUrl: string;
    tokenSet: boolean;
    steamDirs: string[];
    /** Start the game once a checkout has put the save in place. */
    launchOnCheckout: boolean;
  };
  links: Link[];
  discovered: Discovery;
  sync: SyncState;
  version: string;
  update?: UpdateState;
}

export interface Artwork {
  cover?: string;
  name?: string;
}

export interface BrowseEntry {
  name: string;
  path: string;
  saveish?: boolean;
}

export interface Browse {
  path: string;
  parent?: string;
  entries: BrowseEntry[];
  roots: { label: string; path: string }[];
  error?: string;
}

/** Where a chosen folder divides into the part a joining player supplies
 * and the part the world carries with it. */
export interface SplitInfo {
  root: string;
  leaf: string;
  why?: string;
}

/**
 * gameKey matches the service's artwork map key and the companion's own
 * hidden-list key. One identity for a game, used by all three — art, the
 * hide list, and the server.
 */
export const gameKey = (g: { appId?: string; name?: string }) =>
  g.appId ? `app:${g.appId}` : `name:${String(g.name || "").toLowerCase().trim()}`;

/**
 * artFor resolves a cover for anything that can name a game: a discovered
 * game, or a link that remembers which one it came from. A link made
 * before app ids were recorded still matches by title.
 */
export function artFor(
  art: Record<string, Artwork>,
  g: { appId?: string; name?: string },
): Artwork {
  return (
    art[gameKey(g)] ??
    (g.name ? art[`name:${String(g.name).toLowerCase().trim()}`] : undefined) ??
    {}
  );
}

/**
 * The custody state a linked world is in.
 *
 * The redesign names four (`free | yours | held | expired`); this app has
 * always carried two more, and they are real rather than decorative:
 * `fetching` is "your account holds it, but on another session — the save
 * is still on its way here", and `gone` is "the service does not have
 * this world any more". Both need a different answer from the four, so
 * they stay. `mine` is this app's name for the handoff's `yours`.
 */
export type CustodyState = "free" | "mine" | "fetching" | "held" | "expired" | "gone";

/**
 * One custody answer, and the only one. The chip and the row's primary
 * action are both derived from this single value so they cannot disagree
 * — a chip reading "Free" beside a disabled Check out is the failure this
 * shape exists to make impossible. Nothing else may decide either.
 */
export interface Custody {
  state: CustodyState;
  /** Who holds it, when someone does — including you. */
  holder?: string;
  /** When the hold lapses, as the service reported it. */
  expiresAt?: string;
  /** Who is queued for it next, when anyone is. */
  claimedBy?: string;
}

export function custodyOf(
  link: Link,
  world: SyncWorld | undefined,
  me: string | undefined,
  configured: boolean,
): Custody {
  if (!world) return { state: configured ? "gone" : "free" };
  const h = world.holder;
  const rest = { holder: h?.username, expiresAt: h?.expiresAt, claimedBy: world.claimedBy };
  if (!h) return { state: "free", ...rest };
  if (h.username === me) {
    // The service says this account holds it, but this machine has no
    // session for it: another machine of theirs took it, or the download
    // is still on its way here.
    return { state: link.sessionId === h.sessionId ? "mine" : "fetching", ...rest };
  }
  return { state: h.claimable ? "expired" : "held", ...rest };
}

/** How long a hold has left, in ms — negative once it has lapsed, and
 * NaN when there is no hold to measure. */
export function holdLeft(custody: Custody, now = Date.now()): number {
  if (!custody.expiresAt) return NaN;
  const at = new Date(custody.expiresAt).getTime();
  return Number.isNaN(at) ? NaN : at - now;
}

/** Under three hours: the point at which a hold stops being a fact and
 * starts being pressure, and the only point at which a countdown is
 * worth the ink. */
export const HOLD_PRESSURE_MS = 3 * 60 * 60 * 1000;

export function holdIsPressing(custody: Custody, now = Date.now()): boolean {
  const left = holdLeft(custody, now);
  return !Number.isNaN(left) && left > 0 && left < HOLD_PRESSURE_MS;
}
