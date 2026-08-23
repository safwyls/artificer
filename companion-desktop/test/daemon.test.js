// daemon.test.js — headless verification of the spawn+handshake+lifecycle
// module against a REAL `go run ./cmd/companiond` child process. No
// display, no electron binary required: daemon.ts deliberately has no
// `electron` import so this can run in plain Node (see src/daemon.ts's
// header comment).
//
// Run with: npm test (builds dist/ first, then `node --test test/`).
//
// Covers the three things companion-cutover.md Phase 3's verification
// step calls out explicitly:
//   1. address line is parsed correctly from real stdout
//   2. /healthz gate actually gates (fails before token, or wrong token,
//      succeeds with the right one)
//   3. no orphaned companiond process survives after stdin close
//      (the "most common defect in this architecture")

const test = require("node:test");
const assert = require("node:assert/strict");
const path = require("node:path");

const {
  spawnDaemon,
  waitForHealthy,
  killDaemon,
  resolveDaemonCommandForApp,
  resolvePackagedDaemonBin,
  companiondBinaryName,
} = require("../dist/daemon.js");

const repoRoot = path.resolve(__dirname, "..", "..");

test("resolvePackagedDaemonBin joins resourcesPath and the platform binary name", () => {
  assert.equal(
    resolvePackagedDaemonBin({ platform: "linux", resourcesPath: "/opt/app/resources" }),
    path.join("/opt/app/resources", "companiond")
  );
  assert.equal(
    resolvePackagedDaemonBin({ platform: "win32", resourcesPath: "C:\\Program Files\\App\\resources" }),
    path.join("C:\\Program Files\\App\\resources", "companiond.exe")
  );
  assert.equal(
    resolvePackagedDaemonBin({ platform: "darwin", resourcesPath: "/Applications/App.app/Contents/Resources" }),
    path.join("/Applications/App.app/Contents/Resources", "companiond")
  );
});

test("companiondBinaryName picks .exe only on win32", () => {
  assert.equal(companiondBinaryName("win32"), "companiond.exe");
  assert.equal(companiondBinaryName("linux"), "companiond");
  assert.equal(companiondBinaryName("darwin"), "companiond");
});

test("resolveDaemonCommandForApp: packaged uses the bundled resourcesPath binary", () => {
  const result = resolveDaemonCommandForApp({
    isPackaged: true,
    platform: "linux",
    resourcesPath: "/opt/reliquary-companion/resources",
  });
  assert.equal(result.cmd, path.join("/opt/reliquary-companion/resources", "companiond"));
  assert.deepEqual(result.args, []);
  assert.equal(result.cwd, "/opt/reliquary-companion/resources");
});

test("resolveDaemonCommandForApp: packaged on Windows uses companiond.exe", () => {
  const result = resolveDaemonCommandForApp({
    isPackaged: true,
    platform: "win32",
    resourcesPath: "C:\\Users\\test\\AppData\\Local\\Programs\\reliquary-companion\\resources",
  });
  assert.equal(
    result.cmd,
    path.join(
      "C:\\Users\\test\\AppData\\Local\\Programs\\reliquary-companion\\resources",
      "companiond.exe"
    )
  );
});

test("resolveDaemonCommandForApp: dev (not packaged) falls back to go run", () => {
  const result = resolveDaemonCommandForApp(
    { isPackaged: false, platform: "linux", resourcesPath: "/opt/whatever/resources" },
    { repoRoot }
  );
  assert.equal(result.cmd, "go");
  assert.deepEqual(result.args, ["run", "./cmd/companiond"]);
  assert.equal(result.cwd, repoRoot);
});

test("resolveDaemonCommandForApp: an explicit companiondBin override wins even when packaged", () => {
  const result = resolveDaemonCommandForApp(
    { isPackaged: true, platform: "linux", resourcesPath: "/opt/whatever/resources" },
    { companiondBin: "/custom/path/companiond", repoRoot }
  );
  assert.equal(result.cmd, "/custom/path/companiond");
  assert.deepEqual(result.args, []);
  assert.equal(result.cwd, repoRoot);
});

function isProcessAlive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch {
    return false;
  }
}

test("spawnDaemon parses the handshake address and gates /healthz on the token", async (t) => {
  const handle = await spawnDaemon({
    repoRoot,
    handshakeTimeoutMs: 30_000,
    onLog: (line) => t.diagnostic(`companiond stderr: ${line}`),
  });

  t.diagnostic(`companiond pid=${handle.child.pid} baseUrl=${handle.baseUrl}`);

  assert.match(handle.baseUrl, /^http:\/\/127\.0\.0\.1:\d+$/, "baseUrl should be a parsed loopback host:port");
  assert.equal(handle.token.length, 64, "token should be a 32-byte hex string");

  await waitForHealthy(handle.baseUrl, { timeoutMs: 20_000 });

  // /healthz needs no token.
  const healthNoAuth = await fetch(`${handle.baseUrl}/healthz`);
  assert.equal(healthNoAuth.status, 200, "/healthz should be reachable without a token");

  // Every other route requires the bearer token.
  const stateNoAuth = await fetch(`${handle.baseUrl}/api/state`);
  assert.equal(stateNoAuth.status, 401, "/api/state without a token should be rejected");

  const stateWrongToken = await fetch(`${handle.baseUrl}/api/state`, {
    headers: { Authorization: "Bearer wrong-token" },
  });
  assert.equal(stateWrongToken.status, 401, "/api/state with the wrong token should be rejected");

  const stateAuthed = await fetch(`${handle.baseUrl}/api/state`, {
    headers: { Authorization: `Bearer ${handle.token}` },
  });
  assert.equal(stateAuthed.status, 200, "/api/state with the right token should succeed");

  const pid = handle.child.pid;
  assert.ok(isProcessAlive(pid), "companiond should be alive before shutdown");

  await killDaemon(handle.child);

  // Give the OS a brief moment to reap; killDaemon already resolves only
  // after the 'exit' event, so this should be immediate.
  await new Promise((r) => setTimeout(r, 200));
  assert.equal(isProcessAlive(pid), false, "companiond must not survive after stdin close (no orphan)");
});

test("killDaemon resolves promptly even if the child ignores stdin close (escalation path)", async (t) => {
  // Spawn a small stand-in process that never exits on stdin EOF, to
  // exercise the SIGTERM/SIGKILL escalation branch of killDaemon without
  // depending on companiond ever misbehaving.
  const { spawn } = require("node:child_process");
  const child = spawn(
    process.execPath,
    ["-e", "process.on('SIGTERM', () => {}); setInterval(() => {}, 1000);"],
    { stdio: ["pipe", "pipe", "pipe"] }
  );

  const start = Date.now();
  await killDaemon(child, { termGraceMs: 300, killGraceMs: 300 });
  const elapsed = Date.now() - start;

  t.diagnostic(`escalation took ${elapsed}ms`);
  assert.ok(elapsed < 2000, "killDaemon should escalate to SIGTERM/SIGKILL within its grace windows, not hang");
  assert.equal(isProcessAlive(child.pid), false, "the stubborn child must be gone after escalation");
});
