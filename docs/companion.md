# The Artificer Companion — the save-sync client

One Go binary (`cmd/companion`, shipped as `artificer-companion.exe`)
that runs on a player's own machine and moves shared world saves
between it and the save-sync service, reliquary
(docs/save-sync-architecture.md). No installer, no service: a tray
icon, a local page on `127.0.0.1:8377`, a config file under the user's
config directory.

Born `wkcompanion`, the Dragonwilds character relay. That job retired
when the recon corrected itself — the world save carries a connected
player's sheet and wildskeeper reads and remembers those directly — so
the app is now solely the custody client, deliberately game-blind: one
binary for every game Artificer syncs. (The console side of the old
relay still accepts pushes from old wkcompanion builds;
`games/dragonwilds/docs/companion.md` is that contract.)

## What it does

1. **Finds installed games.** Steam's own metadata is the ground truth
   (`libraryfolders.vdf` → `appmanifest_*.acf`); save locations are
   heuristics — Unreal's `Saved/SaveGames` convention plus a small
   catalog — and every candidate is confirmed by the player, never
   followed blindly. `STEAM_ROOT` overrides odd installs.
2. **Links games to worlds.** Linking tells the service which game a
   world belongs to and where its save lives here (`game_title`,
   `save_hint`, and a free-form JSON blob with the Steam app id); it
   can create the world and seed it with the folder's current save in
   the same step. Any number of worlds can be linked.
3. **Moves the saves.** Checkout installs a world's head into its
   folder (tmp-extract-and-swap, one `.pre-checkout` copy kept);
   check-in packages the folder and returns the hold; mid-session
   checkpoints push automatically as crash insurance; a queued claim's
   handoff is fetched without the player doing anything. Packaging
   waits out a settle window on the folder's mtimes — the app is
   game-blind, so the settle window is the torn-save guard, and the
   service verifies every upload again.

The credential is the player's personal sync token from the service's
page. Nothing leaves the machine until a service URL and token are set.

## Getting it

- **GitHub releases**: every push to main rebuilds the exe onto the
  rolling `companion-latest` release
  (`.github/workflows/release-companion.yml`); version tags attach it
  to their own releases.
- **The service hands it out**: the reliquary image bundles the exe and
  serves it behind each player's token
  (`/api/public/sync/{token}/companion/download`).
- **By hand**: `GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" ./cmd/companion`
  (the flag suppresses the console window; without it the window
  flashes once and closes — see `console_windows.go`).

To start it with Windows, drop a shortcut in `shell:startup` —
deliberately manual; an app that installs itself into startup is not
this repo's style.

## Config, and the migration chain

`%AppData%`-equivalent (`os.UserConfigDir()`) `artificer-companion/config.json`,
mode 0600: the service URL, the token, and the linked worlds with their
hold state. Older configs are read as fallbacks: the first Artificer
Companion cut's nested sync block maps forward, and a wkcompanion-era
file maps to its sync side only — a relay-only config maps to empty,
because its credential has nothing to authenticate any more. Logs go to
`companion.log` beside the config (a windowed build has no console).

## Reaching the service through an auth layer

The token-in-path endpoints are unauthenticated-with-token by design,
so anything that forces its own login in front of the service breaks
them: Cloudflare Access answers with its login page — HTTP 200, HTML.
Hit for real on 2026-08-19; the companion never counts such a 200 as
delivered and names the interceptor instead. Fixes, either of: a
bypass/service-auth policy for `/api/public/*`, or a direct/LAN
address.
