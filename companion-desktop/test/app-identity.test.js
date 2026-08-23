// The shell's Windows identity must match the one it is packaged under.
//
// Windows decides which taskbar button a window belongs to, and whose
// icon and name a toast carries, from the AppUserModelID. If main.ts
// sets one string and electron-builder packages under another, the
// installed app and the running app are two different applications as
// far as the OS is concerned: pinning breaks, notifications wear the
// wrong name, and the taskbar shows a second button for an app that is
// already open. None of that fails a build, and none of it is visible
// until someone installs it.
const { test } = require("node:test");
const assert = require("node:assert");
const { readFileSync } = require("node:fs");
const path = require("node:path");

const read = (...p) => readFileSync(path.join(__dirname, "..", ...p), "utf8");

test("main.ts's AppUserModelID is electron-builder's appId", () => {
  const built = read("dist", "main.js").match(/APP_ID\s*=\s*"([^"]+)"/);
  assert.ok(built, "dist/main.js declares no APP_ID");

  const packaged = read("electron-builder.yml").match(/^appId:\s*(\S+)\s*$/m);
  assert.ok(packaged, "electron-builder.yml declares no appId");

  assert.strictEqual(built[1], packaged[1]);
});

test("the window title is the packaged productName", () => {
  const built = read("dist", "main.js").match(/APP_NAME\s*=\s*"([^"]+)"/);
  assert.ok(built, "dist/main.js declares no APP_NAME");

  const packaged = read("electron-builder.yml").match(/^productName:\s*(.+?)\s*$/m);
  assert.ok(packaged, "electron-builder.yml declares no productName");

  assert.strictEqual(built[1], packaged[1]);
});
