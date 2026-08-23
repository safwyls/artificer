// The window smoke has to start Electron before it can assert anything,
// and on Linux that means getting past Chromium's sandbox.
//
// Chromium sandboxes renderers with unprivileged user namespaces where
// the kernel allows it and falls back to a setuid helper where it does
// not. Ubuntu 24.04 restricts unprivileged userns through AppArmor, so a
// current runner always falls back — and the helper npm unpacks is owned
// by the installing user and is not setuid, which Electron treats as
// fatal: "The SUID sandbox helper binary was found, but is not configured
// correctly ... I'm aborting now", exit 133, before a single frame.
//
// CI fixes the helper (chown root, chmod 4755) so the check runs against
// the same sandbox a shipped build uses. This is the fallback for a
// developer's own Linux box, where sudo is not this script's to ask for.
//
// Tested through injected stat results rather than a real file, because
// the case that matters cannot be reproduced on the machine most of this
// is written on.
const { test } = require("node:test");
const assert = require("node:assert");

const { sandboxArgs, sandboxHelperUsable } = require("../scripts/smoke.js");

const ELECTRON = "/app/node_modules/electron/dist/electron";
const stat = (uid, mode) => () => ({ uid, mode });

// 0o4755: setuid bit set, owned by root. Anything less and the helper
// refuses, so both halves have to be checked.
const ROOT_SETUID = stat(0, 0o104755);
const USER_OWNED = stat(1001, 0o100755);
const ROOT_NOT_SETUID = stat(0, 0o100755);

test("a correctly configured helper is left alone", () => {
  assert.strictEqual(sandboxHelperUsable(ELECTRON, "linux", ROOT_SETUID), true);
  assert.deepStrictEqual(sandboxArgs(ELECTRON, "linux", ROOT_SETUID), []);
});

test("an npm-unpacked helper is not usable, and the sandbox is turned off", () => {
  assert.strictEqual(sandboxHelperUsable(ELECTRON, "linux", USER_OWNED), false);
  assert.deepStrictEqual(sandboxArgs(ELECTRON, "linux", USER_OWNED), ["--no-sandbox"]);
});

test("root-owned but not setuid is still not usable", () => {
  // The half-fixed case: `chown root` without `chmod 4755`. Electron
  // aborts on this exactly as it does on the untouched file.
  assert.strictEqual(sandboxHelperUsable(ELECTRON, "linux", ROOT_NOT_SETUID), false);
});

test("the two platforms the shell ships to are never touched", () => {
  // Windows and macOS have no setuid helper and no such failure mode.
  // Passing --no-sandbox there would weaken a real build's sandbox to
  // work around a problem that platform does not have.
  for (const platform of ["win32", "darwin"]) {
    assert.strictEqual(sandboxHelperUsable(ELECTRON, platform, USER_OWNED), true);
    assert.deepStrictEqual(sandboxArgs(ELECTRON, platform, USER_OWNED), []);
  }
});

test("a helper that cannot be read is left for Electron to explain", () => {
  const missing = () => {
    throw Object.assign(new Error("ENOENT"), { code: "ENOENT" });
  };
  // Not this script's failure to diagnose: guessing here would replace
  // Electron's own message with a worse one.
  assert.strictEqual(sandboxHelperUsable(ELECTRON, "linux", missing), true);
});

test("requiring the script does not launch a window", () => {
  // The helpers are only testable because main() is behind a
  // require.main guard. Without it this file would spawn Electron on
  // import and hang the suite.
  assert.strictEqual(typeof sandboxArgs, "function");
  assert.strictEqual(typeof sandboxHelperUsable, "function");
});
