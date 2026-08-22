// Wildskeeper's design system, as data.
//
// The Dragonwilds console. Deep-night ground, brass structure, and one rule
// that holds the whole theme together: **rune cyan is reserved for live
// state**. Nothing decorative is allowed to be cyan, so when something on the
// page is cyan it means something is happening right now.
//
// Structure comes from `../lib/console.mjs` — palcon, wildskeeper and
// flametender are the same application wearing three faces, and all three map
// a literal palette onto shadcn's semantic tokens for exactly that reason.
// What is here is the theme: the values, the two typefaces, and the three
// flourishes that make it Dragonwilds rather than Enshrouded.
//
// Design source: `mocks/dragonwilds-dashboard.html` (in the archived
// wildskeeper workspace); the palette is reproduced in
// `web/wildskeeper/tailwind.config.js` as `wk.*`, and
// `scripts/checkdesign.sh` fails CI if either that or the semantic mapping in
// `web/wildskeeper/src/index.css` stops matching this file.
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

/** shadcn's semantic tokens, carrying wildskeeper's values. */
const colors = [
  { name: "background", value: "215 24% 8%", hex: "#10141a", use: "wk.bg — the deep-night ground" },
  { name: "foreground", value: "44 40% 84%", hex: "#e6dcc4", use: "wk.parchment — body text" },
  { name: "card", value: "217 28% 15%", hex: "#1b2330", use: "wk.panel — every panel and tile" },
  { name: "card-foreground", value: "44 40% 84%", hex: "#e6dcc4", use: "text on a panel" },
  { name: "popover", value: "217 28% 15%", hex: "#1b2330", use: "dialogs, menus, tooltips" },
  { name: "popover-foreground", value: "44 40% 84%", hex: "#e6dcc4", use: "text in an overlay" },
  { name: "primary", value: "40 40% 39%", hex: "#8a6f3a", use: "wk.brass — the default button" },
  { name: "primary-foreground", value: "44 40% 84%", hex: "#e6dcc4", use: "text on brass" },
  { name: "secondary", value: "216 27% 19%", hex: "#232d3d", use: "wk.edge — the quiet button, and the active sub-nav pill" },
  { name: "secondary-foreground", value: "44 40% 84%", hex: "#e6dcc4", use: "text on the quiet button" },
  { name: "muted", value: "213 26% 12%", hex: "#161c24", use: "wk.raise — the shade between ground and panel" },
  { name: "muted-foreground", value: "88 6% 56%", hex: "#8e9688", use: "wk.mist — secondary text and labels" },
  { name: "accent", value: "42 51% 54%", hex: "#c9a24b", use: "wk.brasshi — panel titles, hover, warnings" },
  { name: "accent-foreground", value: "220 24% 6%", hex: "#0b0e12", use: "wk.ink — text on lit brass" },
  { name: "destructive", value: "16 71% 58%", hex: "#e0704a", use: "wk.ember — danger" },
  { name: "destructive-foreground", value: "44 40% 84%", hex: "#e6dcc4", use: "text on ember" },
  { name: "border", value: "216 27% 19%", hex: "#232d3d", use: "every border" },
  { name: "input", value: "216 27% 19%", hex: "#232d3d", use: "field borders" },
  { name: "ring", value: "176 63% 58%", hex: "#52d8d0", use: "wk.rune — focus is live-state territory" },
];

/** The literal palette, for the spots that need an exact hue rather than a
 *  semantic role. Checked against `web/wildskeeper/tailwind.config.js`. */
