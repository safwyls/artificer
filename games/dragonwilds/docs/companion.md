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

Build: `GOOS=windows GOARCH=amd64 go build ./cmd/wkcompanion` (players
run Windows; the binary itself is portable).

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
