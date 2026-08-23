#!/usr/bin/env node
// Launches the built shell with COMPANION_SMOKE=1, which loads the window
// once, asserts the daemon's page actually rendered, and exits. Needs a
// display: uses xvfb-run automatically where one is absent.
const { spawnSync, spawnSync: run } = require("node:child_process");
const { statSync } = require("node:fs");
const path = require("node:path");

/**
 * Whether Chromium's setuid sandbox helper can actually be used.
 *
 * Linux only. Chromium sandboxes renderers with unprivileged user
 * namespaces where the kernel allows it and falls back to a setuid helper
 * binary where it does not — Ubuntu 24.04 restricts unprivileged userns
 * through AppArmor, so on a current runner it always falls back. The
 * helper npm unpacks is owned by the installing user and is not setuid,
 * which Electron treats as fatal ("configured incorrectly ... I'm
 * aborting now") rather than quietly running unsandboxed.
 *
 * `chown root` + `chmod 4755` on that file is the real fix and is what CI
 * does, so the check runs against the same sandbox a shipped build uses.
 * This is the fallback for everywhere else: someone on their own Linux
 * box should be able to run `npm run smoke` without sudo.
 */
function sandboxHelperUsable(electronPath, platform = process.platform, stat = statSync) {
  if (platform !== "linux") return true;
  try {
    const s = stat(path.join(path.dirname(electronPath), "chrome-sandbox"));
    // Root-owned *and* setuid; the helper refuses without both.
    return s.uid === 0 && (s.mode & 0o4000) !== 0;
  } catch {
    // A missing helper is not this script's to diagnose — let Electron
    // speak for itself rather than guessing at the reason.
    return true;
  }
}

/** The switches Electron needs to start here, which is normally none. */
function sandboxArgs(electronPath, platform = process.platform, stat = statSync) {
  return sandboxHelperUsable(electronPath, platform, stat) ? [] : ["--no-sandbox"];
}

function main() {
  const electron = require("electron");
  const appDir = path.join(__dirname, "..");
  // DISPLAY/WAYLAND_DISPLAY are how X11 and Wayland say there is a screen;
  // Windows and macOS have no such variable and always have one. Without
  // this the check refused to run on the two platforms the shell actually
  // ships to.
  const hasDisplay =
    process.platform === "win32" ||
    process.platform === "darwin" ||
    Boolean(process.env.DISPLAY || process.env.WAYLAND_DISPLAY);
  const haveXvfb = run("sh", ["-c", "command -v xvfb-run"], { encoding: "utf8" }).status === 0;

  const extra = sandboxArgs(electron);
  if (extra.length) {
    console.error(
      "smoke: chrome-sandbox is not setuid-root, so Chromium's OS sandbox is off for this run " +
        "(CI fixes the helper instead — see .github/workflows/ci.yml).",
    );
  }

  let cmd = electron;
  let args = [appDir, ...extra];
  if (!hasDisplay) {
    if (!haveXvfb) {
      console.error("smoke: no display and no xvfb-run; cannot render a window here");
      process.exit(2);
    }
    cmd = "xvfb-run";
    args = ["-a", electron, appDir, ...extra];
  }

  // ELECTRON_RUN_AS_NODE makes the electron binary behave as plain node, so
  // main.js would load with `app` undefined and die on the first electron
  // call. Some editors (VS Code's extension host among them) set it in the
  // environment they hand to terminals, so it can arrive without anyone
  // asking for it — strip it rather than inherit it.
  const env = { ...process.env, COMPANION_SMOKE: "1" };
  delete env.ELECTRON_RUN_AS_NODE;

  const res = spawnSync(cmd, args, { cwd: appDir, stdio: "inherit", env });
  process.exit(res.status === null ? 1 : res.status);
}

// Guarded so the helpers above can be unit-tested without launching a
// window: requiring this file must not spawn Electron.
if (require.main === module) main();

module.exports = { sandboxArgs, sandboxHelperUsable };