const literals = {
  prefix: "wk",
  entries: [
    { name: "bg", hex: "#10141a", use: "page ground" },
    { name: "raise", hex: "#161c24", use: "the shade between ground and panel" },
    { name: "panel", hex: "#1b2330", use: "panels" },
    { name: "edge", hex: "#232d3d", use: "borders" },
    { name: "brass", hex: "#8a6f3a", use: "structure and interaction" },
    { name: "brasshi", hex: "#c9a24b", use: "lit brass — titles, hover, warnings" },
    { name: "rune", hex: "#52d8d0", use: "RESERVED: live and active state only" },
    { name: "runedim", hex: "#2b6f6c", use: "the dim end of a live meter" },
    { name: "ember", hex: "#e0704a", use: "danger and errors" },
    { name: "emberdim", hex: "#7a3d2c", use: "the dim end of a danger fill" },
    { name: "parchment", hex: "#e6dcc4", use: "text" },
    { name: "mist", hex: "#8e9688", use: "secondary text" },
    { name: "ink", hex: "#0b0e12", use: "the rail, darker than the page" },
    { name: "ok", hex: "#7fc46a", use: "healthy, saved, running" },
  ],
};

const swatches = hslSwatches(colors);
const wk = literalSwatches(literals);

const kit = `${chromeCSS}
${consoleKit}
/* ---- wildskeeper flourishes ----------------------------------------- */
/* The mock's signature clipped corner. Used sparingly on primary CTAs and
   hero panels — applied globally it stops reading as a deliberate accent. */
.clip-notch { clip-path: polygon(0 0, calc(100% - 10px) 0, 100% 10px, 100% 100%, 10px 100%, 0 calc(100% - 10px)); }
.clip-notch-lg { clip-path: polygon(0 0, calc(100% - 22px) 0, 100% 22px, 100% 100%, 22px 100%, 0 calc(100% - 22px)); }

/* Brass corner brackets on the hero band, from the mock's ::before/::after. */
.wk-corners { position: relative; }
.wk-corners::before, .wk-corners::after {
  content: ""; position: absolute; width: 14px; height: 14px;
  border: 2px solid var(--wk-brasshi); pointer-events: none;
}
.wk-corners::before { top: 6px; left: 6px; border-right: none; border-bottom: none; }
.wk-corners::after { bottom: 6px; right: 6px; border-left: none; border-top: none; }

/* Dragon-eye pulse for the rune sigil; stilled for reduced motion. */
@keyframes wk-pulse { 0%, 100% { opacity: 0.55; } 50% { opacity: 1; } }
.wk-eye { animation: wk-pulse 3.2s ease-in-out infinite; }
@media (prefers-reduced-motion: reduce) { .wk-eye { animation: none; opacity: 0.9; } }

/* The page's atmosphere: a rune wash at top right, a brass one at bottom
   left. Static — a monitoring page should be visually silent at rest. */
.wk-ground {
  background-image:
    radial-gradient(ellipse 900px 500px at 75% -10%, rgba(82, 216, 208, 0.06), transparent),
    radial-gradient(ellipse 700px 600px at -10% 110%, rgba(201, 162, 75, 0.05), transparent);
}
.wk-sigil {
  display: flex; height: 36px; width: 36px; align-items: center; justify-content: center;
  border: 1px solid var(--wk-brass); border-radius: 999px; background: var(--wk-panel);
  font-family: var(--font-display); font-size: 14px; font-weight: 700; color: var(--wk-brasshi);
}
`;

