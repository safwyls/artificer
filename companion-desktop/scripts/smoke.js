#!/usr/bin/env node
// Launches the built shell with COMPANION_SMOKE=1, which loads the window
// once, asserts the daemon's page actually rendered, and exits. Needs a
// display: uses xvfb-run automatically where one is absent.
const { spawnSync, spawnSync: run } = require("node:child_process");
const path = require("node:path");

const electron = require("electron");
const appDir = path.join(__dirname, "..");
const hasDisplay = Boolean(process.env.DISPLAY || process.env.WAYLAND_DISPLAY);
const haveXvfb = run("sh", ["-c", "command -v xvfb-run"], { encoding: "utf8" }).status === 0;

let cmd = electron;
let args = [appDir];
if (!hasDisplay) {
  if (!haveXvfb) {
    console.error("smoke: no display and no xvfb-run; cannot render a window here");
    process.exit(2);
  }
  cmd = "xvfb-run";
  args = ["-a", electron, appDir];
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
