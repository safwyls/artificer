# UE4SS under Wine — the Phase 4 unblocker

Proven working 2026-08-09 on this stack (see `games/dragonwilds/docs/recon.md`,
"Phase 4 unblocked"): the **Windows** dedicated server (same app id
4019830, `+@sSteamCmdForcePlatformType windows`) runs headless under plain
Wine on Linux, and the UE4SS nightly injects, signature-scans UE 5.6.1
successfully, and runs Lua mods that can reach live game objects. That is
the entire feasibility chain the dwbridge mod needs.

## Why the shim exists

UE4SS ships as a `dwmapi.dll` proxy — but the *server* binary never
imports dwmapi (no window manager on a server; checked in its PE import
table, so this is true on real Windows too). `version.dll` **is**
imported, so `version_proxy.c` is a minimal version.dll that loads
`ue4ss/UE4SS.dll` from `DllMain` and answers the real version API with
graceful failures (UE treats file-version info as optional metadata).
Probe-quality: a production dwbridge deployment should forward to the real
version.dll instead of stubbing it.

## Build

```sh
x86_64-w64-mingw32-gcc -shared -O1 -o version.dll version_proxy.c version.def
```

## Run

Lay UE4SS's release zip out next to the server exe
(`RSDragonwilds/Binaries/Win64/`: the `ue4ss/` folder; its `dwmapi.dll` is
unused here), drop `version.dll` beside the exe, then:

```sh
WINEDLLOVERRIDES="version=n,b" \
  wine RSDragonwildsServer-Win64-Shipping.exe -log -Port=7777
```

The override is required: without it Wine prefers its builtin version.dll
and the shim never loads. `wine-core` alone suffices (verified with Wine
11.0, Fedora); the `LogNNERuntimeORT: Failed to create D3D12 device`
errors are the ML runtime failing harmlessly on a headless box.

`probe.lua` is the feasibility probe (a UE4SS Lua mod: put it at
`ue4ss/Mods/DwbridgeProbe/Scripts/main.lua` and list `DwbridgeProbe : 1`
in `Mods/mods.txt`). It found the live `World` and `BP_GameMode_C`
instances from inside the running server — the handles a real dwbridge
mod would drive kick/ban/broadcast through.
