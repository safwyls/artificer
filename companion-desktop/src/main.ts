// main.ts — Electron main process. Owns exactly what companion-cutover.md
// Phase 3 and docs/companion-api-surface.md assign to the shell:
// spawning/health-gating/killing companiond, the window, tray,
// single-instance raise, native dialogs, autostart, and the three
// notification policies driven off the daemon's SSE stream. Everything
// else — custody, sync, discovery — is the daemon's job over HTTP; this
// file holds no domain logic.

import { app, BrowserWindow, Tray, Menu, dialog, shell, ipcMain, Notification, nativeImage, session, Session } from "electron";
import * as path from "node:path";
import { spawnDaemon, waitForHealthy, killDaemon, DaemonHandle } from "./daemon";
import { setAutostart, getAutostart } from "./autostart";
import { readPrefs, shouldStartMinimized, writePrefs } from "./prefs";
import { IPC } from "./ipc-contract";
import { watchEvents, SSEHandle } from "./sse";
import {
  checkForUpdate,
  checkOnStartup,
  downloadUpdate,
  initUpdater,
  installUpdate,
  updateStatus,
} from "./updater";
import { NotifyPolicyState, StateSnapshot } from "./notifications";

let daemon: DaemonHandle | null = null;
let mainWindow: BrowserWindow | null = null;
let tray: Tray | null = null;
let sse: SSEHandle | null = null;
const notifyPolicy = new NotifyPolicyState();
let quitting = false;

// electron-builder's productName, repeated for the window title, the tray
// and the dialogs. The browser-and-tray build (cmd/companion) is the
// *Artificer* Companion and is a separate product with its own release
// track — see docs/reliquary-companion.md.
const APP_NAME = "Reliquary Companion";

// electron-builder.yml's `appId`, and it has to stay equal to it —
// test/app-identity.test.js holds the two together.
//
// Windows uses the AppUserModelID to decide which taskbar button a window
// belongs to and whose icon and name a toast notification carries. With
// none set, an unpackaged run is grouped under electron.exe and wears
// Electron's identity instead of ours — which is why a dev build could
// show the wrong icon no matter what `build/icon.png` contained. It also
// matters for the three notifications this shell raises. A no-op on
// macOS and Linux.
const APP_ID = "com.artificer.reliquarycompanion";

// The window wears the app's chrome, not the platform's. web/companion
// draws a titlebar into the top of the page (TitleBar.tsx) and marks it
// draggable; the OS still draws the caption buttons, recoloured to sit
// in that strip.
//
// `titleBarOverlay` rather than buttons of our own: it is what keeps
// Windows 11 snap layouts appearing on hover, and losing those to match
// a palette would be a bad trade.
//
// The height here is a *request*. What the OS actually drew is reported
// back to the page as the Window Controls Overlay's `env(titlebar-area-*)`,
// and TitleBar.tsx sizes itself from that rather than from this number —
// the two disagreeing is what put the caption buttons across the strip's
// bottom rule.
const TITLEBAR_HEIGHT = 38;
const TITLEBAR_BG = "#100d17"; // --ink
const TITLEBAR_FG = "#e8e0cf"; // --parchment

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
  const handle = await spawnDaemon({
    onLog: (line) => log("companiond:", line),
    appInfo: {
      isPackaged: app.isPackaged,
      platform: process.platform,
      resourcesPath: process.resourcesPath,
    },
  });
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
  // build/icon.png travels inside the asar (electron-builder.yml `files`),
  // so this same relative path resolves both from dist/ in the source tree
  // and from app.asar/dist in a packaged one. It used to point out at
  // web/companion/public/favicon.ico, which is not shipped in the package
  // at all — so every packaged build handed the tray an empty image.
  return path.join(__dirname, "..", "build", "icon.png");
}

// The tray wants a tray-sized image, not the 1024px app icon: some
// platforms scale a huge source badly, and Linux tray implementations
// vary enough that handing them the intended size is the safe move.
function trayImage() {
  const img = nativeImage.createFromPath(iconPath());
  if (img.isEmpty()) return img;
  const px = process.platform === "darwin" ? 22 : 32;
  return img.resize({ width: px, height: px });
}

