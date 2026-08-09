# Real capture fixtures

`world-empty.sav` is a genuine Dragonwilds world save, produced by app
4019830 (Linux server build) creating a fresh world on 2026-08-09 and never
being played. 57 KB, no players, no personal data.

It is here so Phase 3's save reader can be built and tested without
reinstalling a 5 GB server. The format is the SPUD plugin's chunked
container — magic `SAVE`, then length-prefixed UE strings — **not** GVAS;
see the "Empirical findings" section of `docs/dragonwilds-recon.md` for the
header layout and why the earlier GVAS assumption was wrong.
