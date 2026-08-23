// main.ts — Electron main process. Owns exactly what companion-cutover.md
// Phase 3 and docs/companion-api-surface.md assign to the shell:
// spawning/health-gating/killing companiond, the window, tray,
// single-instance raise, native dialogs, autostart, and the three
// notification policies driven off the daemon's SSE stream. Everything
// else — custody, sync, discovery — is the daemon's job over HTTP; this
// file holds no domain logic.

import { app, BrowserWindow, Tray, Menu, dialog, shell, ipcMain, Notification, nativeImage } from "electron";
import * as path from "node:path";
import { spawnDaemon, waitForHealthy, killDaemon, DaemonHandle } from "./daemon";
import { setAutostart, getAutostart } from "./autostart";
import { IPC } from "./ipc-contract";
import { watchEvents, SSEHandle } from "./sse";
import { NotifyPolicyState, StateSnapshot } from "./notifications";

let daemon: DaemonHandle | null = null;
let mainWindow: BrowserWindow | null = null;
let tray: Tray | null = null;
let sse: SSEHandle | null = null;
const notifyPolicy = new NotifyPolicyState();
let quitting = false;

function log(...args: unknown[]) {
  // eslint-disable-next-line no-console
  console.error("[companion-desktop]", ...args);
}

async function fetchState(): Promise<StateSnapshot | null> {
  if (!daemon) return null;
  try {
    const res = await fetch(`${daemon.baseUrl}/api/state`, {
      headers: { Authorization: `Bearer ${daemon.token}` },
    });
    if (!res.ok) return null;
    return (await res.json()) as StateSnapshot;
  } catch (err) {
    log("failed to fetch /api/state:", err);
    return null;
  }
}

async function pollAndNotify() {
  const state = await fetchState();
  if (!state) return;
  const hidden = !mainWindow || !mainWindow.isVisible();
  const events = notifyPolicy.evaluate(state, hidden);
  for (const ev of events) {
    new Notification({ title: ev.title, body: ev.body }).show();
  }
}

function startEventWatch() {
  if (!daemon) return;
  sse = watchEvents(
    daemon.baseUrl,
    daemon.token,
    (name) => {
      if (name === "changed" || name === "ready") {
        void pollAndNotify();
      }
    },
    (msg) => log(msg)
  );
}

async function startDaemon(): Promise<DaemonHandle> {
  const handle = await spawnDaemon({ onLog: (line) => log("companiond:", line) });
  await waitForHealthy(handle.baseUrl, { timeoutMs: 20_000 });
  return handle;
}

async function stopDaemon() {
  sse?.stop();
  sse = null;
  if (daemon) {
    await killDaemon(daemon.child);
    daemon = null;
  }
}

function iconPath(): string {
  // web/companion ships the shared favicon; reused here for tray/window icon
  // (docs/reliquary-companion.md: byte-identical icon across builds).
  return path.join(__dirname, "..", "..", "web", "companion", "public", "favicon.ico");
}

function createWindow() {
  if (!daemon) throw new Error("createWindow called before daemon is ready");

  mainWindow = new BrowserWindow({
    width: 1120,
    height: 780,
    show: false,
    icon: iconPath(),
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  const devUrl = process.env.COMPANION_DEV_URL;
  const target = devUrl || daemon.baseUrl;
  // baseUrl/token reach the renderer via preload's synchronous IPC call
  // (see ipc-contract.ts / preload.ts), not the URL or argv.
  ipcMain.on(IPC.getConnection, (event) => {
    event.returnValue = { baseUrl: daemon!.baseUrl, token: daemon!.token };
  });

  void mainWindow.loadURL(target);

  mainWindow.once("ready-to-show", () => {
    mainWindow?.show();
  });

  // Close-to-tray: closing the window hides it rather than quitting, since
  // it is a view over a resident sync process (docs/reliquary-companion.md).
  mainWindow.on("close", (e) => {
    if (quitting) return;
    e.preventDefault();
    mainWindow?.hide();
  });

  mainWindow.on("closed", () => {
    mainWindow = null;
  });
}

function createTray() {
  tray = new Tray(nativeImage.createFromPath(iconPath()));
  tray.setToolTip("Reliquary Companion");
  const menu = Menu.buildFromTemplate([
    {
      label: "Show",
      click: () => {
        mainWindow?.show();
        mainWindow?.focus();
      },
    },
    {
      label: "Hide",
      click: () => mainWindow?.hide(),
    },
    { type: "separator" },
    {
      label: "Quit",
      click: () => {
        quitting = true;
        app.quit();
      },
    },
  ]);
  tray.setContextMenu(menu);
  tray.on("click", () => {
    if (!mainWindow) return;
    if (mainWindow.isVisible()) {
      mainWindow.hide();
    } else {
      mainWindow.show();
      mainWindow.focus();
    }
  });
}

function registerIpcHandlers() {
  ipcMain.handle(IPC.pickFolder, async (_event, startDir?: string) => {
    const options = { properties: ["openDirectory" as const], defaultPath: startDir };
    const result = mainWindow
      ? await dialog.showOpenDialog(mainWindow, options)
      : await dialog.showOpenDialog(options);
    if (result.canceled || result.filePaths.length === 0) return null;
    return result.filePaths[0];
  });

  ipcMain.handle(IPC.openPath, async (_event, targetPath: string) => {
    const err = await shell.openPath(targetPath);
    if (err) throw new Error(err);
  });

  ipcMain.handle(IPC.setAutostart, async (_event, enabled: boolean) => {
    setAutostart(enabled, app);
  });

  ipcMain.handle(IPC.getAutostart, async () => {
    return getAutostart(app);
  });
}

async function bootstrap() {
  try {
    daemon = await startDaemon();
  } catch (err) {
    log("failed to start companiond:", err);
    await dialog.showMessageBox({
      type: "error",
      title: "Reliquary Companion",
      message: "Could not start the companion background service.",
      detail: String(err),
    });
    app.quit();
    return;
  }

  registerIpcHandlers();
  createWindow();
  createTray();
  startEventWatch();
}

const gotLock = app.requestSingleInstanceLock();
if (!gotLock) {
  app.quit();
} else {
  app.on("second-instance", () => {
    // The daemon owns /api/raise as a 501 placeholder (no window on the
    // daemon side); the shell that owns the window raises it directly.
    if (mainWindow) {
      if (!mainWindow.isVisible()) mainWindow.show();
      if (mainWindow.isMinimized()) mainWindow.restore();
      mainWindow.focus();
    }
  });

  app.whenReady().then(bootstrap);

  app.on("window-all-closed", () => {
    // Close-to-tray means this normally only fires on real quit (macOS
    // dock-icon-still-alive aside); ensure the daemon doesn't outlive us.
    void stopDaemon();
  });

  // will-quit must actually wait for the child to die before Electron
  // exits, or a fast shutdown can beat killDaemon's escalation and leave
  // an orphaned companiond — the defect the cutover doc calls out as the
  // one to check explicitly. preventDefault + a guarded re-quit lets us
  // await stopDaemon() first.
  let daemonStopped = false;
  app.on("will-quit", (event) => {
    if (daemonStopped) return;
    event.preventDefault();
    void stopDaemon().then(() => {
      daemonStopped = true;
      app.quit();
    });
  });

  app.on("before-quit", () => {
    quitting = true;
  });
}
