// Palcon's design system, as data.
//
// The Palworld console — and the outlier of the three. It is the only **light**
// theme in the monorepo, the only one whose radius is generous
// (<code>0.85rem</code>), and the only one whose accent colours have to survive
// next to game artwork that brings its own. Warm paper, warm ink, brand
// red-orange, brand amber.
//
// Structure comes from `../lib/console.mjs`: palcon, wildskeeper and
// flametender are the same application wearing three faces. What is here is
// the theme plus the one thing the other two consoles do not have — a data
// surface (pals, passives, players, a map) whose colours are sampled from the
// game rather than chosen, and which therefore has rules of its own.
//
// Design source: `mocks/*.html` in the archived palcon workspace;
// `scripts/checkdesign.sh` fails CI if the semantic mapping in
// `web/palcon/src/index.css` or the literal palette in
// `web/palcon/tailwind.config.js` stops matching this file.
import { chromeCSS } from "../lib/chrome.mjs";
import {
  cbtn,
  consoleChrome,
  consoleKit,
  hslSwatches,
  literalSwatches,
  panel,
  stat,
  typeRow,
} from "../lib/console.mjs";

const colors = [
  { name: "background", value: "36 50% 92%", hex: "#f5ede1", use: "paper — the warm page ground" },
  { name: "foreground", value: "22 15% 15%", hex: "#2b2420", use: "ink — body text" },
  { name: "card", value: "33 40% 96%", hex: "#faf5ee", use: "cards, a shade above the paper" },
  { name: "card-foreground", value: "22 15% 15%", hex: "#2b2420", use: "text on a card" },
  { name: "popover", value: "33 40% 96%", hex: "#faf5ee", use: "dialogs, menus, tooltips" },
  { name: "popover-foreground", value: "22 15% 15%", hex: "#2b2420", use: "text in an overlay" },
  { name: "primary", value: "13 82% 51%", hex: "#e8491d", use: "brand red-orange — the default button" },
  { name: "primary-foreground", value: "36 50% 96%", hex: "#fbf6ef", use: "text on the brand red" },
  { name: "secondary", value: "30 20% 88%", hex: "#e6ddd3", use: "the quiet button, and the active pill" },
  { name: "secondary-foreground", value: "22 15% 15%", hex: "#2b2420", use: "text on the quiet button" },
  { name: "muted", value: "33 25% 90%", hex: "#e9e2d9", use: "inset strips" },
  { name: "muted-foreground", value: "25 12% 42%", hex: "#78685e", use: "secondary text and labels" },
  { name: "accent", value: "36 88% 59%", hex: "#f2a93b", use: "brand amber — panel titles, hover, highlights" },
  { name: "accent-foreground", value: "22 15% 15%", hex: "#2b2420", use: "ink, for text on amber" },
  { name: "destructive", value: "3 71% 41%", hex: "#b32720", use: "a deeper, more alarm-toned red than the brand — kick, ban, delete, shutdown" },
  { name: "destructive-foreground", value: "36 50% 96%", hex: "#fbf6ef", use: "text on the alarm red" },
  { name: "border", value: "25 20% 84%", hex: "#dcd2c7", use: "every border" },
  { name: "input", value: "25 20% 84%", hex: "#dcd2c7", use: "field borders" },
  { name: "ring", value: "13 82% 51%", hex: "#e8491d", use: "focus — the brand red, since this theme has no reserved live colour" },
];

/** The literal palette, for the spots that need an exact hue: the dark rail,
 *  per-stat tinting, rarity, and the passive-tier chips whose colours are
 *  pixel-sampled from the game's own tier icons. */
