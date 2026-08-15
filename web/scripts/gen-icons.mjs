// Rasterizes the Flametender flame sigil — the fire lit, every seat
// filled — into the favicon/PWA set in web/public.
// Mirrors FlameSigil.tsx geometry, with pips fattened and the glow baked
// in so it survives 32px.
import sharp from "sharp";

// Run from web/: `node scripts/gen-icons.mjs` (needs `npm i -D sharp` or
// a one-off `npm exec`-style install; sharp is not a checked-in dep).
import { fileURLToPath } from "node:url";
const OUT = fileURLToPath(new URL("../public", import.meta.url));

function sigil({ bg = null, pad = 0 } = {}) {
  const total = 16;
  const radius = 57;
  const pips = Array.from({ length: total }, (_, i) => {
    const a = (i / total) * 2 * Math.PI - Math.PI / 2;
    const x = 66 + radius * Math.cos(a);
    const y = 66 + radius * Math.sin(a);
    return `<circle cx="${x.toFixed(2)}" cy="${y.toFixed(2)}" r="4.4" fill="#7fc3f0" filter="url(#glow)"/>`;
  }).join("");
  // pad grows the canvas around the 132-box art (for the maskable icon).
  const size = 132 + pad * 2;
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${-pad} ${-pad} ${size} ${size}">
  <defs>
    <filter id="glow" x="-50%" y="-50%" width="200%" height="200%">
      <feDropShadow dx="0" dy="0" stdDeviation="2.4" flood-color="#7fc3f0" flood-opacity="0.55"/>
    </filter>
  </defs>
  ${bg ? `<rect x="${-pad}" y="${-pad}" width="${size}" height="${size}" fill="${bg}"/>` : ""}
  <circle cx="66" cy="66" r="64" fill="#101512"/>
  ${pips}
  <circle cx="66" cy="66" r="42" fill="#131a15" stroke="#98917c" stroke-width="2.4"/>
  <path d="M50 88h32" stroke="#2d3b32" stroke-width="5" stroke-linecap="round"/>
  <path d="M66 40c3.5 10 13 14 13 26a13 13 0 0 1-26 0c0-7 3.5-10 5.5-14 2 3 3.5 5.5 5.5 6.5-1-6.5 0-12 2-18.5z"
    fill="#7fc3f0" filter="url(#glow)"/>
  <path d="M66 60c2 4.5 6.5 6.5 6.5 12a6.5 6.5 0 0 1-13 0c0-5.5 4.5-7.5 6.5-12z" fill="#d3ecff"/>
</svg>`;
}

const tight = Buffer.from(sigil());
for (const [name, px] of [["favicon-32.png", 32], ["favicon-192.png", 192], ["apple-touch-icon.png", 180], ["icon-512.png", 512]]) {
  await sharp(tight, { density: 300 }).resize(px, px).png().toFile(`${OUT}/${name}`);
}
// Maskable: full-bleed void background, sigil inside the safe zone (~28% pad).
const maskable = Buffer.from(sigil({ bg: "#101512", pad: 37 }));
await sharp(maskable, { density: 300 }).resize(512, 512).png().toFile(`${OUT}/icon-maskable-512.png`);
console.log("done");
