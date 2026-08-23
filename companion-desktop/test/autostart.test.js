// "Start the companion when I sign in" did not stick.
//
// On Windows, getLoginItemSettings does not report "is there a Run entry
// for this app". It compares against a path and an argument list, both
// defaulting to process.execPath and *no arguments*. setAutostart
// registered with ["--minimized"] and getAutostart asked with the
// default empty list, so the two were talking about different login
// items: the entry was written, and reading it back said it had not
// been. The checkbox came back unchecked the next time Settings opened.
//
// The fake below models that comparison rather than a boolean flag,
// because a fake that just remembers `openAtLogin` cannot fail this way
// and the test would have passed against the bug.
const { test } = require("node:test");
const assert = require("node:assert");

const { setAutostart, getAutostart } = require("../dist/autostart.js");

/** Electron's Windows behaviour, as far as this module can observe it. */
function fakeWindowsApp() {
  let entry = null; // { path, args }
  const same = (a, b) => JSON.stringify(a ?? []) === JSON.stringify(b ?? []);
  return {
    setLoginItemSettings({ openAtLogin, path, args }) {
      entry = openAtLogin ? { path, args: args ?? [] } : null;
    },
    getLoginItemSettings(options = {}) {
      const wantPath = options.path ?? process.execPath;
      const wantArgs = options.args ?? [];
      const openAtLogin =
        entry !== null && entry.path === wantPath && same(entry.args, wantArgs);
      return { openAtLogin };
    },
    getPath: () => "",
    // Test-only view of what was actually registered.
    entry: () => entry,
  };
}

const onWindows = process.platform === "win32";

test("what was switched on reads back as on", { skip: !onWindows }, () => {
  const app = fakeWindowsApp();
  setAutostart(true, app);
  assert.strictEqual(
    getAutostart(app),
    true,
    "the entry was written but reading it back denied it — set and get are asking about different login items",
  );
});

test("switching it off reads back as off", { skip: !onWindows }, () => {
  const app = fakeWindowsApp();
  setAutostart(true, app);
  setAutostart(false, app);
  assert.strictEqual(getAutostart(app), false);
  assert.strictEqual(app.entry(), null, "the login item was left behind");
});

test("the entry starts the companion minimized", { skip: !onWindows }, () => {
  // The whole point of the setting: login brings it up in the tray, not
  // as a window in your face.
  const app = fakeWindowsApp();
  setAutostart(true, app);
  assert.deepStrictEqual(app.entry().args, ["--minimized"]);
  assert.strictEqual(app.entry().path, process.execPath);
});

test("a login item registered with other arguments is not ours", { skip: !onWindows }, () => {
  // The failure this guards, stated directly: an entry whose arguments
  // differ is a different login item, and reporting it as ours would be
  // the same bug from the other side.
  const app = fakeWindowsApp();
  app.setLoginItemSettings({ openAtLogin: true, path: process.execPath, args: ["--something-else"] });
  assert.strictEqual(getAutostart(app), false);
});
