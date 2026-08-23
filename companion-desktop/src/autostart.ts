// autostart.ts — login-item toggle (docs/companion-api-surface.md op #33).
// Windows/macOS use Electron's built-in app.setLoginItemSettings. Linux has
// no Electron equivalent, so we write/remove a standard XDG autostart
// .desktop file, matching the prior Fyne-era behavior of "Windows-only via
// registry, other platforms answer with a reason" — except here we make
// Linux a real citizen since that's the primary dev platform
// (companion-cutover.md guardrails: Nobara is the dev target).

import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";

const DESKTOP_FILE_NAME = "reliquary-companion.desktop";

/**
 * The arguments the login entry carries, and the reason this is a
 * constant rather than a literal at each call site.
 *
 * On Windows, `getLoginItemSettings` does not simply report "is there a
 * Run entry" — it compares against a path and an argument list, both
 * defaulting to `process.execPath` and *no arguments*. Registering with
 * `["--minimized"]` and then asking with the default empty list is a
 * mismatch, so the answer was always `openAtLogin: false`: the setting
 * was written, and reading it back said it had not been. The checkbox
 * turned itself off again the next time Settings was opened.
 *
 * Set and get have to agree, so they share this.
 */
const AUTOSTART_ARGS = ["--minimized"];

function autostartDir(): string {
  const xdgConfig = process.env.XDG_CONFIG_HOME || path.join(os.homedir(), ".config");
  return path.join(xdgConfig, "autostart");
}

function desktopFilePath(): string {
  return path.join(autostartDir(), DESKTOP_FILE_NAME);
}

function linuxDesktopEntry(execPath: string): string {
  return `[Desktop Entry]
Type=Application
Name=Reliquary Companion
Exec=${execPath} --minimized
X-GNOME-Autostart-enabled=true
`;
}

export interface AutostartApp {
  // Subset of Electron's app used here, so this module stays testable
  // without importing electron directly.
  getLoginItemSettings(options?: { path?: string; args?: string[] }): { openAtLogin: boolean };
  setLoginItemSettings(settings: {
    openAtLogin: boolean;
    path?: string;
    args?: string[];
  }): void;
  getPath(name: string): string;
}

export function setAutostart(enabled: boolean, app: AutostartApp): void {
  if (process.platform === "linux") {
    const dir = autostartDir();
    const file = desktopFilePath();
    if (enabled) {
      fs.mkdirSync(dir, { recursive: true });
      fs.writeFileSync(file, linuxDesktopEntry(process.execPath));
    } else if (fs.existsSync(file)) {
      fs.unlinkSync(file);
    }
    return;
  }
  // win32 / darwin. The path is passed explicitly so that what is
  // written is what getAutostart below asks about.
  app.setLoginItemSettings({
    openAtLogin: enabled,
    path: process.execPath,
    args: enabled ? AUTOSTART_ARGS : [],
  });
}

export function getAutostart(app: AutostartApp): boolean {
  if (process.platform === "linux") {
    return fs.existsSync(desktopFilePath());
  }
  // The same path and arguments setAutostart registered. Asking with
  // anything else is asking about a different login item.
  return app.getLoginItemSettings({ path: process.execPath, args: AUTOSTART_ARGS }).openAtLogin;
}
