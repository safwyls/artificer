// Reliquary's design system, as data.
//
// Reliquary is the vault: a game-blind custody service for shared world saves.
// Its look was specified before its code — `docs/reliquary-ui-rebuild.md`
// §"Design language" is normative — and this file is that section made
// executable. The palette and primitives live in `../lib/vault.mjs` because
// the Artificer Companion wears the same ones; what is here is what only the
// service has: the shell, the world card, the version row and the login page.
//
// `scripts/checkdesign.sh` fails CI if any token here stops matching
// `web/reliquary/src/index.css`.
//
// The game names in the specimens (Dragonwilds, Enshrouded, Palworld) are
// fixture data, the same way `web/reliquary/src/**/*.test.tsx` names one: the
// vault is game-blind and never branches on which game a world is, but a world
// card with no game on it does not show what the card is for.
// `scripts/checkbounds.sh` enforces that rule on `web/reliquary/src`, which
// this file is not part of.
//
// The specimens are written against the `vk-*` kit, not against the app's
// Tailwind utilities: a preview has to render with no build step, no CDN and
// no framework — see design-system/README.md. The trade is that the kit
// restates what Tailwind composes, so each card names its source files.
import { chromeCSS, icons } from "../lib/chrome.mjs";
import {
  btn,
  derivedSwatches,
  swatches,
  typeRow,
  vaultColors,
  vaultChrome,
  vaultDerived,
  vaultKit,
} from "../lib/vault.mjs";

const ICON = icons("vk-i");

const colors = vaultColors;
const derived = vaultDerived;

/** Chrome, the shared vault primitives, then the compositions only the
 *  service draws. Token-only throughout — no literal color appears outside
 *  the `:root` block the renderer emits above this. */
const kit = `${chromeCSS}
${vaultKit}
/* ---- reliquary compositions ---------------------------------------- */
.vk-card { display: flex; align-items: flex-start; gap: 18px; border: 1px solid rgb(var(--edge)); background: rgb(var(--panel)); border-radius: 8px; padding: 18px 20px; }
.vk-card-main { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 8px; }
.vk-card-titlerow { display: flex; flex-wrap: wrap; align-items: baseline; gap: 10px; }
.vk-card-name { font-size: 18px; font-weight: 700; color: rgb(var(--parchment)); text-decoration: none; }
.vk-card-game { font-size: 12px; color: rgb(var(--rune)); }
.vk-card-head { margin-left: auto; font-family: var(--mono); font-size: 12px; color: rgb(var(--mist)); }
.vk-card-line { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; font-size: 13px; color: rgb(var(--mist)); }
.vk-card-asked { font-family: var(--mono); font-size: 12px; color: rgb(var(--rune)); }
.vk-card-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }

.vk-versionrow { display: flex; flex-wrap: wrap; align-items: center; gap: 14px; padding: 12px 18px; border-bottom: 1px solid rgb(var(--edge)); }
.vk-versionrow:last-child { border-bottom: 0; }
.vk-vid { width: 42px; font-family: var(--mono); font-size: 13px; color: rgb(var(--parchment)); }
.vk-vmeta { font-size: 13px; color: rgb(var(--mist)); }
.vk-vmeta b { font-weight: normal; color: rgb(var(--parchment)); }
.vk-vactions { margin-left: auto; display: flex; gap: 8px; }

.vk-shell { display: flex; border: 1px solid rgb(var(--edge)); border-radius: 8px; overflow: hidden; min-height: 340px; }
.vk-side { width: 224px; flex: none; display: flex; flex-direction: column; background: rgb(var(--well)); border-right: 1px solid rgb(var(--edge)); padding: 20px 0; }
.vk-side-brand { border-bottom: 1px solid rgb(var(--edge)); padding: 0 20px 18px; }
.vk-side-name { font-size: 21px; letter-spacing: 0.06em; color: rgb(var(--gold)); }
.vk-side-tag { margin-top: 2px; font-size: 12px; color: rgb(var(--mist)); }
.vk-nav { display: flex; flex-direction: column; gap: 2px; padding: 14px 10px; }
.vk-navitem {
  display: flex; align-items: center; gap: 10px; border: 1px solid transparent;
  border-radius: 4px; padding: 8px 12px; font-size: 15px; color: rgb(var(--mist)); text-decoration: none;
}
.vk-navitem:hover { color: rgb(var(--parchment)); }
.vk-navitem.is-active { border-color: rgb(var(--edge)); background: rgb(var(--panel)); color: rgb(var(--goldhi)); }
.vk-navitem-admin { margin-left: auto; font-size: 10px; letter-spacing: 0.08em; color: rgb(var(--rune)); }
.vk-side-foot { margin-top: auto; display: flex; flex-direction: column; gap: 6px; border-top: 1px solid rgb(var(--edge)); padding: 14px 20px 0; }
.vk-avatar {
  display: flex; align-items: center; justify-content: center; width: 26px; height: 26px;
  border: 1px solid rgb(var(--gold)); border-radius: 999px; background: rgb(var(--panel));
  font-size: 12px; color: rgb(var(--gold)); flex: none;
}
.vk-side-who { display: flex; align-items: center; gap: 8px; }
.vk-side-name2 { font-size: 13px; }
.vk-side-role { font-size: 11px; color: rgb(var(--mist)); }
.vk-side-live { display: flex; align-items: center; gap: 6px; font-family: var(--mono); font-size: 11px; color: rgb(var(--mist)); }
.vk-pagehead {
  display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between;
  gap: 10px 16px; border-bottom: 1px solid rgb(var(--edge)); padding: 26px 32px 18px;
}
.vk-pagesub { margin-top: 2px; font-size: 13px; color: rgb(var(--mist)); }
.vk-main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
.vk-mainbody { padding: 20px 32px; display: flex; flex-direction: column; gap: 14px; }

.vk-login-ground { display: flex; align-items: center; justify-content: center; padding: 32px; background: var(--fill-login); border-radius: 8px; }
.vk-login { display: flex; width: 100%; max-width: 360px; flex-direction: column; border: 1px solid rgb(var(--edge)); background: rgb(var(--panel)); border-radius: 8px; padding: 30px 32px 26px; }
.vk-login-mark { display: flex; flex-direction: column; align-items: center; gap: 6px; margin-bottom: 14px; }
.vk-login-mark .vk-i { width: 32px; height: 32px; color: rgb(var(--gold)); stroke-width: 1.2; }
.vk-login-name { font-size: 22px; letter-spacing: 0.06em; color: rgb(var(--gold)); }
.vk-login-tag { font-size: 13px; color: rgb(var(--mist)); }
.vk-login-err { margin-top: 12px; font-family: var(--mono); font-size: 12px; color: rgb(var(--ember)); }
.vk-login-hint { margin-top: 12px; font-size: 12px; font-style: italic; color: rgb(var(--mist)); }
.vk-login-hint b { font-style: normal; font-weight: normal; color: rgb(var(--parchment)); }
.vk-login-build { margin-top: 10px; text-align: center; font-family: var(--mono); font-size: 11px; color: rgb(var(--mist)); }
`;

