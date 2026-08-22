// Flametender's design system, as data.
//
// The Enshrouded console. Moss-black ground under a fog line, weathered-stone
// structure, and — the mirror of wildskeeper's rule — **flame azure is
// reserved for live state**. The two consoles are the same machine with
// different reserved colours, which is the clearest evidence that the theme
// really is only tokens: swap the values and nothing else moves.
//
// Structure comes from `../lib/console.mjs`. What is here is the theme: the
// values, the three typefaces, and the flourishes that make it Enshrouded —
// the terrace top-light, the fog line pooled at the foot of the page, the
// horizon rule, and the flame that breathes.
//
// Design source: `games/enshrouded/docs/design.md` in the archived flametender
// workspace; `scripts/checkdesign.sh` fails CI if the semantic mapping in
// `web/flametender/src/index.css` or the `ft.*` palette in
// `web/flametender/tailwind.config.js` stops matching this file.
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
  { name: "background", value: "144 14% 7%", hex: "#101512", use: "ft.void — the moss-black ground" },
  { name: "foreground", value: "47 24% 85%", hex: "#e2ded0", use: "ft.bone — body text" },
  { name: "card", value: "144 15% 14%", hex: "#1e2822", use: "ft.panel — every terrace" },
  { name: "card-foreground", value: "47 24% 85%", hex: "#e2ded0", use: "text on a terrace" },
  { name: "popover", value: "144 15% 14%", hex: "#1e2822", use: "dialogs, menus, tooltips" },
  { name: "popover-foreground", value: "47 24% 85%", hex: "#e2ded0", use: "text in an overlay" },
  { name: "primary", value: "45 12% 54%", hex: "#98917c", use: "ft.stone — the default button" },
  { name: "primary-foreground", value: "144 14% 7%", hex: "#101512", use: "void, for text on stone" },
  { name: "secondary", value: "141 13% 20%", hex: "#2d3b32", use: "ft.edge — the quiet button and the active pill" },
  { name: "secondary-foreground", value: "47 24% 85%", hex: "#e2ded0", use: "text on the quiet button" },
  { name: "muted", value: "137 13% 11%", hex: "#182019", use: "ft.fog — the shade the fog line is made of" },
  { name: "muted-foreground", value: "98 8% 54%", hex: "#87947e", use: "ft.lichen — secondary text and labels" },
  { name: "accent", value: "44 23% 71%", hex: "#c6bda4", use: "ft.stonehi — panel titles, hover, warnings" },
  { name: "accent-foreground", value: "144 14% 7%", hex: "#101512", use: "text on lit stone" },
  { name: "destructive", value: "6 53% 54%", hex: "#c95a4d", use: "ft.spore — danger" },
  { name: "destructive-foreground", value: "47 24% 85%", hex: "#e2ded0", use: "text on spore" },
  { name: "border", value: "141 13% 20%", hex: "#2d3b32", use: "every border" },
  { name: "input", value: "141 13% 20%", hex: "#2d3b32", use: "field borders" },
  { name: "ring", value: "204 79% 72%", hex: "#7fc3f0", use: "ft.flame — focus is live-state territory" },
];

const literals = {
  prefix: "ft",
  entries: [
    { name: "void", hex: "#101512", use: "page ground" },
    { name: "fog", hex: "#182019", use: "the Shroud pooled at the foot of the page" },
    { name: "panel", hex: "#1e2822", use: "terraces" },
    { name: "edge", hex: "#2d3b32", use: "borders" },
    { name: "stone", hex: "#98917c", use: "structure and interaction" },
    { name: "stonehi", hex: "#c6bda4", use: "lit stone — titles, hover, warnings" },
    { name: "flame", hex: "#7fc3f0", use: "RESERVED: live and active state only" },
    { name: "flamehi", hex: "#d3ecff", use: "the brightest flame, for a selection" },
    { name: "flamedim", hex: "#31536b", use: "the dim end of a live meter" },
    { name: "spore", hex: "#c95a4d", use: "danger and errors" },
    { name: "sporedim", hex: "#66312a", use: "the dim end of a danger fill" },
    { name: "bone", hex: "#e2ded0", use: "text" },
    { name: "lichen", hex: "#87947e", use: "secondary text" },
    { name: "ok", hex: "#82b378", use: "healthy, saved, running" },
  ],
};

