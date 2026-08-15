# Favicons

`favicon-32.png`, `favicon-192.png`, `apple-touch-icon.png`, `icon-512.png`
and `icon-maskable-512.png` (the last two referenced by `manifest.json`) are
the Flametender flame sigil in its fully lit state — the fire burning, all
sixteen seats filled — rendered from the same geometry as
`src/components/flametender/FlameSigil.tsx`.

Regenerate with `node scripts/gen-icons.mjs` from `web/` (needs `sharp`,
which is not a checked-in dependency — install it transiently, e.g.
`npm i -D --no-save sharp`). The script fattens the pips and bakes the glow
in so the mark survives 32px, and the maskable variant adds a full-bleed
void background with the sigil inside the safe zone so launcher masks never
crop it.
