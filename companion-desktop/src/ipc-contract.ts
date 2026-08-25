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
  runInstaller: "companion:runInstaller",
} as const;

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
  /** Run a downloaded, verified installer and quit, so it can replace
   * the app that is running. The daemon stages it but cannot do this:
   * it is a file inside the application being replaced. Resolves only if
   * the installer could not be started — on success this process is on
   * its way out. */
  runInstaller(path: string): Promise<void>;
}
