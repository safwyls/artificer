# wkcompanion — the player-side character relay

The game stores each character's record — name, skills, inventory,
vitals — on the player's own machine, in the same `SaveCharacters/`
store the client uses for solo play (recon, "Where player state lives",
2026-08-19). A dedicated server never holds it, so wildskeeper's
save-derived character view tops out at guid + position. `wkcompanion`
runs where the data actually is and closes that gap **by the player's
own choice**.

## What it is

One Go binary (`cmd/wkcompanion`), no installer, no service:

1. It watches the local `SaveCharacters/` directory
   (`%LOCALAPPDATA%\RSDragonwilds\Saved\SaveCharacters` on Windows,
   auto-detected; overridable for odd installs).
2. It serves a character-sheet page on `127.0.0.1:8377` — skills with
   levels, inventory with item names, vitals. **Local-only by default:
   with no console configured, nothing leaves the machine.**
3. Optionally, it relays the raw character record to a wildskeeper
   console: paste the console URL and the *companion token* a console
   admin hands out, and the record is pushed on every change plus a
   10-minute heartbeat. The console's Adventurers page then shows the
   full sheet, marked "shared" with its freshness.

## How it runs on a player's machine

On Windows it **lives in the system tray**: no window while it works,
just an icon whose menu opens the character sheet, pushes to the console
on demand, shows the sharing state at a glance ("Sharing with grimwood ·
pushed 16:02" / "Local-only"), and quits. The character sheet itself is
the local browser page — the tray is the handle, not the UI. Launching
the exe a second time doesn't start a second copy: it notices the
running instance and opens its page instead. Because a windowed build
has no console, logs go to `companion.log` beside the config file.

## Getting the app to players

**The console hands it out itself.** The wildskeeper image build
cross-compiles `wkcompanion.exe` (deploy/wildskeeper/Dockerfile) and
ships it beside the console binary; with sharing enabled, the
Adventurers panel shows a **download link** —
`/api/public/companion/<token>/download`, token-gated like the rest of
the tier — that an admin copies and gives players along with the
console address and token. A deployment without the bundle (a source
checkout, an older image) answers the link with the build command
instead of a broken download; `WKCOMPANION_EXE` overrides the bundled
path when needed.

Building by hand instead:

```sh
GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" ./cmd/wkcompanion
```

(`-H windowsgui` is what suppresses the console window entirely. A plain
`go build` produces a console-subsystem exe instead — double-clicked, it
detects that the console was created for it alone and closes it right
after startup, so the flag is polish, not a requirement: without it the
window flashes briefly instead of never appearing. Run from a terminal,
the console is the developer's and stays.) To start it with Windows,
drop a shortcut to the exe in `shell:startup` — deliberately manual for
now, an app that auto-installs itself into startup is not this repo's
style.

## Reaching the console through an auth layer

The push endpoints are unauthenticated-with-token by design, so anything
that forces its own login in front of the console breaks them: a console
behind **Cloudflare Access** (or any tunnel/proxy auth) answers the
companion with its login page — HTTP 200, HTML — instead of the API.
Hit for real on 2026-08-19; the companion now names this instead of
saying "unexpected answer", and never counts such a 200 as a delivered
push. Fixes, either of: add a bypass/service-auth policy for
`/api/public/*` (the same consideration the public status page has), or
give players a direct/LAN address for the console.

## The trust model

- The **companion token** is minted per server by a console admin
  (Adventurers page → Companion sharing) and is the entire credential,
  the same shape as the public status token but write-scoped. Disabling
  sharing revokes it and drops everything it delivered; re-enabling
  mints a fresh token.
- The console-side inbox is **in-memory and bounded** (16 records per
  server — a world holds 6 players). Nothing a player shared outlives
  the console process unless they are still running the app that shares
  it; the heartbeat re-fills the inbox within minutes of a console
  restart.
- The pushed record is validated by the same `dwsave` parser the
  companion itself uses (`dwsave.ParseCharacterRecord`) — one parser to
  be wrong in — and junk is refused with the reason.
- Records merge into the world payload by character guid
  (`dwsave.CanonicalGuid` folds the JSON record's base64url spelling and
  the world save's hex spelling); the world save's own transform
  position outranks the record's last-known location.

## Endpoints (console side)

| Route | Auth | Purpose |
|---|---|---|
| `GET /api/public/companion/{token}` | token | config check; answers the server's name |
| `POST /api/public/companion/{token}/character` | token | push one raw character record (≤256 KB) |
| `GET /api/servers/{id}/companion` | admin | sharing state: enabled, token, shared count |
| `PUT /api/servers/{id}/companion` | admin | enable (mints token) / disable (revokes + drops) |

The public pair rides `api.Server.PublicGameRoutes`, the core seam added
for exactly this trust tier (unauthenticated, token-gated, mounted under
`/api/public` beside the status page).

## Vendored names

The companion page resolves item and skill ids through
`cmd/wkcompanion/ui/data/{itemNames,skillNames}.json` — byte-for-byte
mirrors of the wildskeeper frontend's maps. The refresh chore in
`games/dragonwilds/docs/vendored-game-data.md` covers both copies:
regenerate them together.
