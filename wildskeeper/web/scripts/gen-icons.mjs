// Rasterizes the Wildskeeper rune sigil — fully awakened (all six segments
// lit, eye bright) — into the favicon/PWA set in web/public.
// Mirrors RuneSigil.tsx geometry, with strokes fattened and the glow baked
// in so it survives 32px.
import sharp from "sharp";

// Run from web/: `node scripts/gen-icons.mjs` (needs `npm i -D sharp` or
// a one-off `npm exec`-style install; sharp is not a checked-in dep).
import { fileURLToPath } from "node:url";
const OUT = fileURLToPath(new URL("../public", import.meta.url));

function sigil({ bg = null, pad = 0 } = {}) {
  const total = 6;
  const radius = 56;
  const C = 2 * Math.PI * radius;
  const seg = C / total;
  const arc = seg - 17; // wider than RuneSigil's 8.6: round caps + glow close small gaps
  const rings = Array.from({ length: total }, (_, i) => `
    <circle cx="66" cy="66" r="${radius}" fill="none" stroke="#52d8d0"
      stroke-width="9" stroke-linecap="round"
      stroke-dasharray="${arc} ${C - arc}" stroke-dashoffset="${-i * seg}"
      filter="url(#glow)"/>`).join("");
  // pad grows the canvas around the 132-box art (for the maskable icon).
  const size = 132 + pad * 2;
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${-pad} ${-pad} ${size} ${size}">
  <defs>
    <filter id="glow" x="-50%" y="-50%" width="200%" height="200%">
      <feDropShadow dx="0" dy="0" stdDeviation="2.6" flood-color="#52d8d0" flood-opacity="0.6"/>
    </filter>
  </defs>
  ${bg ? `<rect x="${-pad}" y="${-pad}" width="${size}" height="${size}" fill="${bg}"/>` : ""}
  <circle cx="66" cy="66" r="64" fill="#0d1218"/>
  <g transform="rotate(-90 66 66)">${rings}</g>
  <circle cx="66" cy="66" r="40" fill="#0d1218" stroke="#8a6f3a" stroke-width="2"/>
  <path d="M46 66q20-16 40 0q-20 16-40 0z" fill="#52d8d0" filter="url(#glow)"/>
  <ellipse cx="66" cy="66" rx="4" ry="9" fill="#0b0e12"/>
  <path d="M66 30v8M66 94v8M30 66h8M94 66h8M41 41l5 5M86 86l5 5M91 41l-5 5M46 86l-5 5"
    stroke="#c9a24b" stroke-width="2" fill="none" opacity="0.9"/>
</svg>`;
}

const tight = Buffer.from(sigil());
for (const [name, px] of [["favicon-32.png", 32], ["favicon-192.png", 192], ["apple-touch-icon.png", 180], ["icon-512.png", 512]]) {
  await sharp(tight, { density: 300 }).resize(px, px).png().toFile(`${OUT}/${name}`);
}
// Maskable: full-bleed ink background, sigil inside the safe zone (~28% pad).
const maskable = Buffer.from(sigil({ bg: "#0d1218", pad: 37 }));
await sharp(maskable, { density: 300 }).resize(512, 512).png().toFile(`${OUT}/icon-maskable-512.png`);
console.log("done");
