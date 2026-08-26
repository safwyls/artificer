// The preload's copy of the channel names must match the contract's.
//
// preload.ts cannot import them: it is a *sandboxed* preload, which
// Electron loads as a single file with a `require` that resolves only a
// handful of built-ins. `require("./ipc-contract")` threw "module not
// found" and killed the whole script, so `window.companion` was never
// defined in any build — and nothing failed loudly, because the shell
// injects the daemon's bearer at the network layer and the page reached
// the daemon regardless. What was actually lost was every native
// capability: the OS folder picker, "open the save folder", the
// autostart toggle. All three quietly fell back to the browser build's
// "this build cannot answer".
//
// So the names are duplicated on purpose, and this is what holds the two
// copies to each other.
const { test } = require("node:test");
const assert = require("node:assert");
const { readFileSync } = require("node:fs");
const path = require("node:path");

/** The compiled preload with its comments stripped — preload.ts explains
 * this very rule in prose, and prose that names a require is not one. */
function preloadCode() {
  return readFileSync(path.join(__dirname, "..", "dist", "preload.js"), "utf8")
    .replace(/\/\*[\s\S]*?\*\//g, "")
    .replace(/^\s*\/\/.*$/gm, "");
}

const { IPC } = require("../dist/ipc-contract.js");

test("the preload's channel names match ipc-contract's, exactly", () => {
  const src = preloadCode();
  const literal = src.match(/const IPC = \{([\s\S]*?)\};/);
  assert.ok(literal, "preload.js declares no IPC literal of its own");
  const copy = Object.fromEntries(
    [...literal[1].matchAll(/(\w+)\s*:\s*"([^"]+)"/g)].map((m) => [m[1], m[2]]),
  );
  assert.deepStrictEqual(copy, { ...IPC }, "preload.js and ipc-contract.ts disagree");
});

// The same rule for the one channel that goes the other way: main pushes
// update status to the page, and the preload has its own copy of that
// channel name for the same reason it has its own copy of the verbs.
// A mismatch here is a page that simply never hears about an update.
test("the preload's push channel matches the contract's", () => {
  const contract = readFileSync(path.join(__dirname, "..", "dist", "ipc-contract.js"), "utf8").match(/channel:\s*"([^"]+)"/);
  assert.ok(contract, "ipc-contract.js declares no push channel");

  const copy = preloadCode().match(/UPDATE_STATUS_CHANNEL\s*=\s*"([^"]+)"/);
  assert.ok(copy, "preload.js declares no copy of the push channel");

  assert.strictEqual(copy[1], contract[1]);
});

test("the preload requires nothing a sandbox cannot give it", () => {
  const src = preloadCode();
  // Electron's sandboxed loader resolves `electron` and a few built-ins.
  // A relative require is the failure this guards: it does not throw at
  // build time, only at load time, and only where nobody is watching.
  const requires = [...src.matchAll(/require\("([^"]+)"\)/g)].map((m) => m[1]);
  assert.deepStrictEqual(
    requires.filter((r) => r.startsWith(".")),
    [],
    `preload.js requires sibling modules a sandboxed preload cannot load: ${requires}`,
  );
});
