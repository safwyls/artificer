// updater.ts — the shell updates the application, because the
// application is what gets replaced.
//
// The browser-and-tray companion (`cmd/companion`) is a single exe a
// player keeps wherever they like, and it replaces itself: the Go
// engine's update.go downloads a new binary over the old one and
// restarts. That is right for a file, and wrong for an installed app.
// Here the release asset is an *installer*, the thing being replaced is
// the whole application, and companiond is one file inside it — on
// Windows a running executable cannot be overwritten at all.
//
// So companiond does not watch for updates and this does, through
// electron-updater. One owner: two checkers would be two answers to "is
// there an update", and two readings of one fact drift.
//
// What electron-updater brings that a hand-rolled version did not:
// differential downloads against the NSIS blockmap (so a small change is
// a small download rather than the whole ~90MB installer), progress,
// resume, and `quitAndInstall` — which installs silently and relaunches,
// instead of leaving someone clicking through a wizard and starting the
// app again themselves.

import { autoUpdater } from "electron-updater";
import type { BrowserWindow } from "electron";
import { UpdateStatus } from "./ipc-contract";

/**
 * macOS cannot auto-update here and it is not worth pretending.
 * Squirrel.Mac verifies a code signature before swapping an app in, and
 * these builds are unsigned (the release workflow says so, and there are
 * no certificates in this repo). The check would download and then
 * refuse, so it is not offered.
 */
const canUpdate = process.platform !== "darwin";

let status: UpdateStatus = { state: canUpdate ? "idle" : "unsupported" };
if (!canUpdate) {
  status.why = "macOS builds are unsigned, so they are replaced by hand from the .dmg";
}

let send: ((s: UpdateStatus) => void) | null = null;

function set(next: UpdateStatus) {
  status = next;
  send?.(status);
}

export function updateStatus(): UpdateStatus {
  return status;
}

/**
 * Wire the updater to a window, so status reaches the page as it
 * changes rather than only when the page asks.
 *
 * `autoDownload` is off: a ~90MB download that starts because the app
 * happened to notice something is a download nobody asked for, on a
 * connection this app knows nothing about. The page offers it and the
 * player decides — the same shape the browser build has always had.
 */
export function initUpdater(win: BrowserWindow, log: (...args: unknown[]) => void) {
  send = (s) => {
    if (!win.isDestroyed()) win.webContents.send(UpdateStatus.channel, s);
  };
  if (!canUpdate) return;

  autoUpdater.autoDownload = false;
  // Left at the default, false, which is what makes the GitHub provider
  // resolve through /releases/latest — the endpoint that excludes
  // prereleases. This app publishes normal, semver-tagged releases and
  // nothing else in this repository does, so that names exactly one
  // thing. `allowPrerelease` would instead walk the whole releases list,
  // which is where another product's rolling prerelease could turn up.
  autoUpdater.allowPrerelease = false;
  // Unsigned builds: there is no publisher name for Windows to check a
  // downloaded installer against, and refusing to install our own
  // release helps nobody. The blockmap and size checks still apply.
  autoUpdater.autoInstallOnAppQuit = false;
  // electron-updater's own logging goes to our log, so an update that
  // fails quietly is not a thing.
  autoUpdater.logger = {
    info: (m: unknown) => log("updater:", m),
    warn: (m: unknown) => log("updater warn:", m),
    error: (m: unknown) => log("updater error:", m),
    debug: () => {},
  };

  autoUpdater.on("checking-for-update", () => set({ state: "checking" }));
  autoUpdater.on("update-not-available", (info) =>
    set({ state: "current", version: info?.version }),
  );
  autoUpdater.on("update-available", (info) =>
    set({ state: "available", version: info?.version }),
  );
  autoUpdater.on("download-progress", (p) =>
    set({
      state: "downloading",
      version: status.version,
      percent: Math.round(p?.percent ?? 0),
      transferred: p?.transferred,
      total: p?.total,
    }),
  );
  autoUpdater.on("update-downloaded", (info) => {
    // What a differential update looks like from here, recorded where a
    // bug report can find it: a download much smaller than the installer
    // means the blockmap matched, and one the same size means it did not.
    const total = status.total;
    log(
      "updater: downloaded",
      info?.version,
      total ? `(${Math.round(total / 1_000_000)} MB fetched)` : "(size unknown)",
    );
    set({ state: "ready", version: info?.version, total });
  });
  autoUpdater.on("error", (err) =>
    // Never fatal. Not knowing about an update is not a problem with the
    // companion, and it must never look like one: custody is unaffected.
    set({ state: "error", version: status.version, why: String(err?.message ?? err) }),
  );
}

/** Ask now. Safe to call repeatedly; electron-updater coalesces. */
export async function checkForUpdate(): Promise<UpdateStatus> {
  if (!canUpdate) return status;
  try {
    await autoUpdater.checkForUpdates();
  } catch (err) {
    set({ state: "error", why: String(err) });
  }
  return status;
}

/** Download the update the last check found. Progress arrives as events. */
export async function downloadUpdate(): Promise<void> {
  if (!canUpdate) throw new Error(status.why ?? "this build cannot update itself");
  await autoUpdater.downloadUpdate();
}

/**
 * Install and come back.
 *
 * `quitAndInstall(false, true)`: not silent — an unsigned installer
 * raises SmartScreen, and a UAC prompt behind a window that has already
 * vanished is a companion that simply disappeared. The second argument
 * relaunches afterwards, which is the half that matters.
 *
 * Returns only if the install could not be started; otherwise this
 * process is on its way out.
 */
export function installUpdate(beforeQuit: () => void): void {
  if (status.state !== "ready") {
    throw new Error("no update has been downloaded yet");
  }
  beforeQuit();
  autoUpdater.quitAndInstall(false, true);
}

/**
 * Check once at startup, quietly.
 *
 * Delayed, because the first seconds after launch belong to the daemon
 * handshake and the first custody poll — the things someone opened the
 * app for.
 */
export function checkOnStartup(delayMs = 20_000): NodeJS.Timeout | null {
  if (!canUpdate) return null;
  return setTimeout(() => void checkForUpdate(), delayMs);
}
