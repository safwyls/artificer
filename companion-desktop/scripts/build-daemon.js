#!/usr/bin/env node
// build-daemon.js — builds cmd/companiond for the CURRENT host platform
// into companion-desktop/resources/, the staging dir electron-builder's
// `extraResources` (see ../electron-builder.yml) carries into the
// packaged app. This is a dev-machine convenience for local `npm run
// package` runs; Phase 6's CI matrix cross-compiles per target platform
// instead (companion-cutover.md, Phase 5 item 3).
//
// Staging dir contract (see README.md "Packaging"): a binary landing at
// resources/companiond (or resources/companiond.exe on Windows) is all
// electron-builder or a CI step needs to produce — this script is one
// way to produce it, not the only one.

const { spawnSync } = require("node:child_process");
const path = require("node:path");
const fs = require("node:fs");

const repoRoot = path.resolve(__dirname, "..", "..");
const resourcesDir = path.resolve(__dirname, "..", "resources");
const binName = process.platform === "win32" ? "companiond.exe" : "companiond";
const outPath = path.join(resourcesDir, binName);

fs.mkdirSync(resourcesDir, { recursive: true });

console.log(`Building companiond -> ${outPath} (GOOS=${process.platform === "win32" ? "windows" : process.platform})`);

const result = spawnSync("go", ["build", "-o", outPath, "./cmd/companiond"], {
  cwd: repoRoot,
  stdio: "inherit",
  env: process.env,
});

if (result.error) {
  console.error("go build failed to start:", result.error);
  process.exit(1);
}
if (result.status !== 0) {
  console.error(`go build exited with status ${result.status}`);
  process.exit(result.status ?? 1);
}

console.log("companiond built successfully.");