const swatches = hslSwatches(colors);
const ft = literalSwatches(literals);

const kit = `${chromeCSS}
${consoleKit}
/* ---- flametender flourishes ------------------------------------------ */
/* The terrace top-light: a 1px inset highlight — stone catching light from
   above while the fog sits below. Panels and cards, never controls. */
.ft-toplight { box-shadow: inset 0 1px 0 rgba(226, 222, 208, 0.07); }

/* The fog line: the Shroud pooled at the foot of the viewport, the one piece
   of atmosphere on the page. Static by design. */
.ft-ground {
  background-image:
    linear-gradient(to top, rgba(24, 32, 25, 0.9), rgba(24, 32, 25, 0.45) 30%, transparent 55%),
    radial-gradient(ellipse 1000px 420px at 50% 108%, rgba(52, 68, 55, 0.35), transparent);
}

/* The horizon rule under the Overview hero: a stone line fading at both ends
   — the page's single flourish outside the fog and the flame. */
.ft-horizon {
  height: 1px;
  background: linear-gradient(90deg, transparent, rgba(152, 145, 124, 0.55) 12%, rgba(152, 145, 124, 0.55) 88%, transparent);
}

/* The lit flame's breathing — slow enough to read as a flame at rest, not an
   alert. Stilled for reduced motion. */
@keyframes ft-flicker {
  0%, 100% { opacity: 0.65; transform: scale(1); }
  38% { opacity: 1; transform: scale(1.04); }
  62% { opacity: 0.82; transform: scale(0.985); }
}
.ft-flame-lit { animation: ft-flicker 3.6s ease-in-out infinite; transform-origin: 50% 80%; }
@media (prefers-reduced-motion: reduce) { .ft-flame-lit { animation: none; opacity: 0.9; } }

.ft-sigil {
  display: flex; height: 36px; width: 36px; align-items: center; justify-content: center;
  border: 1px solid var(--ft-stone); border-radius: 999px; background: var(--ft-panel);
  font-family: var(--font-display); font-size: 16px; color: var(--ft-stonehi);
}
`;

