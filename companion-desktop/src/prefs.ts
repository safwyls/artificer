// prefs.ts — the handful of settings that belong to the shell rather
// than to the companion engine.
//
// The daemon's config holds what syncing needs: the service, the token,
// the links, whether to launch a game on checkout. None of that is this.
// "Start minimized" is a fact about a window, on a machine that has one,
// and the daemon has no window — the same reason autostart is a shell
// concern (autostart.ts) rather than a field in config.json.
//
// A file of its own, in the app's userData, so an uninstall takes it and
// a corrupt one costs nothing: every read falls back to the default
// rather than failing, because none of this is worth refusing to start
// over.

import * as fs from "node:fs";
import * as path from "node:path";

export interface Prefs {
  /** Open into the tray instead of showing the window. */
  startMinimized: boolean;
}

const DEFAULTS: Prefs = { startMinimized: false };

/** Where the file lives. Takes the directory so tests need no Electron. */
export function prefsPath(userDataDir: string): string {
  return path.join(userDataDir, "shell-prefs.json");
}

export function readPrefs(userDataDir: string): Prefs {
  try {
    const raw = fs.readFileSync(prefsPath(userDataDir), "utf8");
    const parsed = JSON.parse(raw) as Partial<Prefs>;
    return { ...DEFAULTS, ...pick(parsed) };
  } catch {
    // Missing, unreadable, or not JSON. All the same answer: the
    // defaults. A preferences file is not a reason to fail to open.
    return { ...DEFAULTS };
  }
}

export function writePrefs(userDataDir: string, next: Partial<Prefs>): Prefs {
  const merged = { ...readPrefs(userDataDir), ...pick(next) };
  fs.mkdirSync(userDataDir, { recursive: true });
  fs.writeFileSync(prefsPath(userDataDir), JSON.stringify(merged, null, 2) + "\n");
  return merged;
}

/** Only the keys this file owns, and only if they are the right type —
 * a hand-edited file should not be able to put anything else in here. */
function pick(input: Partial<Prefs>): Partial<Prefs> {
  const out: Partial<Prefs> = {};
  if (typeof input?.startMinimized === "boolean") out.startMinimized = input.startMinimized;
  return out;
}

/**
 * Whether this launch should open into the tray.
 *
 * Two ways to say so, and either is enough. The stored preference is the
 * one a player sets in Settings. `--minimized` is what the login item
 * passes (autostart.ts's AUTOSTART_ARGS), so that logging in comes up
 * quietly whether or not the preference is set — which is the whole
 * point of starting with the machine.
 *
 * That argument was being passed to nothing until this existed: the
 * login entry carried `--minimized`, no code read it, and the window
 * opened in your face at every login.
 */
export function shouldStartMinimized(userDataDir: string, argv: readonly string[]): boolean {
  return argv.includes("--minimized") || readPrefs(userDataDir).startMinimized;
}