const worldCard = ({ name, game, head, chip, line, asked, primary, quiet }) => `
          <article class="vk-card">
            <div class="vk-cover vk-cover--fallback">${game}</div>
            <div class="vk-card-main">
              <div class="vk-card-titlerow">
                <span class="vk-card-name">${name}</span>
                <span class="vk-card-game">${game}</span>
                <span class="vk-card-head">${head}</span>
              </div>
              <div class="vk-card-line">${chip}<span>${line}</span></div>
              ${asked ? `<div class="vk-card-asked">${asked}</div>` : ""}
              <div class="vk-card-actions">
                ${primary}
                ${quiet.map((q) => btn("quiet", q)).join("\n                ")}
                <button class="vk-btn vk-btn--quiet vk-btn--icon" aria-label="More actions for ${name}">${ICON.more}</button>
              </div>
            </div>
          </article>`;

const CHIP_FREE = `<span class="vk-chip vk-chip--free">${ICON.lockOpen.replace('class="vk-i"', 'class="vk-i vk-i--sm"')}Free</span>`;
const CHIP_HELD = `<span class="vk-chip vk-chip--held">${ICON.lock.replace('class="vk-i"', 'class="vk-i vk-i--sm"')}Held</span>`;
const CHIP_EXPIRED = `<span class="vk-chip vk-chip--expired">${ICON.clock.replace('class="vk-i"', 'class="vk-i vk-i--sm"')}Hold expired</span>`;

const NAV = [
  { icon: ICON.globe, label: "Worlds", active: true },
  { icon: ICON.laptop, label: "Companion" },
  { icon: ICON.users, label: "Users", admin: true },
  { icon: ICON.image, label: "Cover art", admin: true },
  { icon: ICON.database, label: "Save catalogue", admin: true },
];

