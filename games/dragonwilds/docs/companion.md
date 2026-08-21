# Companion sharing — the console-side character relay

The wildskeeper console's inbox for character records pushed from
players' machines. The app that once did the pushing, `wkcompanion`,
became the Artificer Companion and is now solely the save-sync client
(`docs/companion.md`); this tier remains for the old builds still
running, and this document is its contract.

## Why it still exists

The game keeps each character's record on the player's machine and
caches it server-side only while they are connected (recon, "Where
player state lives", corrected 2026-08-20). The console therefore reads
a full sheet from the world save for whoever is online and remembers it
afterwards (`dwapi/records.go`); what remains uncovered is a character
whose player has not been online since this console started observing,
or who plays elsewhere entirely. An old wkcompanion build relaying from
the player's own `SaveCharacters/` store closes that gap by the
player's choice — and nothing else does.

New installs don't get that app any more: current Artificer Companion
builds do not relay character data, the wildskeeper image no longer
bundles an exe, and the download link explains itself instead of 404ing.
An operator who wants to keep handing one out points `COMPANION_EXE`
(or the retired `WKCOMPANION_EXE`) at an old build.

## The trust model

- The **companion token** is minted per server by a console admin
  (Adventurers page → Companion sharing) and is the entire credential,
  the same shape as the public status token but write-scoped. Disabling
  sharing revokes it and forgets everything it delivered — including
  shared sheets already folded into the console's record memory, which
  tracks each sheet's provenance for exactly this reason; sheets the
  console read from the world save itself are its own and stay.
  Re-enabling mints a fresh token.
- The console-side inbox is **in-memory and bounded** (16 records per
  server — a world holds 6 players). Nothing a player shared outlives
  the console process unless they are still running the app that shares
  it; the old app's heartbeat re-fills the inbox within minutes of a
  console restart. Shared sheets are deliberately never written to the
  console's database — only sheets it read from the world save itself
  are persisted (`dwapi/records.go`), so a revoke cannot be undone by a
  later restart reloading a row.
- The pushed record is validated by the console's own `dwsave` parser
  (`dwsave.ParseCharacterRecord`) — one parser to be wrong in — and
  junk is refused with the reason.
- Records merge into the world payload by character guid
  (`dwsave.CanonicalGuid` folds the JSON record's base64url spelling
  and the world save's hex spelling); the world save's own transform
  position outranks the record's last-known location.

## Endpoints (console side)

| Route | Auth | Purpose |
|---|---|---|
| `GET /api/public/companion/{token}` | token | config check; answers the server's name |
| `GET /api/public/companion/{token}/download` | token | a bundled relay exe, when `COMPANION_EXE` names one; an explanation otherwise |
| `POST /api/public/companion/{token}/character` | token | push one raw character record (≤256 KB) |
| `GET /api/servers/{id}/companion` | admin | sharing state: enabled, token, shared count |
| `PUT /api/servers/{id}/companion` | admin | enable (mints token) / disable (revokes + drops) |

The public pair rides `api.Server.PublicGameRoutes`, the core seam for
this trust tier (unauthenticated, token-gated, mounted under
`/api/public` beside the status page). A console behind Cloudflare
Access needs a bypass for `/api/public/*` or a direct/LAN address —
the app-side writeup in `docs/companion.md` has the incident.
