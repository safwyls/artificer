// preload.ts — the only bridge between the renderer (web/companion's
// built dist/, served by companiond) and Electron/Node. contextIsolation
// is on and nodeIntegration is off (see main.ts's BrowserWindow options),
// so this is the sole surface the page gets; every privileged operation
// goes back through ipcRenderer.invoke to main, never a raw Node API.
//
// baseUrl/token are fetched from main over a synchronous IPC call made
// here in preload, before any page content runs — not via process.argv
// or a query string, both of which end up visible in the OS process
// table or navigation history. The renderer already knows how to talk
// to a companion-shaped HTTP API (docs/companion-api-surface.md); it
// just needs to be told where and with what credential.

import { contextBridge, ipcRenderer } from "electron";
import type { CompanionBridge, UpdateStatus } from "./ipc-contract";

/**
 * The channel names, repeated from ipc-contract.ts rather than imported.
 *
 * A *sandboxed* preload — which this is, by main.ts's `sandbox: true` —
 * is loaded as a single file: the `require` it gets resolves a short list
 * of Electron built-ins and nothing else, so `require("./ipc-contract")`
 * threw "module not found" and took the whole script with it. The page
 * then had no `window.companion` at all, and nothing looked wrong,
 * because the shell injects the daemon's bearer at the network layer
 * (attachDaemonAuth) — so the app still loaded and still talked to the
 * daemon while every native capability silently fell back to the browser
 * build's answer: no OS folder picker, no "open the save folder", no
 * autostart toggle.
 *
 * The type above is still imported, because `import type` is erased at
 * compile time and never becomes a require. The values cannot be, so
 * test/ipc-contract.test.js fails if these two copies ever disagree.
 */
const IPC = {
  getConnection: "companion:getConnection",
  pickFolder: "companion:pickFolder",
  openPath: "companion:openPath",
  setAutostart: "companion:setAutostart",
  getAutostart: "companion:getAutostart",
  checkForUpdate: "companion:checkForUpdate",
  downloadUpdate: "companion:downloadUpdate",
  installUpdate: "companion:installUpdate",
  updateStatus: "companion:updateStatus",
  getStartMinimized: "companion:getStartMinimized",
  setStartMinimized: "companion:setStartMinimized",
} as const;

/** Must match UpdateStatus.channel in ipc-contract.ts, for the same
 * reason the names above are repeated: a sandboxed preload cannot
 * import them. test/ipc-contract.test.js holds the copies together. */
const UPDATE_STATUS_CHANNEL = "companion:updateStatus:changed";

const connection = ipcRenderer.sendSync(IPC.getConnection) as { baseUrl: string; token: string };

const bridge: CompanionBridge = {
  baseUrl: connection.baseUrl,
  token: connection.token,
  platform: process.platform,
  pickFolder: (startDir?: string) => ipcRenderer.invoke(IPC.pickFolder, startDir),
  openPath: (path: string) => ipcRenderer.invoke(IPC.openPath, path),
  setAutostart: (enabled: boolean) => ipcRenderer.invoke(IPC.setAutostart, enabled),
  getAutostart: () => ipcRenderer.invoke(IPC.getAutostart),
  checkForUpdate: () => ipcRenderer.invoke(IPC.checkForUpdate),
  downloadUpdate: () => ipcRenderer.invoke(IPC.downloadUpdate),
  installUpdate: () => ipcRenderer.invoke(IPC.installUpdate),
  updateStatus: () => ipcRenderer.invoke(IPC.updateStatus),
  getStartMinimized: () => ipcRenderer.invoke(IPC.getStartMinimized),
  setStartMinimized: (enabled: boolean) => ipcRenderer.invoke(IPC.setStartMinimized, enabled),
  onUpdateStatus: (fn: (s: UpdateStatus) => void) => {
    // The listener is wrapped rather than passed through: what arrives
    // from main carries an IpcRendererEvent the page has no business
    // seeing, and handing a renderer callback straight to ipcRenderer
    // would leak it across the bridge.
    const listener = (_event: unknown, status: UpdateStatus) => fn(status);
    ipcRenderer.on(UPDATE_STATUS_CHANNEL, listener);
    return () => ipcRenderer.removeListener(UPDATE_STATUS_CHANNEL, listener);
  },
};

contextBridge.exposeInMainWorld("companion", bridge);
