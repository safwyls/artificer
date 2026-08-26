// The shell's own settings, and one flag that was being passed to
// nothing.
//
// autostart.ts registers the login item with `--minimized`, and has since
// it was written. No code read it, so logging in opened a window in your
// face — the opposite of what "start the companion when I sign in" is
// for. `shouldStartMinimized` is what reads it now.
const { test } = require("node:test");
const assert = require("node:assert");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const { readPrefs, writePrefs, prefsPath, shouldStartMinimized } = require("../dist/prefs.js");

function tmpdir() {
  return fs.mkdtempSync(path.join(os.tmpdir(), "companion-prefs-"));
}

test("defaults when there is nothing saved", () => {
  const dir = tmpdir();
  assert.deepStrictEqual(readPrefs(dir), { startMinimized: false });
});

test("what was saved is what comes back", () => {
  const dir = tmpdir();
  writePrefs(dir, { startMinimized: true });
  assert.strictEqual(readPrefs(dir).startMinimized, true);
  writePrefs(dir, { startMinimized: false });
  assert.strictEqual(readPrefs(dir).startMinimized, false);
});

test("a corrupt file costs the defaults, not the launch", () => {
  const dir = tmpdir();
  fs.writeFileSync(prefsPath(dir), "{ this is not json");
  // Preferences are not a reason to fail to open.
  assert.deepStrictEqual(readPrefs(dir), { startMinimized: false });
});

test("a hand-edited file cannot put anything else in there", () => {
  const dir = tmpdir();
  fs.writeFileSync(
    prefsPath(dir),
    JSON.stringify({ startMinimized: "yes please", token: "secret", extra: 1 }),
  );
  const prefs = readPrefs(dir);
  // Wrong type is not a value, and nothing but the known key survives.
  assert.deepStrictEqual(prefs, { startMinimized: false });
});

test("--minimized is honoured, whatever the preference says", () => {
  const dir = tmpdir();
  // This is the login item's argument. It has to work on a machine where
  // the preference has never been touched, because logging in quietly is
  // the whole point of starting with the machine.
  assert.strictEqual(shouldStartMinimized(dir, ["electron", "--minimized"]), true);
  writePrefs(dir, { startMinimized: false });
  assert.strictEqual(shouldStartMinimized(dir, ["electron", "--minimized"]), true);
});

test("the preference is honoured without the argument", () => {
  const dir = tmpdir();
  assert.strictEqual(shouldStartMinimized(dir, ["electron"]), false);
  writePrefs(dir, { startMinimized: true });
  assert.strictEqual(shouldStartMinimized(dir, ["electron"]), true);
});

test("an ordinary launch with nothing set shows the window", () => {
  const dir = tmpdir();
  assert.strictEqual(shouldStartMinimized(dir, ["electron", "."]), false);
});
