// ipc-contract.ts — the channel names and the shape of what the preload
// exposes, and the definition main.ts works from.
//
// preload.ts cannot import the values here: a sandboxed preload is loaded
// as a single file and its `require` resolves only Electron built-ins, so
// it keeps its own copy and test/ipc-contract.test.js holds the two to
// each other. It does import the `CompanionBridge` type, which is erased.

export const IPC = {
  getConnection: "companion:getConnection",
  pickFolder: "companion:pickFolder",
  openPath: "companion:openPath",
  setAutostart: "companion:setAutostart",
  getAutostart: "companion:getAutostart",
  checkForUpdate: "companion:checkForUpdate",
  downloadUpdate: "companion:downloadUpdate",
  installUpdate: "companion:installUpdate",
  updateStatus: "companion:updateStatus",
} as const;

/**
 * Update status, pushed to the page as it changes rather than polled.
 *
 * The shell owns updates here — companiond does not watch for them,
 * because the thing that gets replaced is the application it lives
 * inside. So this is the page's only source of update state, and it is
 * one source on purpose.
 */
export const UpdateStatus = {
  /** main → renderer, on every change. */
  channel: "companion:updateStatus:changed",
} as const;

export interface UpdateStatus {
  state:
    | "idle"
    | "checking"
    /** The release ships the build already running. */
    | "current"
    | "available"
    | "downloading"
    /** Downloaded and verified; installing is one call away. */
    | "ready"
    | "error"
    /** This build cannot update itself, and `why` says so. */
    | "unsupported";
  version?: string;
  /** 0-100 while downloading. */
  percent?: number;
  /** Bytes fetched so far, and how many this download is. Carried
   * because they are the only way to see whether a differential update
   * happened: an update that pulls the whole ~82MB installer works
   * exactly like one that pulls two, and "it felt fast" is not a
   * measurement. */
  transferred?: number;
  total?: number;
  why?: string;
}

/** What contextBridge exposes on window.companion in the renderer. */
export interface CompanionBridge {
  baseUrl: string;
  token: string;
  /** `process.platform`. The renderer draws the window's titlebar and
   * has to leave room for the caption buttons the OS draws over it —
   * which end of the strip those land on is a platform question. */
  platform: NodeJS.Platform;
  pickFolder(startDir?: string): Promise<string | null>;
  openPath(path: string): Promise<void>;
  setAutostart(enabled: boolean): Promise<void>;
  getAutostart(): Promise<boolean>;
  /** Ask the update feed now, and answer with what it said. */
  checkForUpdate(): Promise<UpdateStatus>;
  /** Download the update the last check found. Progress arrives on the
   * subscription below, not here. */
  downloadUpdate(): Promise<void>;
  /** Install what was downloaded and relaunch. Resolves only if the
   * install could not be started — on success this process is going
   * away. */
  installUpdate(): Promise<void>;
  /** Current status, for a page that has just loaded and missed the
   * events so far. */
  updateStatus(): Promise<UpdateStatus>;
  /** Subscribe to status changes. Returns the unsubscribe. */
  onUpdateStatus(fn: (s: UpdateStatus) => void): () => void;
}
