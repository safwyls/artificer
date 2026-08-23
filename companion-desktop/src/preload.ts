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
import { IPC, CompanionBridge } from "./ipc-contract";

const connection = ipcRenderer.sendSync(IPC.getConnection) as { baseUrl: string; token: string };

const bridge: CompanionBridge = {
  baseUrl: connection.baseUrl,
  token: connection.token,
  pickFolder: (startDir?: string) => ipcRenderer.invoke(IPC.pickFolder, startDir),
  openPath: (path: string) => ipcRenderer.invoke(IPC.openPath, path),
  setAutostart: (enabled: boolean) => ipcRenderer.invoke(IPC.setAutostart, enabled),
  getAutostart: () => ipcRenderer.invoke(IPC.getAutostart),
};

contextBridge.exposeInMainWorld("companion", bridge);