const literals = {
  prefix: "pc",
  entries: [
    { name: "brand-red", hex: "#E8491D", use: "brand red-orange — the primary" },
    { name: "brand-amber", hex: "#F2A93B", use: "brand amber — the accent" },
    { name: "paper", hex: "#F5EDE1", use: "the page ground" },
    { name: "ink", hex: "#2B2420", use: "text, and the dark server rail" },
    { name: "ink-light", hex: "#3D342D", use: "a raised strip on the rail" },
    { name: "ink-soft", hex: "#544A40", use: "a dimmed dot on the rail" },
    { name: "ink-muted", hex: "#5F5850", use: "an off switch" },
    { name: "pal-green", hex: "#4A9D7C", use: "reachable, healthy" },
    { name: "pal-blue", hex: "#5B9BD5", use: "a second server colour" },
    { name: "legendary", hex: "#8B3A9E", use: "legendary rarity" },
    { name: "tier-slate", hex: "#1B2725", use: "the ground the game draws passive rows on" },
    { name: "tier-ice", hex: "#E9F8FA", use: "tier +1 chevrons" },
    { name: "tier-gold", hex: "#FFE083", use: "tier +2 and +3 chevrons" },
    { name: "tier-red", hex: "#FF4649", use: "negative-tier chevrons, pointing down" },
    { name: "tier-aqua", hex: "#7AFFF2", use: "Rainbow and World Tree chevrons" },
    { name: "tier-indigo", hex: "#334383", use: "the Rainbow tier ground" },
    { name: "tier-violet", hex: "#52359D", use: "the World Tree tier ground" },
  ],
};

const swatches = hslSwatches(colors);
const pc = literalSwatches(literals);

const kit = `${chromeCSS}
${consoleKit}
/* ---- palcon flourishes ----------------------------------------------- */
/* The mock's clipped corner. Sparingly: primary CTAs, the brand mark, the
   switch. Applied globally it becomes the border radius and stops reading. */
.clip-notch { clip-path: polygon(0 0, calc(100% - 10px) 0, 100% 10px, 100% 100%, 10px 100%, 0 calc(100% - 10px)); }
.clip-notch-lg { clip-path: polygon(0 0, calc(100% - 22px) 0, 100% 22px, 100% 100%, 22px 100%, 0 calc(100% - 22px)); }

/* Pal Sphere ring: top half the server's assigned colour, bottom half ink,
   like the capture sphere. The rail's server buttons are drawn with it. */
.sphere-ring { background: conic-gradient(var(--ring-color, var(--pc-pal-green)) 0 50%, var(--pc-ink) 50% 100%); }
.pc-sphere {
  position: relative; height: 44px; width: 44px; flex: none; border: 0; padding: 3px;
  border-radius: 999px; cursor: pointer; transition: opacity 0.12s;
}
.pc-sphere--idle { opacity: 0.6; }
.pc-sphere--idle:hover { opacity: 1; }
.pc-sphere-face {
  display: flex; height: 100%; width: 100%; align-items: center; justify-content: center;
  border-radius: 999px; background: var(--pc-ink); font-family: var(--font-display);
  font-size: 13px; font-weight: 700; color: var(--pc-paper);
}
.pc-sphere-dot {
  position: absolute; right: 1px; bottom: 1px; height: 9px; width: 9px;
  border: 2px solid var(--pc-ink); border-radius: 999px;
}
.pc-brand {
  height: 36px; width: 36px; flex: none; border-radius: 999px;
  background: linear-gradient(to bottom right, var(--pc-brand-red), var(--pc-brand-amber));
}

/* Passive-skill chips: the game's own tier iconography, on the game's own
   slate. Colours are pixel-sampled from the tier icons, not invented. */
.pc-passive {
  display: inline-flex; align-items: center; gap: 5px; border-radius: 999px;
  padding: 3px 9px; font-size: 11px; font-weight: 600; background: var(--pc-tier-slate);
  color: rgb(245 237 225 / 0.9);
}
.pc-passive--rainbow { background: var(--pc-tier-indigo); color: var(--pc-paper); }
.pc-passive--worldtree { background: var(--pc-tier-violet); color: var(--pc-paper); }
.pc-chev { flex: none; }

.pc-portrait {
  display: flex; height: 56px; width: 56px; flex: none; align-items: center; justify-content: center;
  border: 1px solid hsl(var(--border)); border-radius: 14px; background: hsl(var(--muted));
  font-size: 10px; text-align: center; line-height: 1.15; color: hsl(var(--muted-foreground));
}
.pc-portrait--legendary { border-color: rgb(139 58 158 / 0.4); background: rgb(139 58 158 / 0.1); }
.pc-triplet .pc-sep { opacity: 0.5; }
`;

