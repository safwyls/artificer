# dwbridge — the Dragonwilds command channel

RuneScape: Dragonwilds ships no RCON, REST, or query protocol, so Wildskeeper
derives everything it shows from logs and files. The one thing it cannot
derive is *action* — save-now, kick, ban, broadcast. dwbridge is that missing
channel: a [UE4SS](https://github.com/UE4SS-RE/RE-UE4SS) Lua mod that calls the
game's own functions on request.

Proven end to end 2026-08-09: `POST /api/servers/{id}/save` in the console
wrote the world on a headless server with no player connected —
dwcon → palagent → this mod → `PersistenceSubsystem:SaveGame`.

## How it fits together

```
dwcon  ──HTTP──▶  palagent  ──files──▶  dwbridge (this mod)  ──▶  game
       /save      /v1/bridge/command    DWBRIDGE_DIR               UFunction
```

The agent and the mod share a directory (`<install>/dwbridge/`, the mod sees
it as `DWBRIDGE_DIR`). The transport is files because that is the one thing
they reliably share across the Wine boundary — no sockets, no ports. It is
single-flight (the agent serializes commands), so the files have fixed names:

| file            | writer | meaning                                             |
|-----------------|--------|-----------------------------------------------------|
| `heartbeat.json`| mod    | every ~2 s: liveness + the commands this build has  |
| `request.json`  | agent  | `{"id","command","args"}`                            |
| `response.json` | mod    | `{"id","ok","error","data"}`, echoing the id         |

The agent reports the heartbeat (fresh within 8 s) as `health.bridge`, and
only routes commands the heartbeat lists — so shipping a new verb is adding a
handler here, never a version handshake.

## Commands

- `ping` — liveness with a real object touch (finds the live GameMode).
- `save` — `PersistenceSubsystem:SaveGame`, the same call the autosave makes.
  **Verified headless.**

`kick`/`ban`/`unban`/`broadcast` are mapped in the recon doc's "Command
surface" section (`Server_RequestAdminAction` + the `EAdminAction` enum, the
chat component's `Server_SendChatMessage`) but not implemented here yet: they
take struct parameters and, for kick, a connected player's controller, so they
need a live client to build against safely. The console reports them as an
honest capability gap until the mod lists them.

## Install

1. Stand up the Windows server build under Wine with UE4SS — see
   `../ue4ss-wine-shim/` (the server imports no dwmapi, so UE4SS loads via a
   `version.dll` shim).
2. Copy this folder to
   `<server>/RSDragonwilds/Binaries/Win64/ue4ss/Mods/dwbridge/` and add
   `dwbridge : 1` to `ue4ss/Mods/mods.txt`.
3. Launch with `DWBRIDGE_DIR` set to the shared directory. Under Wine that is
   the `Z:`-mapped install path, e.g.
   `DWBRIDGE_DIR='Z:\...\dwbridge'`, pointing at the same `<install>/dwbridge`
   the agent uses.

## The one Wine gotcha

Windows `rename` does not overwrite an existing file (it fails and strands the
`.tmp`), so the mod removes the destination before renaming. Getting this
wrong freezes `heartbeat.json` at its first value and the bridge reads as
permanently stale — see `writeFileAtomic` in `Scripts/main.lua`.