export default {
  app: "reliquary",
  title: "Reliquary",
  tagline: "the vault of shared worlds",
  intent: `One deliberate dark look — <b>no light theme and no toggle</b>, so there is exactly
      one palette. Reliquary is game-blind custody: it holds other people's worlds and hands
      them back, and the interface is built so a player can answer "can I take this?" from
      across the room. That is what the custody chip, the one-primary-action rule and the
      badge vocabulary are all for.`,
  source: {
    css: "web/reliquary/src/index.css",
    tailwind: "web/reliquary/tailwind.config.js",
    doc: "docs/reliquary-ui-rebuild.md",
  },
  chrome: vaultChrome,
  tokens: { colors, derived, colorScheme: "dark" },
  kit,
  groups: [
    {
      name: "Foundations",
      cards: [
        {
          slug: "colors",
          name: "Palette",
          subtitle: "11 tokens, one theme",
          viewport: { width: 900, height: 900 },
          intent: `Eleven tokens and nothing else. They are declared once in
            <code>index.css</code> and named in <code>tailwind.config.js</code>; no component
            reaches for a literal color except the six compound fills at the bottom, each of
            which is an accent laid over the ground.`,
          specimens: [
            { stage: "block", caption: "Grounds — three depths, darkest on top", html: swatches(["ink", "well", "panel", "edge"]) },
            { stage: "block", caption: "Text", html: swatches(["parchment", "mist"]) },
            { stage: "block", caption: "Accent and state", html: swatches(["gold", "goldhi", "ok", "ember", "rune"]) },
            {
              caption: "Compound fills",
              stage: "block",
              html: `          <div class="vk-swatches">
${Object.entries(derived)
  .filter(([n]) => n !== "mono")
  .map(
    ([n, v]) => `            <div class="vk-swatch">
              <div class="vk-swatch-chip" style="background: var(--${n})"></div>
              <div class="vk-swatch-body">
                <div class="vk-swatch-name">--${n}</div>
                <div class="vk-swatch-hex">${v.length > 34 ? v.slice(0, 32) + "…" : v}</div>
              </div>
            </div>`,
  )
  .join("\n")}
          </div>`,
            },
          ],
          rules: [
            "A new color is a change to this palette, not a one-off hex in a component.",
            "State reads by hue and never by hue alone: free is green <i>and</i> says “Free”, conflict is ember <i>and</i> stamps CONFLICT.",
            "<code>ok</code> / <code>ember</code> / <code>rune</code> are meanings, not decorations — green never means “go”, it means “nobody holds this”.",
          ],
          sources: ["web/reliquary/src/index.css", "web/reliquary/tailwind.config.js"],
        },
        {
          slug: "typography",
          name: "Type",
          subtitle: "Georgia throughout, mono for machine facts",
          viewport: { width: 900, height: 720 },
          intent: `Georgia for everything — the vault's voice — with Gelasio as the
            metric-compatible webfont for machines without it. Monospace is reserved for
            facts a machine produced: ids, byte counts, timestamps, tokens, build strings.
            If it is mono, you can copy it into a bug report and it will still mean something.`,
          specimens: [
            {
              stage: "block",
              caption: "Scale",
              html: [
                typeRow("h1 · 22px / 0.05em / gold", `<span class="vk-h1">Worlds</span>`),
                typeRow("card title · 18px / bold", `<span class="vk-card-name">Ashwood Hollow</span>`),
                typeRow("body · 15px", `Everything the app says in a sentence.`),
                typeRow("secondary · 13px / mist", `<span style="font-size:13px;color:rgb(var(--mist))">held by hazel until 18:40 — claimable</span>`),
                typeRow("field label · 11px / 0.1em / caps", `<span class="vk-label">Retention</span>`),
                typeRow("table head · 10px / 0.12em / caps", `<span class="vk-tablehead" style="border:0;padding:0">User · Role · Custody</span>`),
                typeRow("mono · ids, sizes, times", `<span class="vk-mono" style="font-size:12px;color:rgb(var(--mist))">head v41 · 1.8 GB · 2026-08-21 18:02</span>`),
                typeRow("mono · badges", `<span class="vk-badge vk-badge--head">HEAD</span>`),
              ].join("\n"),
            },
          ],
          rules: [
            "Panel headings are uppercase, letterspaced and gold — one heading treatment, everywhere a panel names itself.",
            "Labels are uppercased by the words the caller writes, not by <code>text-transform</code>, so a test can find a label by what it says.",
            "No emoji anywhere. Icons are lucide, stroke style.",
          ],
          sources: ["web/reliquary/src/index.css", "web/reliquary/src/components/ui/label.tsx"],
        },
        {
          slug: "surfaces",
          name: "Surfaces",
          subtitle: "Panel, well, dashed well, 8px corner",
          viewport: { width: 900, height: 620 },
          intent: `Three grounds stacked darkest-to-lightest: <code>ink</code> is the page,
            <code>well</code> sits under it for sidebars and inset strips, <code>panel</code>
            rises above it for anything you can act on. A dashed <code>edge</code> on a well
            means "this is a form or an aside, not a record".`,
          specimens: [
            {
              stage: "stack",
              caption: "Solid panel — a record you act on",
              html: `          <div class="vk-panel">A world, a version, a user: something the vault holds.</div>`,
            },
            {
              stage: "stack",
              caption: "Dashed well — a form, a hint, an empty state",
              html: `          <div class="vk-well">No worlds yet. Create one, or point the companion at a save you already have.</div>`,
            },
            {
              stage: "stack",
              caption: "Table shell — a panel with a header strip",
              html: `          <div class="vk-tableshell">
            <div class="vk-tablehead">User · Role · Custody</div>
            <div class="vk-tablerow"><span>hazel</span><span class="vk-badge vk-badge--muted">ADMIN</span><span style="color:rgb(var(--mist));font-size:13px">may check worlds out</span></div>
            <div class="vk-tablerow"><span>rook</span><span class="vk-badge vk-badge--muted">USER</span><span style="color:rgb(var(--mist));font-size:13px">read-only</span></div>
          </div>`,
            },
          ],
          rules: [
            "Corner radius is 8px for panels (<code>rounded-panel</code>) and 4px for controls. Chips are fully round; badges are 3px.",
            "Depth is the ground color, not a shadow. Only overlays — dialog, menu, toast — carry a shadow.",
            "A table is drawn as a panel with grid rows, not a <code>&lt;table&gt;</code>: every column is a word or a row of buttons, and grid keeps them aligned without colspan arithmetic.",
          ],
          sources: [
            "web/reliquary/src/components/ui/table.tsx",
            "web/reliquary/tailwind.config.js",
          ],
        },
        {
          slug: "focus",
          name: "Focus",
          subtitle: "One gold ring, offset from the page ground",
          viewport: { width: 900, height: 420 },
          intent: `Focus is visible in exactly one style across the app — a 2px gold ring at
            60% offset from <code>ink</code>, the same accent the primary button carries.
            Declared once on <code>:focus-visible</code> in the base layer, never re-styled
            per component.`,
          specimens: [
            {
              caption: "As it renders (the ring is drawn here, not focused)",
              html: `          <span class="vk-focus-demo" style="box-shadow: 0 0 0 2px rgb(var(--ink)), 0 0 0 4px rgb(var(--gold) / 0.6); border-radius: 4px">${btn("primary", "Check out")}</span>
          <span class="vk-focus-demo" style="box-shadow: 0 0 0 2px rgb(var(--ink)), 0 0 0 4px rgb(var(--gold) / 0.6); border-radius: 4px">${btn("quiet", "Download head")}</span>`,
            },
            {
              caption: "Tab into these to see the real thing",
              html: `          ${btn("primary", "Check out")}\n          ${btn("quiet", "Download head")}\n          <input class="vk-input" style="width:220px" placeholder="world name" />`,
            },
          ],
          rules: [
            "<code>color-scheme: dark</code> is set on <code>:root</code> — without it a dark page still gets light scrollbars and light form widgets.",
            "Never remove the ring without replacing it; the offset exists so it reads on both <code>panel</code> and <code>ink</code>.",
          ],
          sources: ["web/reliquary/src/index.css"],
        },
      ],
    },
    {
      name: "Actions",
      cards: [
        {
          slug: "button",
          name: "Buttons",
          subtitle: "Primary / quiet / danger, four sizes",
          viewport: { width: 900, height: 760 },
          intent: `Three buttons, and only three. <b>Primary</b> is gold outline on a dark gold
            gradient and there is <i>one per custody state</i> — never several competing for
            the same decision. <b>Quiet</b> is mist on an edge border, for everything else.
            <b>Danger</b> is quiet that turns ember when you reach for it, so a destructive
            verb costs nothing to look at and announces itself the moment you aim.`,
          specimens: [
            {
              caption: "Variants · resting",
              html: `          ${btn("primary", "Check out")}\n          ${btn("quiet", "Download head")}\n          ${btn("danger", "Force release")}`,
            },
            {
              caption: "Variants · hover",
              html: `          ${btn("primary", "Check out", " is-hover")}\n          ${btn("quiet", "Download head", " is-hover")}\n          ${btn("danger", "Force release", " is-hover")}`,
            },
            {
              caption: "Sizes · sm 12px · default 13px · lg 14px · icon",
              html: `          ${btn("quiet", "Make head", " vk-btn--sm")}\n          ${btn("quiet", "Download head")}\n          ${btn("primary", "Sign in", " vk-btn--lg")}\n          <button class="vk-btn vk-btn--quiet vk-btn--icon" aria-label="More actions">${ICON.more}</button>`,
            },
            {
              caption: "Disabled",
              html: `          ${btn("primary", "Signing in…", " is-disabled")}\n          ${btn("quiet", "Check in…", " is-disabled")}`,
            },
            {
              caption: "One custody state, one primary — the shape every card takes",
              html: `          ${btn("primary", "Check out")}\n          ${btn("quiet", "Download head")}\n          ${btn("quiet", "History")}\n          <button class="vk-btn vk-btn--quiet vk-btn--icon" aria-label="More actions">${ICON.more}</button>`,
            },
          ],
          rules: [
            "Never two primaries in one action row. If a second verb feels primary, the custody state is being drawn wrong.",
            "A download is a real <code>&lt;a&gt;</code> (the browser must stream it); an in-app destination is a router link. Both wear the button, via <code>asChild</code>.",
            "Danger is never the primary <i>and</i> never hidden: it lives in the quiet row or the overflow menu, and it always confirms.",
            "Labels are verbs in the vault's own words — “Check out”, “Make head”, “Force release” — never “OK”, never “Submit”.",
          ],
          sources: ["web/reliquary/src/components/ui/button.tsx", "web/reliquary/src/components/WorldActions.tsx"],
        },
        {
          slug: "overflow-menu",
          name: "Overflow menu",
          subtitle: "The rare and admin verbs, one click away",
          viewport: { width: 900, height: 560 },
          intent: `Everything a world can do that the custody state is not asking for. The verbs
            stay reachable and stop competing with the one action that matters — which is the
            whole reason the card has room for a single primary.`,
          specimens: [
            {
              stage: "block",
              caption: "Trigger and open menu",
              html: `          <div style="display:flex;gap:16px;align-items:flex-start">
            <button class="vk-btn vk-btn--quiet vk-btn--icon" aria-label="More actions for Ashwood Hollow">${ICON.more}</button>
            <div class="vk-menu">
              <div class="vk-menuitem">Host on the dedicated server</div>
              <div class="vk-menuitem is-highlighted">Take back from the server</div>
              <div class="vk-menuitem">Import a save…</div>
              <div class="vk-menusep"></div>
              <div class="vk-menuitem vk-menuitem--danger">Force release</div>
              <div class="vk-menuitem vk-menuitem--danger is-highlighted">Delete world</div>
            </div>
          </div>`,
            },
          ],
          rules: [
            "Admin-only items are rendered only for admins — an item that answers “403” is worse than no item.",
            "Danger items highlight to ember on a 10% ember wash, not to a filled red row.",
            "A separator precedes the destructive block; nothing follows it.",
          ],
          sources: ["web/reliquary/src/components/ui/menu.tsx", "web/reliquary/src/lib/worldActions.ts"],
        },
      ],
    },
    {
      name: "Forms",
      cards: [
        {
          slug: "fields",
          name: "Fields",
          subtitle: "Label above, input on ink",
          viewport: { width: 900, height: 620 },
          intent: `One field shape. The label is small caps in mist and always sits above its
            input — never beside it, never as a placeholder. The input's ground is
            <code>ink</code>, one shade below the panel it sits on, so a field reads as a hole
            in the surface rather than a raised control.`,
          specimens: [
            {
              stage: "block",
              caption: "A field",
              html: `          <div style="max-width:320px">
            <label class="vk-label" for="w">World name</label>
            <input class="vk-input" id="w" style="margin-top:4px" value="Ashwood Hollow" />
          </div>`,
            },
            {
              stage: "block",
              caption: "An inline admin form — dashed well, fields, one primary",
              html: `          <form class="vk-well" style="display:flex;flex-wrap:wrap;align-items:flex-end;gap:12px">
            <div style="width:180px"><label class="vk-label" for="u">Username</label><input class="vk-input" id="u" style="margin-top:4px" placeholder="rook" /></div>
            <div style="width:140px"><label class="vk-label" for="r">Role</label><input class="vk-input" id="r" style="margin-top:4px" placeholder="user" /></div>
            ${btn("primary", "Add user")}
          </form>`,
            },
            {
              stage: "block",
              caption: "An error — mono, ember, under the field it belongs to",
              html: `          <div style="max-width:320px">
            <label class="vk-label" for="p">Password</label>
            <input class="vk-input" id="p" type="password" style="margin-top:4px" value="wrong" />
            <div class="vk-login-err">401 — that username and password do not match</div>
          </div>`,
            },
          ],
          rules: [
            "Errors quote the server's own words. A message the operator can grep for beats a message that reads nicely.",
            "Placeholders are examples, never labels.",
            "The whole form is a dashed well when it is an aside on a page that is mostly records.",
          ],
          sources: ["web/reliquary/src/components/ui/input.tsx", "web/reliquary/src/components/ui/label.tsx"],
        },
        {
          slug: "dialog",
          name: "Confirm dialog",
          subtitle: "What the action costs, not that it is irreversible",
          viewport: { width: 900, height: 620 },
          intent: `Replaces <code>window.confirm</code> for every verb that used to sit behind
            one. The body says what the action <i>costs</i> — “Anything they have not sent is
            left on their machine” — because “are you sure?” tells a player nothing they did
            not already know.`,
          specimens: [
            {
              stage: "block",
              caption: "Confirming a normal verb",
              html: `          <div class="vk-dialog">
            <button class="vk-dialog-x" aria-label="Close">${ICON.close}</button>
            <h2 class="vk-dialog-title">Make v37 the canonical head?</h2>
            <p class="vk-dialog-body">The next player to check this world out gets this version.</p>
            <div class="vk-dialog-foot">${btn("quiet", "Cancel")}${btn("primary", "Make head")}</div>
          </div>`,
            },
            {
              stage: "block",
              caption: "Confirming a destructive one",
              html: `          <div class="vk-dialog">
            <button class="vk-dialog-x" aria-label="Close">${ICON.close}</button>
            <h2 class="vk-dialog-title">Force release hazel's hold?</h2>
            <p class="vk-dialog-body">Anything they have not sent is left on their machine. Their late check-in will be kept and flagged as a conflict, not lost.</p>
            <div class="vk-dialog-foot">${btn("quiet", "Cancel")}${btn("danger", "Force release")}</div>
          </div>`,
            },
          ],
          rules: [
            "The confirm button repeats the verb (“Force release”), never “OK”.",
            "Cancel is quiet and comes first; the committing button is last and on the right.",
            "The trigger is whatever the caller drew — a quiet button, a menu item — so the dialog never dictates how the verb looks.",
          ],
          sources: ["web/reliquary/src/components/ConfirmDialog.tsx", "web/reliquary/src/components/ui/dialog.tsx"],
        },
      ],
    },
    {
      name: "Status",
      cards: [
        {
          slug: "custody-chip",
          name: "Custody chip",
          subtitle: "Free / Held / Hold expired",
          viewport: { width: 900, height: 640 },
          intent: `The one thing a player needs from across the room: can I take this world?
            The chip and the card's primary action both read <code>custodyOf(status)</code>,
            so the two can never disagree — the chip <i>is</i> the reason the button says what
            it says.`,
          specimens: [
            { caption: "The three states", html: `          ${CHIP_FREE}\n          ${CHIP_HELD}\n          ${CHIP_EXPIRED}` },
            {
              stage: "stack",
              caption: "Each with the sentence beside it, and the action it drives",
              html: `          <div class="vk-card-line">${CHIP_FREE}<span>nobody holds this world · next claim: rook</span>${btn("primary", "Check out")}</div>
          <div class="vk-card-line">${CHIP_HELD}<span>held by hazel until 18:40</span>${btn("quiet", "Ask for it back")}</div>
          <div class="vk-card-line">${CHIP_EXPIRED}<span>held by hazel (on the dedicated server) — claimable</span>${btn("primary", "Claim it")}</div>`,
            },
            {
              stage: "block",
              caption: "A request nobody can see the state of is a request nobody trusts",
              html: `          <div class="vk-card-asked">waiting for hazel's companion to check in and release · asked 18:12 — it answers within a minute of being online; if their machine is asleep the request stands until it wakes.</div>`,
            },
          ],
          rules: [
            "Green is not “go” — it is “nobody holds this”. Gold is not “warning” — it is “someone does”.",
            "Every chip carries both an icon and a word; neither alone.",
            "“Hold expired” is ember but not an error: the world is claimable, and the holder's late check-in is still accepted and flagged.",
          ],
          sources: ["web/reliquary/src/components/CustodyChip.tsx", "web/reliquary/src/lib/types.ts"],
        },
        {
          slug: "badge",
          name: "Version badges",
          subtitle: "HEAD / CONFLICT / CHECKPOINT",
          viewport: { width: 900, height: 520 },
          intent: `A monospace stamp on the page ground, colored by what it <i>means</i> rather
            than by rank. HEAD is what the next player gets. CONFLICT is a check-in from a hold
            that could no longer move the head — accepted and flagged rather than lost, waiting
            for an admin to pick a head. CHECKPOINT is a mid-session snapshot, which never moved
            the head in the first place.`,
          specimens: [
            {
              caption: "The vocabulary",
              html: `          <span class="vk-badge vk-badge--head">HEAD</span>
          <span class="vk-badge vk-badge--conflict">CONFLICT</span>
          <span class="vk-badge vk-badge--checkpoint">CHECKPOINT</span>
          <span class="vk-badge vk-badge--muted">ADMIN</span>`,
            },
          ],
          rules: [
            "Badges never carry an action. They say what a row <i>is</i>; the buttons on the right say what you can do about it.",
            "CONFLICT always appears with the sentence that explains it — the badge alone is a puzzle.",
            "Muted is for classification with no state attached (a user's role), not for a fourth kind of version.",
          ],
          sources: ["web/reliquary/src/components/ui/badge.tsx", "web/reliquary/src/components/VersionRow.tsx"],
        },
        {
          slug: "feedback",
          name: "Toasts and the live dot",
          subtitle: "Every action says what happened, or why it didn't",
          viewport: { width: 900, height: 560 },
          intent: `The old page's one-line status readout, as toasts in the bottom-right. Every
            action says what happened — “hold extended”, “asked — their companion answers within
            a minute” — or why it didn't, in the server's own words. The live dot in the sidebar
            footer says whether the custody stream is still open; a stale page that looks fresh
            is the failure mode this exists to prevent.`,
          specimens: [
            {
              stage: "stack",
              caption: "Toasts",
              html: `          <div class="vk-toast vk-toast--success"><span>Checked out Ashwood Hollow</span><span class="vk-toast-desc">held by you until 21:14 · your companion has the save</span></div>
          <div class="vk-toast vk-toast--error"><span>Could not force release</span><span class="vk-toast-desc">409 — the hold was already released</span></div>
          <div class="vk-toast"><span>Asked hazel to check in</span><span class="vk-toast-desc">their companion answers within a minute of being online</span></div>`,
            },
            {
              caption: "Live dot",
              html: `          <span class="vk-side-live"><span class="vk-dot vk-dot--live"></span>live · reliquary v1.9.2</span>
          <span class="vk-side-live"><span class="vk-dot vk-dot--down"></span>reconnecting… · reliquary v1.9.2</span>`,
            },
          ],
          rules: [
            "A failed action shows the server's status and message verbatim. Do not rewrite a 409 into “something went wrong”.",
            "The build string rides beside the live dot on every screen including login: a bug report about save sync should be able to name the build without anyone opening a container.",
            "One stream for the whole app, opened by the shell — a page that mounts and unmounts must not take the connection with it.",
          ],
          sources: ["web/reliquary/src/components/ui/toaster.tsx", "web/reliquary/src/lib/live.ts", "web/reliquary/src/components/AppShell.tsx"],
        },
      ],
    },
    {
      name: "Patterns",
      cards: [
        {
          slug: "cover-art",
          name: "Cover art",
          subtitle: "The image, and the tile for when there isn't one",
          viewport: { width: 900, height: 480 },
          intent: `A world's cover is the same art the companion's shelf shows, so the two views
            of one world look like one world. With no cover — IGDB unconfigured, a game it does
            not know, a lookup that failed — it falls back to the game's name in a gradient
            tile: never a broken image, never an error.`,
          specimens: [
            {
              caption: "Card size (84×112) and detail size (96×128), both fallbacks",
              html: `          <div class="vk-cover vk-cover--fallback">Dragonwilds</div>
          <div class="vk-cover vk-cover--fallback vk-cover--detail">Enshrouded</div>
          <div class="vk-cover vk-cover--fallback" style="width:64px;height:85px">Palworld</div>`,
            },
          ],
          rules: [
            "The fallback shows the game title, falling back to the world name — never a placeholder glyph.",
            "An <code>onError</code> swaps a broken image for the tile; the player never sees a torn-image icon.",
            "Covers are lazy-loaded and the <code>alt</code> is empty: the world's name is already beside it, so the image is decoration.",
          ],
          sources: ["web/reliquary/src/components/CoverArt.tsx", "web/reliquary/src/lib/art.ts"],
        },
        {
          slug: "world-card",
          name: "World card",
          subtitle: "Cover, custody, and the single action custody calls for",
          viewport: { width: 940, height: 780 },
          intent: `One world on the shelf. The cover names it, the chip says whether you can take
            it, and exactly one primary button does the thing the chip implies. Everything rarer
            is a click away — the world's own page for history and settings, the overflow menu
            for the admin verbs.`,
          specimens: [
            {
              stage: "stack",
              caption: "Free — the primary is “Check out”",
              html: worldCard({
                name: "Ashwood Hollow",
                game: "Dragonwilds",
                head: "head v41 · 1.8 GB · 18:02",
                chip: CHIP_FREE,
                line: "nobody holds this world",
                primary: btn("primary", "Check out"),
                quiet: ["Download head", "History"],
              }),
            },
            {
              stage: "stack",
              caption: "Held by someone else — the primary becomes the ask, and the request is visible",
              html: worldCard({
                name: "Cinderfall",
                game: "Enshrouded",
                head: "head v12 · 640 MB · yesterday",
                chip: CHIP_HELD,
                line: "held by hazel until 18:40 · next claim: rook",
                asked: "waiting for hazel's companion to check in and release · asked 18:12",
                primary: btn("quiet", "Ask for it back"),
                quiet: ["Download head", "History"],
              }),
            },
            {
              stage: "stack",
              caption: "Hold expired — claimable, and the primary says so",
              html: worldCard({
                name: "Verdant Reach",
                game: "Palworld",
                head: "head v7 · 210 MB · 3 days ago",
                chip: CHIP_EXPIRED,
                line: "held by rook (on the dedicated server) — claimable",
                primary: btn("primary", "Claim it"),
                quiet: ["Download head", "History"],
              }),
            },
          ],
          rules: [
            "The chip and the primary are computed from the same <code>custodyOf()</code> call. They cannot disagree.",
            "The head line is mono and right-aligned: version, size, time — the three facts an operator asks for first.",
            "Cover and title both link to the world's page; the card itself is not a link, because it contains buttons.",
          ],
          sources: [
            "web/reliquary/src/components/WorldCard.tsx",
            "web/reliquary/src/lib/worldActions.ts",
            "web/reliquary/src/components/WorldActions.tsx",
          ],
        },
        {
          slug: "version-row",
          name: "Version row",
          subtitle: "One kept version, badges first",
          viewport: { width: 940, height: 520 },
          intent: `The history list. The badges are the whole point of the row — they are how an
            admin finds the head, spots the conflict, and ignores the checkpoints — so they sit
            immediately after the id and before the prose.`,
          specimens: [
            {
              stage: "block",
              caption: "A world's history",
              html: `          <div class="vk-tableshell">
            <div class="vk-versionrow">
              <span class="vk-vid">v41</span>
              <span style="display:flex;gap:6px"><span class="vk-badge vk-badge--head">HEAD</span></span>
              <span class="vk-vmeta">checkin by <b>hazel</b> · 1.8 GB · 2026-08-21 18:02</span>
              <span class="vk-vactions">${btn("quiet", "Download", " vk-btn--sm")}</span>
            </div>
            <div class="vk-versionrow">
              <span class="vk-vid">v40</span>
              <span style="display:flex;gap:6px"><span class="vk-badge vk-badge--conflict">CONFLICT</span></span>
              <span class="vk-vmeta">checkin by <b>rook</b> · 1.8 GB · 2026-08-21 17:55 — from a hold that could no longer move the head</span>
              <span class="vk-vactions">${btn("quiet", "Download", " vk-btn--sm")}${btn("quiet", "Make head", " vk-btn--sm")}</span>
            </div>
            <div class="vk-versionrow">
              <span class="vk-vid">v39</span>
              <span style="display:flex;gap:6px"><span class="vk-badge vk-badge--checkpoint">CHECKPOINT</span></span>
              <span class="vk-vmeta">checkpoint by <b>hazel</b> · 1.7 GB · 2026-08-21 16:30</span>
              <span class="vk-vactions">${btn("quiet", "Download", " vk-btn--sm")}${btn("quiet", "Make head", " vk-btn--sm")}</span>
            </div>
          </div>`,
            },
          ],
          rules: [
            "“Make head” is offered only to a user who may set one, and never on the row that already is the head.",
            "Download is a real link so the browser streams the tar; it wears the quiet button via <code>asChild</code>.",
            "Retention and total storage sit under the list, not in it.",
          ],
          sources: ["web/reliquary/src/components/VersionRow.tsx"],
        },
        {
          slug: "app-shell",
          name: "App shell",
          subtitle: "Sidebar, page header, footer identity",
          viewport: { width: 1000, height: 620 },
          intent: `The shell every signed-in page sits in. The sidebar names what this deployment
            <i>has</i>; the footer names who you are and whether the vault is still talking.
            Admin destinations are rendered only for admins — a nav item that answers
            “Failed to load users” is worse than no item. On mobile the same job is done by two
            slim pinned rows: identity and sign-out, then the nav as one scrollable pill row.`,
          specimens: [
            {
              stage: "block",
              caption: "Desktop shell",
              html: `          <div class="vk-shell">
            <nav class="vk-side">
              <div class="vk-side-brand">
                <div class="vk-side-name">Reliquary</div>
                <div class="vk-side-tag">the vault of shared worlds</div>
              </div>
              <div class="vk-nav">
${NAV.map(
  (n) =>
    `                <a class="vk-navitem${n.active ? " is-active" : ""}" href="#">${n.icon.replace(
      'class="vk-i"',
      'class="vk-i vk-i--lg"',
    )}<span>${n.label}</span>${n.admin ? `<span class="vk-navitem-admin">ADMIN</span>` : ""}</a>`,
).join("\n")}
              </div>
              <div class="vk-side-foot">
                <div class="vk-side-who">
                  <div class="vk-avatar">H</div>
                  <div>
                    <div class="vk-side-name2">hazel</div>
                    <div class="vk-side-role">admin · sign out</div>
                  </div>
                </div>
                <div class="vk-side-live"><span class="vk-dot vk-dot--live"></span>live · reliquary v1.9.2</div>
              </div>
            </nav>
            <div class="vk-main">
              <div class="vk-pagehead">
                <div>
                  <h1 class="vk-h1">Worlds</h1>
                  <div class="vk-pagesub">Every world the vault holds, and who has it right now.</div>
                </div>
                ${btn("primary", "New world")}
              </div>
              <div class="vk-mainbody">
                <div class="vk-well">You have no custody rights on this deployment — you can download heads and read history, but not check a world out.</div>
                <div class="vk-panel" style="color:rgb(var(--mist));font-size:13px">…world cards…</div>
              </div>
            </div>
          </div>`,
            },
          ],
          rules: [
            "The page header is a gold title, a line saying what the screen is for, and room on the right for its one primary action.",
            "The read-only notice is a banner on Worlds, not a disabled button on every card.",
            "The live dot and the build ride in the footer on desktop and beside the wordmark on mobile — both pinned, because both are useless once scrolled away.",
          ],
          sources: ["web/reliquary/src/components/AppShell.tsx"],
        },
        {
          slug: "login",
          name: "Login",
          subtitle: "Password box, and three distinguishable SSO hints",
          viewport: { width: 900, height: 760 },
          intent: `The one screen outside the shell. Its SSO behavior is load-bearing: try
            <code>/me</code>, then <code>/login/cloudflare</code>, and show <i>three
            distinguishable</i> hints — not configured, no assertion, other error — because
            "sign-in failed" sends an operator to the wrong half of the stack.`,
          specimens: [
            {
              stage: "block",
              caption: "Resting",
              html: `          <div class="vk-login-ground">
            <form class="vk-login">
              <div class="vk-login-mark">${ICON.shield}<div class="vk-login-name">Reliquary</div><div class="vk-login-tag">Sign in to the vault.</div></div>
              <label class="vk-label" for="lu" style="margin-top:6px">Username</label>
              <input class="vk-input" id="lu" style="margin-top:4px" />
              <label class="vk-label" for="lp" style="margin-top:10px">Password</label>
              <input class="vk-input" id="lp" type="password" style="margin-top:4px" />
              ${btn("primary", "Sign in", " vk-btn--lg").replace('class="vk-btn', 'style="margin-top:16px" class="vk-btn')}
              <div class="vk-login-build">reliquary v1.9.2</div>
            </form>
          </div>`,
            },
            {
              stage: "block",
              caption: "The three SSO hints",
              html: `          <div class="vk-login-hint">Cloudflare Access sign-in is <b>not configured on this server</b>. Even behind the tunnel, this deployment wants a username and password.</div>
          <div class="vk-login-hint">Cloudflare Access is configured, but <b>this request carried no assertion</b> — you are reaching the server directly rather than through the tunnel.</div>
          <div class="vk-login-err">502 — Cloudflare Access replied, but the vault could not verify the assertion</div>`,
            },
          ],
          rules: [
            "The build string appears here too, before anyone is signed in.",
            "The password error is the server's status and message, in mono, under the field.",
            "The ground is a radial gradient from <code>panel</code> to <code>ink</code> — the only gradient background in the app.",
          ],
          sources: ["web/reliquary/src/pages/Login.tsx"],
        },
      ],
    },
  ],
};