export default {
  app: "palcon",
  title: "Palcon",
  tagline: "the Palworld console",
  intent: `The one <b>light</b> theme in the monorepo, and the one with a real data surface.
      Warm paper, warm ink, brand red-orange for action and brand amber for accent — and a
      destructive red kept <i>deliberately deeper</i> than the brand red, so that kick, ban
      and delete never look like a routine primary. Everything below the chrome is subject to
      a second rule: colours that describe game data are sampled from the game, never chosen
      to match the theme.`,
  fontHref:
    "https://fonts.googleapis.com/css2?family=Baloo+2:wght@500;700;800&family=Manrope:wght@400;500;600;700&family=JetBrains+Mono:wght@500;700&display=swap",
  source: {
    css: "web/palcon/src/index.css",
    tailwind: "web/palcon/tailwind.config.js",
    doc: "docs/palcon-port-verification.md",
  },
  chrome: consoleChrome,
  tokens: {
    colors,
    literals,
    scalars: [{ name: "radius", value: "0.85rem", use: "the round one — the friendliest of the three" }],
    colorScheme: "light",
    derived: {
      "font-display": "'Baloo 2', system-ui, sans-serif",
      "font-body": "Manrope, system-ui, sans-serif",
      "font-mono": "'JetBrains Mono', ui-monospace, monospace",
      live: "var(--pc-brand-amber)",
      "live-dim": "#f6d193",
      warm: "var(--pc-brand-red)",
      "warm-dim": "#f0a08a",
      ok: "var(--pc-pal-green)",
      danger: "hsl(var(--destructive))",
      "meter-track": "hsl(var(--muted))",
      "log-bg": "var(--pc-ink)",
      "rail-bg": "var(--pc-ink)",
      "coin-idle-border": "rgba(43, 36, 32, 0.4)",
      "coin-idle-bg": "var(--pc-ink-light)",
      "coin-idle-fg": "var(--pc-paper)",
      "coin-hi": "var(--pc-brand-amber)",
      "coin-glow": "0 0 8px rgba(242, 169, 59, 0.5)",
      "brand-mark": "linear-gradient(to bottom right, var(--pc-brand-red), var(--pc-brand-amber))",
      "brand-mark-fg": "var(--pc-paper)",
      // The three per-stat tints, from lib/stats.ts. The same colours tint
      // the pal dialog's bars and the compact roster triplet.
      "stat-hp": "#5B9E6F",
      "stat-atk": "#E0502F",
      "stat-def": "#5B8DEF",
    },
  },
  kit,
  groups: [
    {
      name: "Foundations",
      cards: [
        {
          slug: "palette",
          name: "Palette",
          subtitle: "Warm paper and ink, brand red and amber, a deeper alarm red",
          viewport: { width: 940, height: 1020 },
          intent: `The two-layer structure the consoles share, with one decision worth calling
            out: <code>--destructive</code> is <i>not</i> the brand red. It is a deeper,
            more alarm-toned red chosen specifically so a destructive verb stays visually
            distinct from a routine primary — on a page where the primary is already red,
            reusing it would make “Save” and “Delete” the same colour.`,
          specimens: [
            { stage: "block", caption: "Literal palette · brand and ground", html: pc(["brand-red", "brand-amber", "paper", "ink", "ink-light", "ink-soft"]) },
            { stage: "block", caption: "Literal palette · game data", html: pc(["pal-green", "pal-blue", "legendary"]) },
            { stage: "block", caption: "Literal palette · passive tiers, pixel-sampled from the game", html: pc(["tier-slate", "tier-ice", "tier-gold", "tier-red", "tier-aqua", "tier-indigo", "tier-violet"]) },
            { stage: "block", caption: "Semantic roles — note primary vs destructive", html: swatches(["primary", "destructive", "accent", "secondary", "muted", "border"]) },
          ],
          rules: [
            "<b>Destructive is deeper than the brand red.</b> If they ever converge, kick and ban stop looking different from save.",
            "The rail is <code>ink</code> — the one dark surface in a light app. It anchors the page and makes the spheres read.",
            "Data colours are sampled, not chosen: the passive-tier chips take their hues from the game's own tier icons, and the per-stat tints are fixed in <code>lib/stats.ts</code>. Re-theming the console must not re-theme the data.",
            "<code>--radius</code> is <code>0.85rem</code>. Palcon is the round one; wildskeeper is 6px with a notch, flametender is 4px square.",
          ],
          sources: ["web/palcon/src/index.css", "web/palcon/tailwind.config.js", "web/palcon/src/lib/stats.ts"],
        },
        {
          slug: "typography",
          name: "Type",
          subtitle: "Baloo 2 for identity, Manrope for prose, JetBrains Mono for facts",
          viewport: { width: 940, height: 740 },
          intent: `The friendliest of the three type systems, matching the game. <b>Baloo 2</b>
            is the rounded display face for names and numbers; <b>Manrope</b> carries prose;
            <b>JetBrains Mono</b> is for anything a machine produced — ids, coordinates,
            player counts, log lines, save paths.`,
          specimens: [
            {
              stage: "block",
              caption: "Scale",
              html: [
                typeRow("wordmark · Baloo 2 26px", `<span class="con-display" style="font-size:26px;font-weight:800;color:hsl(var(--foreground))">Palcon</span>`),
                typeRow("stat value · Baloo 2 26px", `<span class="con-stat-value">14<small class="con-stat-unit">pals</small></span>`),
                typeRow("panel title · 13px / 0.08em / caps", `<span class="con-panel-t">Guild roster</span>`),
                typeRow("body · Manrope 14px", `Everything the console says in a sentence.`),
                typeRow("label · 11px / 0.14em / caps", `<span class="con-stat-label">Base workers</span>`),
                typeRow("mono · coordinates and ids", `<span class="con-mono" style="font-size:12px">(-142, 318) · steam_76561198000000000</span>`),
                typeRow("stat triplet · mono, per-stat tints", `<span class="con-mono pc-triplet" style="font-size:13px"><span style="color:var(--stat-hp)">4120</span><span class="pc-sep">/</span><span style="color:var(--stat-atk)">318</span><span class="pc-sep">/</span><span style="color:var(--stat-def)">254</span></span>`),
              ].join("\n"),
            },
          ],
          rules: [
            "The stat triplet is always <code>hp / attack / defense</code> in that order and those three tints — the same ones the pal dialog's bars use, so a roster row and a detail view agree.",
            "Coordinates, Steam ids and save paths are mono and never abbreviated: they exist to be copied.",
            "Baloo 2 names things. A paragraph in the display face reads as marketing.",
          ],
          sources: ["web/palcon/tailwind.config.js", "web/palcon/src/components/StatTriplet.tsx", "web/palcon/src/lib/stats.ts"],
        },
      ],
    },
    {
      name: "Components",
      cards: [
        {
          slug: "buttons",
          name: "Buttons",
          subtitle: "Brand red primary, deeper red for loss, generous corners",
          viewport: { width: 940, height: 660 },
          intent: `The same shadcn button as the other two consoles. What the theme changes is
            the corner (0.85rem — noticeably rounder), the primary (brand red-orange) and the
            destructive (a deeper alarm red that must never be mistaken for it). The clipped
            notch appears on a small number of CTAs.`,
          specimens: [
            {
              caption: "Variants — look at primary and destructive side by side",
              html: `          ${cbtn("default", "Start")}\n          ${cbtn("secondary", "Restart")}\n          ${cbtn("outline", "Logs")}\n          ${cbtn("ghost", "Cancel")}\n          ${cbtn("destructive", "Ban player")}\n          ${cbtn("link", "Open the wiki")}`,
            },
            {
              caption: "Sizes, and the disabled state",
              html: `          ${cbtn("default", "Save", " con-btn--sm")}\n          ${cbtn("default", "Save")}\n          ${cbtn("default", "Save", " con-btn--lg")}\n          ${cbtn("default", "Saving…", " is-disabled")}`,
            },
            {
              caption: "The clipped notch, on a CTA",
              html: `          ${cbtn("default", "Provision a server", " clip-notch")}\n          ${cbtn("default", "Not clipped")}`,
            },
          ],
          rules: [
            "Kick, ban, delete and shutdown are <code>destructive</code>. Restart is not.",
            "One primary per panel. On a light theme a second red button competes rather than ranks.",
            "The notch is rationed to CTAs, the brand mark and the switch. Everywhere else the 0.85rem radius is the shape.",
          ],
          sources: ["web/palcon/src/components/ui/button.tsx", "web/palcon/src/components/ui/switch.tsx"],
        },
        {
          slug: "panel",
          name: "Panel and stats",
          subtitle: "The shared grammar, on paper",
          viewport: { width: 940, height: 800 },
          intent: `The same panel and stat tile as wildskeeper and flametender, proving the
            point about the shared kit: nothing in the markup changes, only the tokens
            underneath it. The log panel is the exception worth noticing — it keeps a dark
            ground even here, because a server log is machine output and reads better as a
            terminal than as a document.`,
          specimens: [
            {
              stage: "block",
              caption: "Panel with meta and a note",
              html: panel(
                "Server log",
                "last 200 lines",
                `<div class="con-log" style="color:var(--pc-paper)">
              <div class="con-log-line con-log-line--join" style="color:var(--pc-brand-amber)">[19:14:02] hazel joined the server</div>
              <div class="con-log-line con-log-line--save" style="color:var(--pc-pal-green)">[19:20:11] world saved</div>
              <div class="con-log-line" style="color:rgb(245 237 225 / 0.7)">[19:22:40] rook joined the server</div>
              <div class="con-log-line con-log-line--error" style="color:#ff7a6a">[19:23:01] RCON: connection refused</div>
            </div>
            <p class="con-note">Lines are coloured by what they say. Palworld emits no severity levels.</p>`,
              ),
            },
            {
              stage: "block",
              caption: "Stat tiles",
              html: `          <div class="con-stats">
${stat({ label: "Players", value: "4", unit: "of 32", hint: "peak 9 today", meter: 13, warm: true })}
${stat({ label: "Pals", value: "217", hint: "across 3 guilds" })}
${stat({ label: "Memory", value: "11.4", unit: "GB", hint: "of 16 GB", meter: 71 })}
${stat({ label: "Uptime", value: "2d", unit: "9h", hint: "since the last restart" })}
          </div>`,
            },
          ],
          rules: [
            "The log keeps a dark ground on the light theme. Machine output is not prose.",
            "A meter appears only when the number has a ceiling — “Pals” has none.",
            "The note states a capability gap in the game's own terms, never as a console limitation.",
          ],
          sources: ["web/palcon/src/components/ui/card.tsx", "web/palcon/src/components/ContainerLogsDialog.tsx"],
        },
        {
          slug: "shell",
          name: "Shell and Pal Spheres",
          subtitle: "A dark rail in a light app",
          viewport: { width: 1000, height: 640 },
          intent: `The rail is the one dark surface here, and its servers are <b>Pal Spheres</b>:
            a conic ring split top-half in the server's assigned colour and bottom-half ink,
            like the game's capture sphere, with the server's initials in the middle and a
            connectivity dot on the rim. The dot re-probes on a timer, so a server that comes
            back stops looking offline without anyone refreshing.`,
          specimens: [
            {
              stage: "block",
              caption: "Desktop shell",
              html: `          <div class="con-shell">
            <aside class="con-rail">
              <span class="pc-brand clip-notch" title="Palcon"></span>
              <button class="pc-sphere sphere-ring" style="--ring-color: var(--pc-pal-green)"><span class="pc-sphere-face">PV</span><span class="pc-sphere-dot" style="background: var(--pc-pal-green)"></span></button>
              <button class="pc-sphere pc-sphere--idle sphere-ring" style="--ring-color: var(--pc-pal-blue)"><span class="pc-sphere-face">SK</span><span class="pc-sphere-dot" style="background: var(--pc-ink-soft)"></span></button>
              <button class="con-coin con-coin--add">+</button>
              <span class="con-rail-spacer"></span>
              <button class="con-coin con-coin--idle" style="height:40px;width:40px;font-size:13px">⌂</button>
            </aside>
            <div class="con-main">
              <div class="con-subnav">
                <a class="con-subnav-item is-active">Dashboard</a>
                <a class="con-subnav-item">Players</a>
                <a class="con-subnav-item">Pals</a>
                <a class="con-subnav-item">Map</a>
                <a class="con-subnav-item">Settings</a>
              </div>
              <div class="con-body">
                <div class="con-panel" style="padding:20px 22px">
                  <div class="con-stat-label">Palworld</div>
                  <div class="con-display" style="font-size:24px;font-weight:800;color:hsl(var(--foreground));margin-top:2px">Palvale</div>
                  <div class="con-panel-meta"><span class="con-dot con-dot--ok"></span> running · 4 of 32 players online</div>
                </div>
                <div class="con-stats">
${stat({ label: "Players", value: "4", unit: "of 32", meter: 13, warm: true })}
${stat({ label: "Pals", value: "217", hint: "across 3 guilds" })}
                </div>
              </div>
            </div>
          </div>`,
            },
          ],
          rules: [
            "The active sphere is at full opacity; the others sit at 60% and come up on hover. There is no second “active” treatment.",
            "The rim dot is green when the server answered, ink-soft when it did not, and translucent while the probe is still out — three states, not two.",
            "Switching servers keeps the current view: from a map, land on the new server's map.",
          ],
          sources: [
            "web/palcon/src/components/ServerRail.tsx",
            "web/palcon/src/components/ServerSphere.tsx",
            "web/palcon/src/lib/palette.ts",
          ],
        },
        {
          slug: "game-data",
          name: "Game data",
          subtitle: "Passives, rarity and stats — colours sampled, not chosen",
          viewport: { width: 940, height: 820 },
          intent: `Palcon's data surface is the reason its palette is bigger than the other two.
            These colours are <b>sampled from the game</b>: the passive-tier chips mirror the
            game's own tier iconography down to the slate ground and the aqua chevrons of the
            Rainbow and World Tree tiers, and the per-stat tints are fixed constants. A player
            comparing the console against the game should see the same colours mean the same
            things — which means a theme change must not touch any of them.`,
          specimens: [
            {
              caption: "Passive-skill chips — one chevron per tier step, pointing the way the tier pulls",
              html: `          <span class="pc-passive"><svg class="pc-chev" width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="var(--pc-tier-ice)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M1.5 6.25 L5 3.75 L8.5 6.25"/></svg>Swift</span>
          <span class="pc-passive"><svg class="pc-chev" width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="var(--pc-tier-gold)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M1.5 4.75 L5 2.25 L8.5 4.75"/><path d="M1.5 7.75 L5 5.25 L8.5 7.75"/></svg>Musclehead</span>
          <span class="pc-passive"><svg class="pc-chev" width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="var(--pc-tier-red)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><g transform="rotate(180 5 5)"><path d="M1.5 4.75 L5 2.25 L8.5 4.75"/><path d="M1.5 7.75 L5 5.25 L8.5 7.75"/></g></svg>Coward</span>
          <span class="pc-passive pc-passive--rainbow"><svg class="pc-chev" width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="var(--pc-tier-aqua)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M1.5 3.25 L5 0.75 L8.5 3.25"/><path d="M1.5 6.25 L5 3.75 L8.5 6.25"/><path d="M1.5 9.25 L5 6.75 L8.5 9.25"/></svg>Legend</span>
          <span class="pc-passive pc-passive--worldtree"><svg class="pc-chev" width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="var(--pc-tier-aqua)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M1.5 3.25 L5 0.75 L8.5 3.25"/><path d="M1.5 6.25 L5 3.75 L8.5 6.25"/><path d="M1.5 9.25 L5 6.75 L8.5 9.25"/></svg>Vanguard</span>`,
            },
            {
              stage: "block",
              caption: "A roster row: portrait, rarity, stats",
              html: panel(
                "Guild roster",
                "14 pals",
                `<table class="con-table">
              <tr><th></th><th>Pal</th><th>Level</th><th>HP / ATK / DEF</th><th>Passives</th></tr>
              <tr>
                <td style="width:70px"><div class="pc-portrait pc-portrait--legendary">Jetragon</div></td>
                <td><b>Jetragon</b><br><span class="con-badge con-badge--outline" style="border-color:rgb(139 58 158 / 0.4);color:var(--pc-legendary);margin-top:4px">LEGENDARY</span></td>
                <td class="con-mono">50</td>
                <td class="con-mono pc-triplet"><span style="color:var(--stat-hp)">4120</span><span class="pc-sep">/</span><span style="color:var(--stat-atk)">318</span><span class="pc-sep">/</span><span style="color:var(--stat-def)">254</span></td>
                <td><span class="pc-passive"><svg class="pc-chev" width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="var(--pc-tier-gold)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M1.5 4.75 L5 2.25 L8.5 4.75"/><path d="M1.5 7.75 L5 5.25 L8.5 7.75"/></svg>Musclehead</span></td>
              </tr>
              <tr>
                <td><div class="pc-portrait">Lyleen</div></td>
                <td><b>Lyleen</b></td>
                <td class="con-mono">42</td>
                <td class="con-mono pc-triplet"><span style="color:var(--stat-hp)">3180</span><span class="pc-sep">/</span><span style="color:var(--stat-atk)">210</span><span class="pc-sep">/</span><span style="color:var(--stat-def)">198</span></td>
                <td><span class="pc-passive"><svg class="pc-chev" width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="var(--pc-tier-ice)" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M1.5 6.25 L5 3.75 L8.5 6.25"/></svg>Serious</span></td>
              </tr>
            </table>
            <p class="con-note">Stats are estimates computed from species base values, level and talents — the save does not store an effective number.</p>`,
              ),
            },
          ],
          rules: [
            "Chevrons are capped at three and point down for negative tiers. Tiers 4 and 5 draw the full three and let their ground colour carry the rank.",
            "Rarity tints the portrait's border and ground at low opacity, never the portrait itself — the artwork brings its own colour.",
            "Anything estimated says so. A computed stat presented as a stored fact is the kind of thing a player builds a team around.",
          ],
          sources: [
            "web/palcon/src/components/PassiveBadge.tsx",
            "web/palcon/src/components/PalPortrait.tsx",
            "web/palcon/src/components/StatTriplet.tsx",
            "web/palcon/src/lib/stats.ts",
          ],
        },
      ],
    },
  ],
};
