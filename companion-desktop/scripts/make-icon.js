#!/usr/bin/env node
// Rasterises build/icon.png — the app's icon on the taskbar, in the tray,
// in the installer and in the window — from the vault mark.
//
// Run with `npm run icon`. It is a *generator* rather than a checked-in
// drawing so the icon cannot drift from the mark the app wears: the two
// path strings below are the ones in web/companion/src/components/
// VaultMark.tsx, and the colours are --ink and --gold from
// web/companion/src/index.css.
//
// It renders through Electron because Electron is already this project's
// only image-capable dependency, and it draws into a <canvas> rather than
// screenshotting a window so the rounded corners come out genuinely
// transparent instead of composited against something.
const { app, BrowserWindow } = require("electron");
const { writeFileSync } = require("node:fs");
const path = require("node:path");

const SIZE = 1024;
const OUT = path.join(__dirname, "..", "build", "icon.png");

// The vault mark: web/companion/src/components/VaultMark.tsx's diamond,
// which is the same shape the titlebar and the empty state draw.
//
// The mark used to be that diamond sitting on a wider kite, and this
// generator drew the pair. It does not survive being an icon: at 16, 32
// and 48px the composition stops reading as a vault and starts reading
// as a small person, the diamond a head and the kite a body. That is
// invisible in the 1024px master and obvious the moment it is rendered
// at the sizes an OS asks for -- which is the check to run before
// believing an icon works. The kite is gone from the app entirely now,
// so this file and the component draw one shape between them.
//
const INK = "#100d17";
const GOLD = "#c9a860";

const page = `<!doctype html><meta charset="utf-8"><canvas id="c" width="${SIZE}" height="${SIZE}"></canvas>
<script>
const S = ${SIZE};
const ctx = document.getElementById("c").getContext("2d");

// A rounded square, the shape every platform expects to be handed.
const r = S * 0.18;
ctx.beginPath();
ctx.moveTo(r, 0);
ctx.arcTo(S, 0, S, S, r);
ctx.arcTo(S, S, 0, S, r);
ctx.arcTo(0, S, 0, 0, r);
ctx.arcTo(0, 0, S, 0, r);
ctx.closePath();
ctx.fillStyle = ${JSON.stringify(INK)};
ctx.fill();

// The diamond. Proportions are fractions of the canvas so the stroke
// keeps its weight relative to the shape at any SIZE -- and VaultMark's
// default strokeWidth is derived from this same ratio, which is what
// keeps the drawn mark and the rasterised one the same drawing.
const c = S / 2, rad = S * 0.32;
ctx.beginPath();
ctx.moveTo(c, c - rad);
ctx.lineTo(c + rad, c);
ctx.lineTo(c, c + rad);
ctx.lineTo(c - rad, c);
ctx.closePath();
ctx.strokeStyle = ${JSON.stringify(GOLD)};
ctx.lineWidth = S * 0.085;
ctx.lineJoin = "round";
ctx.stroke();

window.__png = document.getElementById("c").toDataURL("image/png");
</script>`;

app.disableHardwareAcceleration();
app.whenReady().then(async () => {
  const win = new BrowserWindow({ width: SIZE, height: SIZE, show: false });
  await win.loadURL("data:text/html;charset=utf-8," + encodeURIComponent(page));
  const dataUrl = await win.webContents.executeJavaScript("window.__png");
  const b64 = dataUrl.replace(/^data:image\/png;base64,/, "");
  writeFileSync(OUT, Buffer.from(b64, "base64"));
  // eslint-disable-next-line no-console
  console.error(`wrote ${OUT} (${SIZE}x${SIZE})`);
  app.exit(0);
});