export default {
  app: "flametender",
  title: "Flametender",
  tagline: "the Enshrouded console",
  intent: `Moss-black under a fog line, weathered stone for structure, and <b>one reserved
      colour</b> — flame azure (<code>#7fc3f0</code>) marks live and active state and nothing
      else. Compare it to wildskeeper: same rail, same panel grammar, same stat tile, a
      different palette and a different reserved hue. The theme is the tokens; everything
      else is shared. The page is deliberately still — a monitoring console should be
      visually silent when nothing is happening, so the only motion here is one flame
      breathing, and even that stops under <code>prefers-reduced-motion</code>.`,
  fontHref:
    "https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500;600&family=Grenze+Gotisch:wght@500;600;700&family=Karla:ital,wght@0,400;0,500;0,700;1,400&display=swap",
  source: {
    css: "web/flametender/src/index.css",
    tailwind: "web/flametender/tailwind.config.js",
    doc: "docs/flametender-port-verification.md",
  },
  chrome: consoleChrome,
  tokens: {
    colors,
    literals,
    scalars: [{ name: "radius", value: "0.25rem", use: "calm terraces, no clipped corners" }],
    colorScheme: "dark",
    derived: {
      "font-display": "'Grenze Gotisch', serif",
      "font-body": "Karla, system-ui, sans-serif",
      "font-mono": "'IBM Plex Mono', ui-monospace, monospace",
      live: "var(--ft-flame)",
      "live-dim": "var(--ft-flamedim)",
      warm: "var(--ft-stonehi)",
      "warm-dim": "#5c5747",
      ok: "var(--ft-ok)",
      danger: "var(--ft-spore)",
      "meter-track": "var(--ft-void)",
      "log-bg": "var(--ft-void)",
      "rail-bg": "var(--ft-void)",
      "coin-idle-border": "rgba(152, 145, 124, 0.6)",
      "coin-idle-bg": "var(--ft-fog)",
      "coin-idle-fg": "var(--ft-stonehi)",
      "coin-hi": "var(--ft-stonehi)",
      "coin-glow": "0 0 8px rgba(127, 195, 240, 0.45)",
      "brand-mark": "var(--ft-panel)",
      "brand-mark-fg": "var(--ft-stonehi)",
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
          subtitle: "Stone structure, flame live state, spore danger",
          viewport: { width: 940, height: 980 },
          intent: `Two layers, as in all three consoles. The <b>literal palette</b>
            (<code>ft.*</code>) is the theme as designed; the <b>semantic tokens</b> are that
            palette mapped onto shadcn's roles so the components shared with palcon and
            wildskeeper need no per-console work. The one thing worth memorising is which
            colour is reserved.`,
          specimens: [
            { stage: "block", caption: "Literal palette · grounds", html: ft(["void", "fog", "panel", "edge"]) },
            { stage: "block", caption: "Literal palette · stone is structure", html: ft(["stone", "stonehi"]) },
            { stage: "block", caption: "Literal palette · reserved, and state", html: ft(["flame", "flamehi", "flamedim", "ok", "spore", "sporedim"]) },
            { stage: "block", caption: "Literal palette · text", html: ft(["bone", "lichen"]) },
            { stage: "block", caption: "Semantic roles the components actually use", html: swatches(["primary", "secondary", "accent", "destructive", "muted", "ring"]) },
          ],
          rules: [
            "<b>Flame azure is reserved.</b> If something is blue, something is live. It is never used to make a terrace look nice.",
            "Stone is structure and interaction; lit stone (<code>stonehi</code>) is the hover and the warning. Spore is the fault.",
            "A load meter fills flame; a capacity meter fills stone. The same rule wildskeeper follows with rune and brass.",
            "<code>--radius</code> is <code>0.25rem</code>: calm terraces, no clipped corners. That is the deliberate difference from wildskeeper's notch.",
          ],
          sources: ["web/flametender/src/index.css", "web/flametender/tailwind.config.js"],
        },
        {
          slug: "typography",
          name: "Type",
          subtitle: "Grenze Gotisch budgeted, Karla for everything, IBM Plex Mono for facts",
          viewport: { width: 940, height: 760 },
          intent: `The blackletter is <b>rationed harder than wildskeeper's Cinzel</b>. Grenze
            Gotisch is kept for identity moments — the wordmark, the server coins, a stat
            tile's number — and explicitly <i>not</i> for panel titles, which are Karla in
            small caps. A blackletter panel title reads as a fantasy skin; a blackletter
            number reads as the console having a voice.`,
          specimens: [
            {
              stage: "block",
              caption: "Scale",
              html: [
                typeRow("wordmark · Grenze Gotisch 26px", `<span class="con-display" style="font-size:26px;color:hsl(var(--foreground))">Flametender</span>`),
                typeRow("stat value · Grenze Gotisch 26px", `<span class="con-stat-value">42<small class="con-stat-unit">players</small></span>`),
                typeRow("panel title · Karla 13px / 0.12em / caps", `<span class="con-panel-t">Server health</span>`),
                typeRow("body · Karla 14px", `Everything the console says in a sentence.`),
                typeRow("label · 11px / 0.14em / caps", `<span class="con-stat-label">Memory</span>`),
                typeRow("note · 12px / italic", `<span class="con-note" style="margin:0">Enshrouded reports no per-player ping.</span>`),
                typeRow("mono · logs and ids", `<span class="con-mono" style="font-size:12px">[19:14:02] player connected: hazel · 10.0.0.4:15637</span>`),
              ].join("\n"),
            },
          ],
          rules: [
            "The display face has a budget. Wordmark, coins, stat numbers — that is the list.",
            "Panel titles are the body face. This is the one place flametender and wildskeeper deliberately diverge in structure rather than colour.",
            "Webfonts load from Google Fonts with a real fallback stack; the console stays legible before they arrive.",
          ],
          sources: ["web/flametender/tailwind.config.js", "web/flametender/index.html"],
        },
        {
          slug: "atmosphere",
          name: "Atmosphere",
          subtitle: "Fog below, top-light above, one flame breathing",
          viewport: { width: 940, height: 780 },
          intent: `The fog line and the terrace top-light are <b>one physical model</b>: stone
            catching light from above while the Shroud pools below. They are applied together
            or not at all — a panel with a top-light on a page with no fog reads as a stray
            highlight. Everything here is static except the lit flame, and that stops under
            reduced motion.`,
          specimens: [
            {
              stage: "block",
              caption: "The fog line, with terraces standing in it",
              html: `          <div class="ft-ground" style="padding:26px;border-radius:var(--radius)">
            <div class="con-panel ft-toplight" style="padding:18px 20px;margin-bottom:14px">
              <div class="con-stat-label">Enshrouded</div>
              <div class="con-display" style="font-size:24px;color:hsl(var(--foreground));margin-top:2px">Embervale</div>
            </div>
            <div class="ft-horizon" style="margin:18px 0"></div>
            <div class="con-panel ft-toplight" style="padding:14px 18px;font-size:13px;color:hsl(var(--muted-foreground))">A second terrace, lower in the fog.</div>
          </div>`,
            },
            {
              caption: "The flame: lit breathes, unlit is still",
              html: `          <span class="ft-sigil"><span class="ft-flame-lit" style="color:var(--ft-flame)">▲</span></span>
          <span class="ft-sigil"><span style="color:var(--ft-lichen)">▲</span></span>
          <span class="con-panel-meta">lit means the server is up; unlit means it is not</span>`,
            },
            {
              stage: "block",
              caption: "The horizon rule",
              html: `          <div class="ft-horizon"></div>`,
            },
          ],
          rules: [
            "Fog and top-light ship together. They are one model, not two effects.",
            "The horizon rule appears once per page, under the Overview hero. A second one is a divider, and dividers are borders.",
            "One animation on the page, and it is stilled — not slowed — under <code>prefers-reduced-motion</code>.",
          ],
          sources: ["web/flametender/src/index.css", "web/flametender/src/components/flametender/FlameSigil.tsx"],
        },
      ],
    },
    {
      name: "Components",
      cards: [
        {
          slug: "buttons",
          name: "Buttons",
          subtitle: "The same six, stone-toned, square corners",
          viewport: { width: 940, height: 640 },
          intent: `Identical to wildskeeper's component; the visible differences are entirely
            token-deep. <code>default</code> is stone rather than brass, <code>destructive</code>
            is spore rather than ember, focus is flame rather than rune, and the corner radius
            is 4px rather than 6px with no notch anywhere.`,
          specimens: [
            {
              caption: "Variants",
              html: `          ${cbtn("default", "Start")}\n          ${cbtn("secondary", "Restart")}\n          ${cbtn("outline", "Logs")}\n          ${cbtn("ghost", "Cancel")}\n          ${cbtn("destructive", "Delete server")}\n          ${cbtn("link", "Open the wiki")}`,
            },
            {
              caption: "Sizes, and the disabled state",
              html: `          ${cbtn("default", "Save", " con-btn--sm")}\n          ${cbtn("default", "Save")}\n          ${cbtn("default", "Save", " con-btn--lg")}\n          ${cbtn("default", "Saving…", " is-disabled")}`,
            },
            {
              caption: "Focus is flame, because focus is live state",
              html: `          <span style="display:inline-flex;box-shadow:0 0 0 1px hsl(var(--ring));border-radius:calc(var(--radius) - 2px)">${cbtn("default", "Start")}</span>
          <span style="display:inline-flex;box-shadow:0 0 0 1px hsl(var(--ring));border-radius:calc(var(--radius) - 2px)">${cbtn("outline", "Logs")}</span>`,
            },
          ],
          rules: [
            "No clipped corners. The notch is wildskeeper's signature and borrowing it blurs two themes that are otherwise distinguishable at a glance.",
            "<code>destructive</code> is for verbs that lose something unrecoverable. Restart is not destructive.",
            "A feature Enshrouded cannot support answers with a 501 that names where the ability actually lives, rather than hiding the button.",
          ],
          sources: ["web/flametender/src/components/ui/button.tsx", "web/flametender/src/components/FeatureGate.tsx"],
        },
        {
          slug: "panel",
          name: "Terrace and stats",
          subtitle: "The grammar every page is built from",
          viewport: { width: 940, height: 840 },
          intent: `A calm rectangle with a top-light on its lintel, a quiet stone title in the
            body face, an optional right-aligned meta line, and a body. The stat tile is the
            same idea at small scale, and it is where the blackletter is allowed out.`,
          specimens: [
            {
              stage: "block",
              caption: "Terrace with meta and a note",
              html: panel(
                "Server log",
                "last 200 lines",
                `<div class="con-log">
              <div class="con-log-line con-log-line--join">[19:14:02] player connected: hazel</div>
              <div class="con-log-line con-log-line--save">[19:20:11] world saved (48 MB)</div>
              <div class="con-log-line con-log-line--warn">[19:22:40] warning: tick took 380ms</div>
              <div class="con-log-line con-log-line--error">[19:23:01] error: could not reach the save directory</div>
              <div class="con-log-line">[19:24:00] player disconnected: hazel</div>
            </div>
            <p class="con-note">Lines are coloured by what they say. Enshrouded emits no severity levels, so inventing them would be a guess presented as a fact.</p>`,
                "ft-toplight",
              ),
            },
            {
              stage: "block",
              caption: "Stat tiles — flame for load, stone for capacity",
              html: `          <div class="con-stats">
${stat({ label: "Players", value: "5", unit: "of 16", hint: "peak 11 today", meter: 31, warm: true })}
${stat({ label: "CPU", value: "36", unit: "%", hint: "4 cores allocated", meter: 36 })}
${stat({ label: "Memory", value: "9.1", unit: "GB", hint: "of 16 GB", meter: 57, warm: true })}
${stat({ label: "Uptime", value: "11d", unit: "2h", hint: "since the last restart" })}
          </div>`
                .split("\n")
                .map((l) => l.replace('class="con-stat"', 'class="con-stat ft-toplight"'))
                .join("\n"),
            },
          ],
          rules: [
            "The top-light goes on panels and tiles, never on a control. A button that catches the light reads as raised, and these controls are flush.",
            "A meter appears only when the number has a ceiling.",
            "The note is where a capability gap is stated plainly. It is italic and quiet because it is a fact about the game, not a warning about the console.",
          ],
          sources: [
            "web/flametender/src/components/flametender/FtPanel.tsx",
            "web/flametender/src/components/ui/card.tsx",
          ],
        },
        {
          slug: "shell",
          name: "Shell and server rail",
          subtitle: "Flame coins, one lit",
          viewport: { width: 1000, height: 640 },
          intent: `The same rail as wildskeeper's, down to the pixel sizes: a sigil, one coin
            per server with its initial in the display face, add and host and sign-out. The
            active coin's ring lights flame with a soft glow.`,
          specimens: [
            {
              stage: "block",
              caption: "Desktop shell",
              html: `          <div class="con-shell ft-ground">
            <aside class="con-rail">
              <span class="ft-sigil" title="Flametender"><span class="ft-flame-lit" style="color:var(--ft-flame)">▲</span></span>
              <button class="con-coin con-coin--active">E</button>
              <button class="con-coin con-coin--idle">V</button>
              <button class="con-coin con-coin--idle">S</button>
              <button class="con-coin con-coin--add">+</button>
              <span class="con-rail-spacer"></span>
              <button class="con-coin con-coin--idle" style="height:40px;width:40px;font-size:13px">⌂</button>
            </aside>
            <div class="con-main">
              <div class="con-subnav">
                <a class="con-subnav-item is-active">Overview</a>
                <a class="con-subnav-item">Players</a>
                <a class="con-subnav-item">Bans</a>
                <a class="con-subnav-item">Roles</a>
                <a class="con-subnav-item">Settings</a>
              </div>
              <div class="con-body">
                <div class="con-panel ft-toplight" style="padding:20px 22px">
                  <div class="con-stat-label">Enshrouded</div>
                  <div class="con-display" style="font-size:24px;color:hsl(var(--foreground));margin-top:2px">Embervale</div>
                  <div class="con-panel-meta"><span class="con-dot con-dot--ok"></span> running · 5 of 16 flames lit</div>
                </div>
                <div class="ft-horizon"></div>
                <div class="con-stats">
                  <div class="con-stat ft-toplight"><div class="con-stat-label">Players</div><div class="con-stat-value">5<small class="con-stat-unit">of 16</small></div><div class="con-meter"><i class="con-meter-f con-meter-f--warm" style="width:31%"></i></div></div>
                  <div class="con-stat ft-toplight"><div class="con-stat-label">CPU</div><div class="con-stat-value">36<small class="con-stat-unit">%</small></div><div class="con-meter"><i class="con-meter-f" style="width:36%"></i></div></div>
                </div>
              </div>
            </div>
          </div>`,
            },
          ],
          rules: [
            "One coin is lit, and it is lit in the reserved colour.",
            "Admin-only rail buttons render only for admins.",
            "The mobile sub-nav is a scrolling pill row with an edge fade instead of a scrollbar; the layout uses <code>dvh</code> so a collapsing address bar cannot hide the bottom rail.",
          ],
          sources: [
            "web/flametender/src/components/ServerRail.tsx",
            "web/flametender/src/components/flametender/FtServerFlame.tsx",
            "web/flametender/src/components/MobileChrome.tsx",
          ],
        },
        {
          slug: "state",
          name: "State, bans and dialogs",
          subtitle: "Running, stopped, unreachable — and confirming a loss",
          viewport: { width: 940, height: 760 },
          intent: `The dot vocabulary is fixed across all three consoles: green for healthy,
            the reserved colour for live activity, mist for stopped, danger for a fault. A ban
            is the console's most consequential routine verb, so it names the person, the
            reason and the reversal.`,
          specimens: [
            {
              caption: "Status dots and badges",
              html: `          <span class="con-panel-meta"><span class="con-dot con-dot--ok"></span> running</span>
          <span class="con-panel-meta"><span class="con-dot con-dot--live"></span> saving…</span>
          <span class="con-panel-meta"><span class="con-dot con-dot--down"></span> stopped</span>
          <span class="con-panel-meta"><span class="con-dot con-dot--danger"></span> unreachable</span>
          <span class="con-badge con-badge--outline">ADMIN</span>
          <span class="con-badge con-badge--secondary">v0.8.1</span>
          <span class="con-badge con-badge--destructive">BANNED</span>`,
            },
            {
              stage: "block",
              caption: "The bans table",
              html: panel(
                "Bans",
                "3 entries",
                `<table class="con-table">
              <tr><th>Player</th><th>Reason</th><th>Since</th><th></th></tr>
              <tr><td>rook</td><td>griefing the shared base</td><td class="con-mono">2026-08-14</td><td style="text-align:right">${cbtn("outline", "Lift", " con-btn--sm")}</td></tr>
              <tr><td>wisp</td><td>—</td><td class="con-mono">2026-07-02</td><td style="text-align:right">${cbtn("outline", "Lift", " con-btn--sm")}</td></tr>
            </table>
            <p class="con-note">A ban with no reason is still a ban, but nobody a year from now will know why.</p>`,
                "ft-toplight",
              ),
            },
            {
              stage: "block",
              caption: "Confirming a loss",
              html: `          <div class="con-dialog">
            <h2 class="con-dialog-t">Delete Embervale?</h2>
            <p class="con-dialog-b">The container, its world save and its logs are removed from the host. This cannot be undone — download a backup first if you want one.</p>
            <div class="con-dialog-f">${cbtn("ghost", "Cancel")}${cbtn("destructive", "Delete server")}</div>
          </div>`,
            },
          ],
          rules: [
            "Every destructive dialog names the cost in the same sentence shape: what is removed, that it cannot be undone, and the way out.",
            "A reversible action gets an <code>outline</code> button (“Lift”), not a destructive one. Colour tracks consequence, not severity of tone.",
            "Spore is for faults. A slow tick is lit stone.",
          ],
          sources: [
            "web/flametender/src/components/flametender/FtBans.tsx",
            "web/flametender/src/components/DeleteServerDialog.tsx",
            "web/flametender/src/components/ServerUnreachable.tsx",
          ],
        },
      ],
    },
  ],
};
