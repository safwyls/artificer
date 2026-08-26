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

// electron-builder's `-c.extraMetadata.version` rewrites package.json
// while it packages and restores it afterwards — but only if it gets
// that far. A run that fails partway (a missing signing toolchain, say)
// leaves the rewritten one behind: version bumped to the synthesized
// build number, and `scripts` and `devDependencies` gone entirely.
//
// That is quiet, survives a commit, and takes the build, test, package
// and smoke scripts with it. This is the tripwire.
test("package.json was not left rewritten by a packaging run", () => {
  const pkg = JSON.parse(read("package.json"));

  // Not compared against a fixed number: package.json's version is the
  // source of truth now and moves on purpose. What a rewritten one loses
  // is the rest of the file — scripts and devDependencies go entirely,
  // which is both the louder signal and the one that actually breaks
  // things.
  //
  // The version is still checked for *shape*: the updater compares
  // versions with semver, so one it cannot parse means an app that can
  // never find its own updates.
  assert.match(
    pkg.version,
    /^\d+\.\d+\.\d+$/,
    `package.json version "${pkg.version}" is not plain semver, so electron-updater cannot order it`,
  );
  for (const script of ["build", "test", "package", "smoke"]) {
    assert.ok(pkg.scripts?.[script], `package.json lost its "${script}" script`);
  }
  assert.ok(pkg.devDependencies?.electron, "package.json lost its devDependencies");
  // electron-updater has to be a *production* dependency or it is not
  // packaged, and the app cannot update itself.
  assert.ok(
    pkg.dependencies?.["electron-updater"],
    "electron-updater is not a production dependency, so it will not be packaged",
  );
});