export default {
  app: "wildskeeper",
  title: "Wildskeeper",
  tagline: "the Dragonwilds console",
  intent: `Deep night, brass structure, and <b>one reserved colour</b>. Rune cyan
      (<code>#52d8d0</code>) is never decoration — it marks live and active state and
      nothing else, which is why focus rings are cyan, the active server coin is cyan, and
      a load meter is cyan while a capacity meter is brass. Everything structural is brass;
      everything wrong is ember. Learn those three and the whole console reads at a glance.`,
  fontHref:
    "https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;700&family=Cinzel:wght@500;600;700&family=Alegreya+Sans:ital,wght@0,400;0,500;0,700;1,400&display=swap",
  source: {
    css: "web/wildskeeper/src/index.css",
    tailwind: "web/wildskeeper/tailwind.config.js",
    doc: "docs/wildskeeper-port-verification.md",
  },
  chrome: consoleChrome,
  tokens: {
    colors,
    literals,
    scalars: [{ name: "radius", value: "0.375rem", use: "the mock's tight 6px corners" }],
    colorScheme: "dark",
    derived: {
      "font-display": "Cinzel, serif",
      "font-body": "'Alegreya Sans', system-ui, sans-serif",
      "font-mono": "'JetBrains Mono', ui-monospace, monospace",
      // What the shared console kit needs a theme to name.
      live: "var(--wk-rune)",
      "live-dim": "var(--wk-runedim)",
      warm: "var(--wk-brasshi)",
      "warm-dim": "#6e5a2a",
      ok: "var(--wk-ok)",
      danger: "var(--wk-ember)",
      "meter-track": "var(--wk-ink)",
      "log-bg": "var(--wk-ink)",
      "rail-bg": "var(--wk-ink)",
      "coin-idle-border": "rgba(138, 111, 58, 0.6)",
      "coin-idle-bg": "var(--wk-raise)",
      "coin-idle-fg": "var(--wk-brasshi)",
      "coin-hi": "var(--wk-brasshi)",
      "coin-glow": "0 0 8px rgba(82, 216, 208, 0.45)",
      "brand-mark": "var(--wk-panel)",
      "brand-mark-fg": "var(--wk-brasshi)",
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
          subtitle: "Brass structure, rune live state, ember danger",
          viewport: { width: 940, height: 980 },
          intent: `Two layers. The <b>literal palette</b> (<code>wk.*</code> in the Tailwind
            config) is the theme as designed; the <b>semantic tokens</b> are that palette
            mapped onto shadcn's roles so the components shared with palcon and flametender
            need no per-console work. Reach for a literal only where a spot needs an exact
            hue — the rail's darker ground, a meter's gradient stops — and for everything
            else use the role.`,
          specimens: [
            { stage: "block", caption: "Literal palette · grounds", html: wk(["bg", "ink", "raise", "panel", "edge"]) },
            { stage: "block", caption: "Literal palette · brass is structure", html: wk(["brass", "brasshi"]) },
            { stage: "block", caption: "Literal palette · reserved, and state", html: wk(["rune", "runedim", "ok", "ember", "emberdim"]) },
            { stage: "block", caption: "Literal palette · text", html: wk(["parchment", "mist"]) },
            { stage: "block", caption: "Semantic roles the components actually use", html: swatches(["primary", "secondary", "accent", "destructive", "muted", "ring"]) },
          ],
          rules: [
            "<b>Rune cyan is reserved.</b> If something is cyan, something is live. Never use it to make a panel look nice.",
            "Brass is structure <i>and</i> interaction: the default button, panel titles, the idle server coin. Lit brass (<code>brasshi</code>) is the hover and the warning.",
            "The rail is <code>wk.ink</code>, darker than the page. It is the only surface below the ground.",
            "A load meter fills rune; a capacity meter fills brass. Two different questions, two different colours.",
          ],
          sources: ["web/wildskeeper/src/index.css", "web/wildskeeper/tailwind.config.js"],
        },
        {
          slug: "typography",
          name: "Type",
          subtitle: "Cinzel for identity, Alegreya Sans for prose, JetBrains Mono for facts",
          viewport: { width: 940, height: 760 },
          intent: `Three faces with three jobs. <b>Cinzel</b> is the display face — panel
            titles, stat values, the server coins — and it is budgeted: it appears where the
            page names something, never in a sentence. <b>Alegreya Sans</b> carries everything
            read as prose. <b>JetBrains Mono</b> is for anything a machine produced: log lines,
            ports, ids, byte counts.`,
          specimens: [
            {
              stage: "block",
              caption: "Scale",
              html: [
                typeRow("panel title · Cinzel 13px / 0.08em / caps", `<span class="con-panel-t">Server health</span>`),
                typeRow("stat value · Cinzel 26px", `<span class="con-stat-value">42<small class="con-stat-unit">players</small></span>`),
                typeRow("body · Alegreya Sans 14px", `Everything the console says in a sentence.`),
                typeRow("label · 11px / 0.14em / caps", `<span class="con-stat-label">Memory</span>`),
                typeRow("note · 12px / italic", `<span class="con-note" style="margin:0">Dragonwilds reports no per-player ping.</span>`),
                typeRow("mono · logs and ids", `<span class="con-mono" style="font-size:12px">LogNet: Join succeeded: hazel · 10.0.0.4:7777</span>`),
              ].join("\n"),
            },
          ],
          rules: [
            "Cinzel names things; it never explains them. A paragraph in the display face is the theme breaking.",
            "Webfonts load from Google Fonts with a real fallback stack — the console must stay legible before they arrive.",
            "Log lines are mono and wrap; they are never truncated to a width.",
          ],
          sources: ["web/wildskeeper/tailwind.config.js", "web/wildskeeper/index.html"],
        },
        {
          slug: "flourishes",
          name: "Flourishes",
          subtitle: "Clipped notch, brass brackets, the dragon eye",
          viewport: { width: 940, height: 720 },
          intent: `Three signature shapes, all rationed. The <b>clipped notch</b> is the mock's
            corner cut, on primary CTAs and hero panels only. The <b>brass brackets</b> frame
            a hero band. The <b>dragon-eye pulse</b> is the one moving thing on the page, and
            it stops entirely under <code>prefers-reduced-motion</code> — a monitoring console
            should be visually silent when nothing is happening.`,
          specimens: [
            {
              caption: "Clipped notch — on the primary, and at hero size",
              html: `          ${cbtn("default", "Raise the server", " clip-notch")}\n          ${cbtn("outline", "Not clipped")}`,
            },
            {
              stage: "block",
              caption: "Brass corner brackets on a hero band",
              html: `          <div class="con-panel wk-corners" style="padding:26px 28px">
            <div class="con-stat-label">Dragonwilds</div>
            <div class="con-display" style="font-size:28px;color:hsl(var(--foreground));margin-top:4px">Emberhold</div>
            <div class="con-panel-meta">3 of 8 keepers online · up 4d 6h</div>
          </div>`,
            },
            {
              caption: "The rune sigil, pulsing",
              html: `          <span class="wk-sigil wk-eye" style="color:var(--wk-rune);border-color:var(--wk-rune)">W</span>
          <span class="wk-sigil">W</span>
          <span class="con-panel-meta">lit means live; brass means idle</span>`,
            },
          ],
          rules: [
            "The notch is an accent. Applied to every button it becomes the border radius, and the theme loses its one shape.",
            "Every animation here is stilled under <code>prefers-reduced-motion</code>, not merely slowed.",
            "The page's radial washes are static and low-contrast. Atmosphere must never compete with a status colour.",
          ],
          sources: ["web/wildskeeper/src/index.css", "web/wildskeeper/src/components/wildskeeper/RuneSigil.tsx"],
        },
      ],
    },
    {
      name: "Components",
      cards: [
        {
          slug: "buttons",
          name: "Buttons",
          subtitle: "shadcn's six variants, brass-toned",
          viewport: { width: 940, height: 640 },
          intent: `The shared shadcn button, which is why there are six variants rather than
            reliquary's three: these consoles have admin surfaces with genuinely different
            weights of action. What the theme decides is only which token each variant
            reaches for — <code>default</code> is brass, <code>destructive</code> is ember,
            and focus is rune because focus is live state.`,
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
              caption: "Focus is rune, because focus is live state",
              html: `          <span style="display:inline-flex;box-shadow:0 0 0 1px hsl(var(--ring));border-radius:calc(var(--radius) - 2px)">${cbtn("default", "Start")}</span>
          <span style="display:inline-flex;box-shadow:0 0 0 1px hsl(var(--ring));border-radius:calc(var(--radius) - 2px)">${cbtn("outline", "Logs")}</span>`,
            },
          ],
          rules: [
            "<code>destructive</code> is for verbs that lose something a player cannot get back: delete, ban, wipe. Restart is not destructive.",
            "One <code>default</code> per panel. Everything else is <code>outline</code> or <code>ghost</code>.",
            "A button that a game cannot support is not hidden — the console answers with a 501 that names where the ability actually lives.",
          ],
          sources: ["web/wildskeeper/src/components/ui/button.tsx"],
        },
        {
          slug: "panel",
          name: "Panel and stats",
          subtitle: "The grammar every page is built from",
          viewport: { width: 940, height: 820 },
          intent: `A Cinzel title over a faint brass gradient, an optional right-aligned meta
            line, and a body. Every Dragonwilds page is assembled from these, which is what
            makes the pages read as one system rather than a stack of features. The stat tile
            is the same idea at small scale: a small-caps label, a display-face number, a hint,
            and — only when the number has a ceiling — a meter.`,
          specimens: [
            {
              stage: "block",
              caption: "Panel with meta and a note",
              html: panel(
                "Server log",
                "last 200 lines",
                `<div class="con-log">
              <div class="con-log-line con-log-line--join">[2026.08.22-19.14.02] LogNet: Join succeeded: hazel</div>
              <div class="con-log-line con-log-line--save">[2026.08.22-19.20.11] LogDragonwilds: World saved (2.1 MB)</div>
              <div class="con-log-line con-log-line--warn">[2026.08.22-19.22.40] Warning: tick took 412ms</div>
              <div class="con-log-line con-log-line--error">[2026.08.22-19.23.01] Error: failed to spawn actor</div>
              <div class="con-log-line">[2026.08.22-19.24.00] LogNet: ClientRequestDisconnect</div>
            </div>
            <p class="con-note">Lines are coloured by what they say, not by an invented severity — the game emits no levels.</p>`,
              ),
            },
            {
              stage: "block",
              caption: "Stat tiles — rune for load, brass for capacity",
              html: `          <div class="con-stats">
${stat({ label: "Players", value: "3", unit: "of 8", hint: "peak 6 today", meter: 37, warm: true })}
${stat({ label: "CPU", value: "41", unit: "%", hint: "4 cores allocated", meter: 41 })}
${stat({ label: "Memory", value: "6.2", unit: "GB", hint: "of 12 GB", meter: 52, warm: true })}
${stat({ label: "Uptime", value: "4d", unit: "6h", hint: "since the last restart" })}
          </div>`,
            },
          ],
          rules: [
            "A meter appears only when the number has a ceiling. “Uptime” has no meter because there is nothing to be full of.",
            "Warm (brass) means capacity, cool (rune) means live load. A player counting brass tiles is reading “how much room is left”.",
            "The note is where a capability gap is stated plainly, in the game's terms.",
          ],
          sources: [
            "web/wildskeeper/src/components/wildskeeper/WkPanel.tsx",
            "web/wildskeeper/src/components/ui/card.tsx",
          ],
        },
        {
          slug: "shell",
          name: "Shell and server rail",
          subtitle: "Rune coins, one lit",
          viewport: { width: 1000, height: 640 },
          intent: `The rail is the console's spine: a sigil coin, then one coin per server with
            its initial in the display face, then add and host and sign-out. The active
            server's ring lights <b>rune</b> with a soft glow — the same reserved colour as
            focus and live meters — and every other coin stays brass on the darker ground.`,
          specimens: [
            {
              stage: "block",
              caption: "Desktop shell",
              html: `          <div class="con-shell wk-ground">
            <aside class="con-rail">
              <span class="wk-sigil" title="Wildskeeper">W</span>
              <button class="con-coin con-coin--active">E</button>
              <button class="con-coin con-coin--idle">T</button>
              <button class="con-coin con-coin--idle">H</button>
              <button class="con-coin con-coin--add">+</button>
              <span class="con-rail-spacer"></span>
              <button class="con-coin con-coin--idle" style="height:40px;width:40px;font-size:13px">⌂</button>
            </aside>
            <div class="con-main">
              <div class="con-subnav">
                <a class="con-subnav-item is-active">Overview</a>
                <a class="con-subnav-item">Players</a>
                <a class="con-subnav-item">Activity</a>
                <a class="con-subnav-item">Automation</a>
                <a class="con-subnav-item">Settings</a>
              </div>
              <div class="con-body">
                <div class="con-panel wk-corners" style="padding:22px 24px">
                  <div class="con-stat-label">Dragonwilds</div>
                  <div class="con-display" style="font-size:26px;color:hsl(var(--foreground));margin-top:4px">Emberhold</div>
                  <div class="con-panel-meta"><span class="con-dot con-dot--ok"></span> running · 3 of 8 keepers online</div>
                </div>
                <div class="con-stats">
${stat({ label: "Players", value: "3", unit: "of 8", meter: 37, warm: true })}
${stat({ label: "CPU", value: "41", unit: "%", meter: 41 })}
                </div>
              </div>
            </div>
          </div>`,
            },
          ],
          rules: [
            "One coin is lit. If two look active the rail has stopped answering “which server am I on?”.",
            "Admin-only rail buttons (add server, host) render only for admins — creating a server is an admin endpoint, and offering it to others is a 403 waiting to happen.",
            "The sub-nav is a scrolling pill row on mobile with an edge fade instead of a scrollbar; the half-visible next pill is the affordance.",
          ],
          sources: [
            "web/wildskeeper/src/components/ServerRail.tsx",
            "web/wildskeeper/src/components/wildskeeper/WkServerRune.tsx",
            "web/wildskeeper/src/components/ServerSubNav.tsx",
          ],
        },
        {
          slug: "state",
          name: "State and dialogs",
          subtitle: "Running, stopped, unreachable — and confirming a loss",
          viewport: { width: 940, height: 720 },
          intent: `Four dots and one dialog carry most of what a console has to say. The dot
            vocabulary is fixed: <code>ok</code> green for healthy, rune for live activity,
            mist for stopped, ember for a fault. A destructive dialog names what is lost, and
            its confirm button repeats the verb.`,
          specimens: [
            {
              caption: "Status dots and badges",
              html: `          <span class="con-panel-meta"><span class="con-dot con-dot--ok"></span> running</span>
          <span class="con-panel-meta"><span class="con-dot con-dot--live"></span> saving…</span>
          <span class="con-panel-meta"><span class="con-dot con-dot--down"></span> stopped</span>
          <span class="con-panel-meta"><span class="con-dot con-dot--danger"></span> unreachable</span>
          <span class="con-badge con-badge--outline">ADMIN</span>
          <span class="con-badge con-badge--secondary">v1.4.2</span>
          <span class="con-badge con-badge--destructive">BANNED</span>`,
            },
            {
              stage: "block",
              caption: "Confirming a loss",
              html: `          <div class="con-dialog">
            <h2 class="con-dialog-t">Delete Emberhold?</h2>
            <p class="con-dialog-b">The container, its world save and its logs are removed from the host. This cannot be undone — download a backup first if you want one.</p>
            <div class="con-dialog-f">${cbtn("ghost", "Cancel")}${cbtn("destructive", "Delete server")}</div>
          </div>`,
            },
            {
              stage: "block",
              caption: "Unreachable — what to do, not just what happened",
              html: `          ${panel("Server unreachable", "last seen 4m ago", `<p style="margin:0;font-size:14px">The agent on this host stopped answering. The container may still be running — check the host page, or view the container log for the last thing it said.</p><div style="display:flex;gap:8px;margin-top:14px">${cbtn("outline", "Container log")}${cbtn("outline", "Host")}</div>`)}`,
            },
          ],
          rules: [
            "A failure state offers the next action. “Unreachable” with no buttons is a dead end.",
            "The confirm button repeats the verb and the dialog body names the cost. “Are you sure?” is not a warning.",
            "Ember is for faults, not for warnings — a slow tick is <code>accent</code> brass, a crashed server is ember.",
          ],
          sources: [
            "web/wildskeeper/src/components/ServerUnreachable.tsx",
            "web/wildskeeper/src/components/DeleteServerDialog.tsx",
            "web/wildskeeper/src/components/ServerPower.tsx",
          ],
        },
      ],
    },
  ],
};
