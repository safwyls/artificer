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
import type { CompanionBridge } from "./ipc-contract";

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
} as const;

const connection = ipcRenderer.sendSync(IPC.getConnection) as { baseUrl: string; token: string };

const bridge: CompanionBridge = {
  baseUrl: connection.baseUrl,
  token: connection.token,
  platform: process.platform,
  pickFolder: (startDir?: string) => ipcRenderer.invoke(IPC.pickFolder, startDir),
  openPath: (path: string) => ipcRenderer.invoke(IPC.openPath, path),
  setAutostart: (enabled: boolean) => ipcRenderer.invoke(IPC.setAutostart, enabled),
  getAutostart: () => ipcRenderer.invoke(IPC.getAutostart),
};

contextBridge.exposeInMainWorld("companion", bridge);