/**
 * The window loads the daemon's page as an ordinary top-level navigation,
 * and a navigation cannot carry an Authorization header — so without this
 * the daemon answers the document request itself with its 401 JSON, and
 * that JSON is the whole window.
 *
 * Injecting the bearer here rather than exempting the page from the token
 * server-side keeps the property the daemon was built around: a loopback
 * port is reachable by every process on the machine, and nothing that did
 * not get the token from this shell gets an answer. The URL filter is the
 * daemon's own origin, so the token never rides along anywhere else the
 * page might reach.
 */
function attachDaemonAuth(sess: Session, baseUrl: string, token: string) {
  sess.webRequest.onBeforeSendHeaders({ urls: [`${baseUrl}/*`] }, (details, callback) => {
    callback({
      requestHeaders: { ...details.requestHeaders, Authorization: `Bearer ${token}` },
    });
  });
}

/**
 * Load the window once, headlessly, and assert the daemon's page is what
 * came back — then exit. Run by `npm run smoke` (needs a display; use
 * xvfb-run where there is none).
 *
 * This exists because the shell shipped a window showing nothing but the
 * daemon's own `missing or wrong bearer token` JSON. Every other check
 * passed while it did: the daemon was healthy, the handshake worked, no
 * process leaked. They all watched the processes and never the page.
 */
async function runSmoke(win: BrowserWindow) {
  // app.exit() skips will-quit, so tear the daemon down explicitly rather
  // than leaning on its stdin-close safety net — an orphaned companiond is
  // the defect this architecture is most prone to, and a check that leaks
  // one while reporting success would be worse than no check.
  const finish = async (code: number) => {
    await stopDaemon();
    app.exit(code);
  };
  const fail = (why: string) => {
    log("SMOKE FAIL:", why);
    void finish(1);
  };
  win.webContents.once("did-fail-load", (_e, code, desc) => fail(`load failed ${code} ${desc}`));
  win.webContents.once("did-finish-load", () => {
    setTimeout(async () => {
      try {
        const text: string = await win.webContents.executeJavaScript(
          "document.body ? document.body.innerText.slice(0, 400) : ''",
        );
        if (/missing or wrong bearer token|"ok"\s*:\s*false/i.test(text)) {
          return fail(`daemon rejected the page request: ${text.trim()}`);
        }
        // The renderer mounts into #root; an error page or a blank
        // document has no such node, so this distinguishes "the app drew
        // itself" from "something loaded".
        const mounted: boolean = await win.webContents.executeJavaScript(
          "!!document.querySelector('#root') && document.querySelector('#root').childElementCount > 0",
        );
        if (!mounted) return fail(`page loaded but the app did not mount; body was: ${text.trim()}`);
        log("SMOKE OK: the companion's page rendered");
        await finish(0);
      } catch (err) {
        fail(String(err));
      }
    }, 1500);
  });
}

/**
 * The frameless-window options, which differ by platform in where the
 * caption buttons land: three at the right on Windows and Linux, the
 * traffic lights at the left on macOS. TitleBar.tsx reserves the
 * matching end of the strip.
 */
function titleBarOptions() {
  if (process.platform === "darwin") {
    return {
      titleBarStyle: "hidden" as const,
      // Centred vertically in a 36px strip: the lights are 12px tall.
      trafficLightPosition: { x: 14, y: 12 },
    };
  }
  return {
    titleBarStyle: "hidden" as const,
    titleBarOverlay: {
      color: TITLEBAR_BG,
      symbolColor: TITLEBAR_FG,
      height: TITLEBAR_HEIGHT,
    },
  };
}

/**
 * The menu bar is chrome this app never used: every command it held is
 * on the page or in the tray, and a grey native menu strip above a dark
 * window reads as two applications stacked. It goes entirely on Windows
 * and Linux.
 *
 * macOS is the exception, and not a cosmetic one — its menu is not in
 * the window, and removing it takes Cmd+Q, Cmd+C and Cmd+V with it. So
 * that platform keeps a role-only menu, which is the standard set and
 * nothing of ours.
 */
