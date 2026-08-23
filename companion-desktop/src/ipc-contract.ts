// ipc-contract.ts — channel names shared between preload.ts and main.ts,
// kept in one place so the two never drift apart.

export const IPC = {
  getConnection: "companion:getConnection",
  pickFolder: "companion:pickFolder",
  openPath: "companion:openPath",
  setAutostart: "companion:setAutostart",
  getAutostart: "companion:getAutostart",
} as const;

/** What contextBridge exposes on window.companion in the renderer. */
export interface CompanionBridge {
  baseUrl: string;
  token: string;
  pickFolder(startDir?: string): Promise<string | null>;
  openPath(path: string): Promise<void>;
  setAutostart(enabled: boolean): Promise<void>;
  getAutostart(): Promise<boolean>;
}