function installAppMenu() {
  if (process.platform !== "darwin") {
    Menu.setApplicationMenu(null);
    return;
  }
  Menu.setApplicationMenu(
    Menu.buildFromTemplate([{ role: "appMenu" }, { role: "editMenu" }, { role: "windowMenu" }]),
  );
}

function createWindow() {
  if (!daemon) throw new Error("createWindow called before daemon is ready");

  mainWindow = new BrowserWindow({
    width: 1120,
    height: 780,
    show: false,
    icon: iconPath(),
    // The page ground, so a frameless window does not flash white in the
    // gap between "created" and "painted".
    backgroundColor: TITLEBAR_BG,
    ...titleBarOptions(),
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  const devUrl = process.env.COMPANION_DEV_URL;
  const target = devUrl || daemon.baseUrl;
  // Before the first request leaves, including the navigation below.
  attachDaemonAuth(session.defaultSession, daemon.baseUrl, daemon.token);
  // baseUrl/token reach the renderer via preload's synchronous IPC call
  // (see ipc-contract.ts / preload.ts), not the URL or argv.
  ipcMain.on(IPC.getConnection, (event) => {
    event.returnValue = { baseUrl: daemon!.baseUrl, token: daemon!.token };
  });

  // The window is the Reliquary Companion; the page it loads is titled
  // for the browser build it is also served to. Without this the OS
  // taskbar and alt-tab take the document's title and call this window
  // by the other product's name.
  mainWindow.setTitle(APP_NAME);
  mainWindow.on("page-title-updated", (e) => e.preventDefault());

  void mainWindow.loadURL(target);

  if (process.env.COMPANION_SMOKE) void runSmoke(mainWindow);

  // With no menu there is no F12 accelerator either, and the devtools are
  // the first thing anyone reaches for when the renderer misbehaves. Only
  // in an unpackaged build: a shipped app should not open them by
  // accident.
  if (!app.isPackaged) {
    mainWindow.webContents.on("before-input-event", (event, input) => {
      const devtools =
        input.key === "F12" || (input.control && input.shift && input.key.toLowerCase() === "i");
      if (input.type === "keyDown" && devtools) {
        mainWindow?.webContents.toggleDevTools();
        event.preventDefault();
      }
    });
  }

  // Updates are the shell's job (updater.ts): companiond does not watch
  // for them, because the thing replaced is the application it is inside.
  initUpdater(mainWindow, log);
  checkOnStartup();

  // Into the tray, when asked. Either the stored preference or the
  // `--minimized` the login item passes — and that argument was being
  // passed to nothing until now: autostart registered it, no code read
  // it, and logging in put a window in your face regardless.
  //
  // The window is still created either way, so the tray can raise it
  // instantly and the daemon is already being watched.
  const minimized = shouldStartMinimized(app.getPath("userData"), process.argv);
  if (minimized) log("starting minimized to the tray");
  mainWindow.once("ready-to-show", () => {
    if (!minimized) mainWindow?.show();
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
  tray = new Tray(trayImage());
  tray.setToolTip(APP_NAME);
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

  ipcMain.handle(IPC.getStartMinimized, async () => {
    return readPrefs(app.getPath("userData")).startMinimized;
  });
  ipcMain.handle(IPC.setStartMinimized, async (_event, enabled: boolean) => {
    writePrefs(app.getPath("userData"), { startMinimized: Boolean(enabled) });
  });

  ipcMain.handle(IPC.checkForUpdate, () => checkForUpdate());
  ipcMain.handle(IPC.updateStatus, () => updateStatus());
  ipcMain.handle(IPC.downloadUpdate, () => downloadUpdate());
  ipcMain.handle(IPC.installUpdate, async () => {
    // The daemon goes down with us through the usual will-quit path, so
    // the installer is not racing a live companiond for its files.
    installUpdate(() => {
      quitting = true;
    });
  });
}

async function bootstrap() {
  // Before the first window and the first notification: both read it.
  app.setAppUserModelId(APP_ID);

  try {
    daemon = await startDaemon();
  } catch (err) {
    log("failed to start companiond:", err);
    await dialog.showMessageBox({
      type: "error",
      title: APP_NAME,
      message: "Could not start the companion background service.",
      detail: String(err),
    });
    app.quit();
    return;
  }

  installAppMenu();
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
