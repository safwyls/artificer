// The Artificer Companion's design system, as data.
//
// The companion is the player-side half of save sync: it discovers installed
// games, links their save folders to worlds in the vault, and moves the saves.
// It shares reliquary's palette and primitives verbatim — one system, two
// halves — so the tokens and the `vk-*` kit come from `../lib/vault.mjs` and
// this file holds only what the player-side app draws: the custody groups and
// their world rows, the games library, the window chrome that says whether the
// vault is reachable, and the diagnostics that say where the scan looked.
//
// The three consoles are a different design language. The companion is not.
//
// `docs/companion.md` and `docs/companion-ui-rebuild.md` are the behavior and
// the plan; `docs/reliquary-ui-rebuild.md` §"Design language" is normative for
// the look. `scripts/checkdesign.sh` fails CI if any token here stops matching
// `web/companion/src/index.css`.
//
// Game names in the specimens are fixture data: the companion is game-blind
// (`scripts/checkbounds.sh` enforces that on `web/companion/src`, which this
// file is not part of) but a shelf with no games on it does not show what a
// shelf is.
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
const sm = (glyph) => glyph.replace('class="vk-i"', 'class="vk-i vk-i--sm"');

const kit = `${chromeCSS}
${vaultKit}
/* ---- companion compositions ----------------------------------------- */
.vk-header {
  display: flex; flex-wrap: wrap; align-items: center; gap: 14px;
  border-bottom: 1px solid rgb(var(--edge)); background: rgb(var(--well)); padding: 16px 28px;
}
.vk-header .vk-i--xl { width: 24px; height: 24px; color: rgb(var(--gold)); stroke-width: 1.3; }
.vk-header-name { font-size: 18px; letter-spacing: 0.05em; color: rgb(var(--gold)); }
.vk-header-tag { font-size: 11px; color: rgb(var(--mist)); }
.vk-header-right { margin-left: auto; display: flex; flex-wrap: wrap; align-items: center; gap: 12px; }
.vk-conn { display: inline-flex; align-items: center; gap: 6px; font-size: 13px; }
.vk-conn b { margin-left: 4px; font-weight: 700; }
.vk-header-err { width: 100%; font-size: 13px; color: rgb(var(--ember)); }
.vk-footer {
  display: flex; flex-wrap: wrap; align-items: center; gap: 10px;
  border-top: 1px solid rgb(var(--edge)); padding: 10px 28px;
  font-family: var(--mono); font-size: 11px; color: rgb(var(--mist));
}
.vk-footer-last { margin-left: auto; }

.vk-section { display: flex; flex-wrap: wrap; align-items: baseline; gap: 10px; }
.vk-section-h { margin: 0; font-size: 12px; font-weight: normal; text-transform: uppercase; letter-spacing: 0.12em; color: rgb(var(--gold)); }
.vk-section-hint { font-size: 12px; font-style: italic; color: rgb(var(--mist)); }
.vk-section-acts { margin-left: auto; display: flex; gap: 8px; }

.vk-shelf { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 14px; }
.vk-tile {
  display: flex; flex-direction: column; overflow: hidden; border-radius: 6px;
  border: 1px solid; text-align: left; cursor: pointer; padding: 0;
  font-family: inherit; color: inherit;
  transition: opacity 0.15s, border-color 0.15s;
}
.vk-tile--linked { border-color: rgb(var(--gold) / 0.5); background: rgb(var(--panel)); }
.vk-tile--unlinked { border-color: rgb(var(--edge)); background: rgb(var(--well)); opacity: 0.72; }
.vk-tile--unlinked:hover { opacity: 1; border-color: rgb(var(--gold) / 0.5); }
.vk-tile--active { border-color: rgb(var(--goldhi)); opacity: 1; }
.vk-tile-art {
  width: 100%; height: 132px; display: flex; align-items: center; justify-content: center;
  background: var(--fill-cover); padding: 10px; text-align: center; font-size: 12px;
  line-height: 1.2; color: rgb(var(--mist));
}
.vk-tile-cap { display: flex; align-items: center; gap: 8px; padding: 8px 10px 10px; }
.vk-tile-capmain { min-width: 0; flex: 1; }
/* Two lines, no ellipsis: a truncated title is not a title. */
.vk-tile-name { font-size: 13px; line-height: 1.25; color: rgb(var(--parchment)); }
.vk-tile-note { margin-top: 3px; font-size: 11.5px; line-height: 1.2; color: rgb(var(--mist)); }
.vk-tile-note--linked { color: rgb(var(--goldhi)); }
.vk-tile-link { white-space: nowrap; font-size: 11.5px; color: rgb(var(--goldhi)); }

/* ---- the window's tab bar -------------------------------------------- */
.vk-tabs { display: flex; gap: 4px; border-bottom: 1px solid rgb(var(--edge)); padding: 16px 28px 0; }
.vk-tab {
  display: flex; align-items: center; gap: 8px; margin-bottom: -1px;
  border: 0; border-bottom: 2px solid transparent; background: none; cursor: pointer;
  padding: 8px 14px 10px; font-family: inherit; font-size: 14.5px; color: rgb(var(--mist));
  transition: color 0.12s ease, border-color 0.12s ease;
}
.vk-tab:hover { color: rgb(var(--parchment)); }
.vk-tab--on { border-bottom-color: rgb(var(--gold)); color: rgb(var(--goldhi)); }
.vk-tab-badge {
  border: 1px solid rgb(var(--ember) / 0.5); border-radius: 3px; padding: 1px 6px;
  font-family: var(--mono); font-size: 10px; letter-spacing: 0.06em; color: rgb(var(--ember));
}

/* ---- a custody group ------------------------------------------------- */
.vk-group { display: flex; flex-direction: column; gap: 8px; }
.vk-group-head { display: flex; flex-wrap: wrap; align-items: baseline; gap: 12px; }
.vk-group-label { font-family: var(--mono); font-size: 10px; text-transform: uppercase; letter-spacing: 0.12em; color: rgb(var(--mist)); }
.vk-group-label--gold { color: rgb(var(--gold)); }
.vk-group-sub { font-size: 12.5px; color: rgb(var(--mist)); }
.vk-group-body { overflow: hidden; border: 1px solid rgb(var(--edge)); background: rgb(var(--panel)); border-radius: 8px; }
.vk-group-body--gold { border-color: rgb(var(--gold) / 0.45); }
.vk-group-body > * + * { border-top: 1px solid rgb(var(--edge)); }

/* Someone else's hold is grey. Gold means *you* have it, and that one
   difference is what makes your world findable without reading a label. */
.vk-chip--other { border-color: rgb(var(--mist)); background: rgb(var(--well)); color: rgb(var(--mist)); }

/* Shown only when a hold is under ~3h — pressure, not a fact. */
.vk-count {
  display: inline-flex; align-items: center; gap: 5px;
  border: 1px solid rgb(var(--ember) / 0.45); border-radius: 3px; padding: 1px 7px;
  font-family: var(--mono); font-size: 11px; color: rgb(var(--ember));
}

/* ---- the games toolbar ----------------------------------------------- */
.vk-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; }
.vk-search { position: relative; flex: 1; max-width: 340px; }
.vk-search .vk-i { position: absolute; left: 11px; top: 11px; color: rgb(var(--mist)); }
.vk-search input {
  width: 100%; border: 1px solid rgb(var(--edge)); border-radius: 4px; background: rgb(var(--ink));
  padding: 8px 10px 8px 32px; font-family: inherit; font-size: 13.5px; color: rgb(var(--parchment));
}
.vk-seg { display: flex; gap: 2px; border: 1px solid rgb(var(--edge)); border-radius: 4px; padding: 2px; }
.vk-seg-item {
  border: 0; background: none; cursor: pointer; border-radius: 3px; padding: 4px 12px;
  font-family: inherit; font-size: 12.5px; color: rgb(var(--mist));
}
.vk-seg-item--on { background: rgb(var(--panel)); color: rgb(var(--goldhi)); }
.vk-toolbar-acts { margin-left: auto; display: flex; gap: 8px; }

/* ---- offline --------------------------------------------------------- */
.vk-offline {
  display: flex; align-items: flex-start; gap: 14px;
  border: 1px solid rgb(var(--ember) / 0.5); background: rgb(var(--panel));
  border-radius: 8px; padding: 15px 18px;
}
.vk-offline .vk-i--lg { color: rgb(var(--ember)); stroke-width: 1.8; width: 17px; height: 17px; }
.vk-offline-body { flex: 1; }
.vk-offline-lead { font-size: 14.5px; color: rgb(var(--parchment)); }
.vk-offline-sub { margin-top: 3px; font-size: 12.5px; color: rgb(var(--mist)); }

/* ---- a first-run step ------------------------------------------------ */
.vk-step { display: flex; align-items: center; gap: 14px; padding: 16px 20px; }
.vk-step + .vk-step { border-top: 1px solid rgb(var(--edge)); }
.vk-step--now { background: rgb(var(--well)); }
.vk-step-mark {
  flex: none; width: 22px; height: 22px; border-radius: 999px; border: 1px solid;
  display: flex; align-items: center; justify-content: center;
  font-family: var(--mono); font-size: 11px;
}
.vk-step-mark--done { border-color: rgb(var(--ok)); color: rgb(var(--ok)); }
.vk-step-mark--now { border-color: rgb(var(--gold)); color: rgb(var(--gold)); }
.vk-step-body { flex: 1; }
.vk-step-title { font-size: 14.5px; color: rgb(var(--parchment)); }
.vk-step-sub { font-size: 12.5px; color: rgb(var(--mist)); }

.vk-worldrow { display: flex; align-items: center; gap: 16px; padding: 15px 18px; }
.vk-worldrow:hover { background: rgb(var(--well)); }
.vk-worldrow-art {
  width: 54px; height: 72px; flex: none; display: flex; align-items: center;
  justify-content: center; border: 1px solid rgb(var(--edge)); border-radius: 5px;
  background: var(--fill-cover); padding: 5px; text-align: center; font-size: 9.5px;
  line-height: 1.15; color: rgb(var(--mist));
}
.vk-worldrow-art--dim { opacity: 0.75; }
.vk-worldrow-main { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 5px; }
.vk-worldrow-titlerow { display: flex; flex-wrap: wrap; align-items: baseline; gap: 10px; }
.vk-worldrow-name { font-size: 17px; font-weight: 700; color: rgb(var(--parchment)); }
.vk-worldrow-game { font-size: 12px; color: rgb(var(--rune)); }
.vk-worldrow-line { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; font-size: 12.5px; color: rgb(var(--mist)); }
.vk-worldrow-meta { text-align: right; font-family: var(--mono); font-size: 11px; line-height: 1.5; color: rgb(var(--mist)); }
.vk-worldrow-acts { display: flex; align-items: center; gap: 8px; margin-left: 8px; }
.vk-dir { word-break: break-all; font-family: var(--mono); font-size: 11px; color: rgb(var(--mist)); }

.vk-trail { font-family: var(--mono); font-size: 12px; color: rgb(var(--mist)); }
.vk-trail summary { cursor: pointer; }
.vk-trail-row { padding: 2px 0; }
.vk-trail-row b { font-weight: 700; }
.vk-trail-hit { color: rgb(var(--ok)); }
.vk-trail-to { padding-left: 16px; word-break: break-all; }
.vk-trail-note { padding-left: 16px; color: rgb(var(--ember)); }

.vk-update {
  display: flex; flex-wrap: wrap; align-items: center; gap: 12px;
  border: 1px solid rgb(var(--gold) / 0.5); background: var(--fill-held);
  border-radius: 8px; padding: 12px 20px;
}
.vk-update .vk-i--lg { color: rgb(var(--gold)); stroke-width: 1.4; width: 20px; height: 20px; }
.vk-update-body { flex: 1; font-size: 13px; }
.vk-update-lead { font-size: 14px; color: rgb(var(--parchment)); }

.vk-panelfail {
  border: 1px solid rgb(var(--ember) / 0.5); background: rgb(var(--panel));
  border-radius: 8px; padding: 12px 16px; font-size: 13px; color: rgb(var(--ember));
}
.vk-panelfail-sub { margin-top: 4px; font-size: 12px; color: rgb(var(--mist)); }
`;

const chip = (state, label, glyph) =>
  `<span class="vk-chip vk-chip--${state}">${glyph ? sm(glyph) : ""}${label}</span>`;

// Gold is reserved for the world *this* machine holds. Someone else's hold
// is grey — one difference, and it is what makes your world findable in a
// list of any length without reading a label.
const CHIP = {
  free: chip("free", "Free", ICON.lockOpen),
  mine: chip("held", "Yours", ICON.lock),
  fetching: chip("held", "Yours — fetching", ICON.lock),
  held: chip("other", "Held", ICON.lock),
  expired: chip("expired", "Hold expired", ICON.clock),
  gone: chip("expired", "Not on the service"),
};

const countdown = (text) => `<span class="vk-count">${sm(ICON.clock)}${text}</span>`;

const dots = `<button class="vk-btn vk-btn--quiet vk-btn--icon" aria-label="More actions">${sm(ICON.dotsVertical)}</button>`;

const tile = (name, note, linked, extra = "") =>
  `            <button class="vk-tile ${linked ? "vk-tile--linked" : "vk-tile--unlinked"}${extra}">
              <div class="vk-tile-art">${name}</div>
              <div class="vk-tile-cap">
                <div class="vk-tile-capmain">
                  <div class="vk-tile-name">${name}</div>
                  <div class="vk-tile-note${linked ? " vk-tile-note--linked" : ""}">${note}</div>
                </div>
                ${linked ? "" : '<span class="vk-tile-link">Link</span>'}
              </div>
            </button>`;

const worldRow = ({ name, game, chip: c, line, meta, acts, dim }) => `
            <div class="vk-worldrow">
              <div class="vk-worldrow-art${dim ? " vk-worldrow-art--dim" : ""}">${game}</div>
              <div class="vk-worldrow-main">
                <div class="vk-worldrow-titlerow">
                  <span class="vk-worldrow-name">${name}</span>
                  <span class="vk-worldrow-game">${game}</span>
                </div>
                <div class="vk-worldrow-line">${c}${line ? `<span>${line}</span>` : ""}</div>
              </div>
              <div class="vk-worldrow-meta">${meta}</div>
              <div class="vk-worldrow-acts">${acts}${dots}</div>
            </div>`;

const group = ({ label, sub, gold, rows }) => `          <div class="vk-group">
            <div class="vk-group-head">
              <div class="vk-group-label${gold ? " vk-group-label--gold" : ""}">${label}</div>
              ${sub ? `<div class="vk-group-sub">${sub}</div>` : ""}
            </div>
            <div class="vk-group-body${gold ? " vk-group-body--gold" : ""}">${rows}
            </div>
          </div>`;

export default {
  app: "companion",
  title: "Artificer Companion",
  tagline: "shared world saves, synced from this machine",
  intent: `The player-side half of save sync, wearing <b>reliquary's palette verbatim</b> —
      one system, two halves. What is different is what the companion knows and the service
      cannot: which games are installed on this machine, which folder each save lives in, and
      whether the vault is reachable from here at all. Every screen is built so a player can
      answer "is my save where it should be?" without opening a folder.`,
  source: {
    css: "web/companion/src/index.css",
    tailwind: "web/companion/tailwind.config.js",
    doc: "docs/companion-ui-rebuild.md",
  },
  chrome: vaultChrome,
  tokens: { colors: vaultColors, derived: vaultDerived, colorScheme: "dark" },
  kit,
  groups: [
    {
      name: "Foundations",
      cards: [
        {
          slug: "shared-palette",
          name: "Shared palette",
          subtitle: "The same eleven tokens reliquary declares",
          viewport: { width: 900, height: 860 },
          intent: `<code>web/companion/src/index.css</code> and
            <code>web/reliquary/src/index.css</code> declare these eleven tokens identically,
            comment for comment. That is deliberate and it is checked: the companion is not a
            separate product with a matching skin, it is the other end of the same custody
            flow, and a player moving between the two should not be able to tell where one
            stops. Both files are compared against this list by
            <code>scripts/checkdesign.sh</code>.`,
          specimens: [
            { stage: "block", caption: "Grounds", html: swatches(["ink", "well", "panel", "edge"]) },
            { stage: "block", caption: "Text", html: swatches(["parchment", "mist"]) },
            { stage: "block", caption: "Accent and state", html: swatches(["gold", "goldhi", "ok", "ember", "rune"]) },
            { stage: "block", caption: "Compound fills", html: derivedSwatches(vaultDerived) },
          ],
          rules: [
            "A change to the vault palette is a change to <i>both</i> apps. There is no companion-only color.",
            "The three consoles are a different design language — do not reach for their tokens here, or these there.",
          ],
          sources: ["web/companion/src/index.css", "web/companion/tailwind.config.js", "design-system/lib/vault.mjs"],
        },
        {
          slug: "typography",
          name: "Type",
          subtitle: "Georgia, and mono for anything about this machine",
          viewport: { width: 900, height: 700 },
          intent: `The vault's type scale, with one role the service does not have: a
            <b>filesystem path</b>. Paths are mono and shown in full, never truncated to an
            ellipsis — the folder is the one thing on the page that is about this machine
            rather than the world, and it is the thing a player checks when a save goes to the
            wrong place.`,
          specimens: [
            {
              stage: "block",
              caption: "Scale",
              html: [
                typeRow("header · 18px / 0.05em / gold", `<span class="vk-header-name">Artificer Companion</span>`),
                typeRow("section · 12px / 0.12em / caps", `<span class="vk-section-h">Linked worlds</span>`),
                typeRow("world name · 16px / bold", `<span class="vk-worldrow-name">Ashwood Hollow</span>`),
                typeRow("body · 15px", `Everything the app says in a sentence.`),
                typeRow("secondary · 13px / mist", `<span style="font-size:13px;color:rgb(var(--mist))">until 21:14 · save is on this machine</span>`),
                typeRow("hint · 12px / italic / mist", `<span class="vk-section-hint">nothing here yet — link a game from the shelf</span>`),
                typeRow("path · 11px / mono, never truncated", `<span class="vk-dir">C:\\Users\\hazel\\AppData\\Local\\Dragonwilds\\Saved\\SaveGames\\AshwoodHollow</span>`),
                typeRow("builds · 11px / mono", `<span class="vk-footer" style="border:0;padding:0">companion v0.9.4 · service v1.9.2</span>`),
              ].join("\n"),
            },
          ],
          rules: [
            "A path is never elided. <code>break-all</code> and let it wrap — a half-path is worse than a long one.",
            "Both build strings appear in the footer: a save-sync report that names one half names nothing.",
            "Hints are italic mist; they explain, they never instruct.",
          ],
          sources: ["web/companion/src/components/WorldRow.tsx", "web/companion/src/components/HeaderBar.tsx"],
        },
      ],
    },
    {
      name: "Actions",
      cards: [
        {
          slug: "button",
          name: "Buttons",
          subtitle: "The vault's three, with the companion's verbs",
          viewport: { width: 900, height: 700 },
          intent: `The same component as reliquary's, down to the gradient — but the companion's
            primary answers a different question. The service asks "may I take this?"; the
            companion asks "what happens on this machine now?" So its primaries name both
            halves of an intention: <b>Check out &amp; play</b> fetches the save <i>and</i>
            starts the game, and it only says "&amp; play" when both are actually going to
            happen.`,
          specimens: [
            {
              caption: "Variants",
              html: `          ${btn("primary", "Check out &amp; play")}\n          ${btn("quiet", "Renew hold")}\n          ${btn("danger", "Unlink")}`,
            },
            {
              caption: "The primary promises only what will happen",
              html: `          ${btn("primary", "Check out &amp; play")}\n          ${btn("primary", "Check out &amp; host")}\n          ${btn("primary", "Check out")}`,
            },
            {
              caption: "With an icon, and the busy state",
              html: `          <button class="vk-btn vk-btn--quiet">${sm(ICON.refresh)}Sync now</button>
          <button class="vk-btn vk-btn--quiet is-disabled">${sm(ICON.refresh)}Syncing…</button>
          <button class="vk-btn vk-btn--quiet vk-btn--icon" aria-label="Settings">${sm(ICON.settings)}</button>`,
            },
            {
              caption: "The action row of a world you hold: one primary, one quiet, three dots",
              html: `          <button class="vk-btn vk-btn--primary">${sm(ICON.play)}Play</button>\n          ${btn("quiet", "Check in")}\n          ${dots}`,
            },
          ],
          rules: [
            "“&amp; play” appears only when the setting is on <i>and</i> the world has something to start. A world linked by hand from a folder has no app id, so the button goes back to promising the save alone.",
            "Checking out is two halves of one intention, and both are reported: a save on disk with a game that would not start is a real outcome — “checked out, but the game did not start” beats a failure toast.",
            "“Checkpoint now” shows only for worlds the service keeps checkpoints for. A button that always 501s is a lie about the feature.",
            "<b>One primary, one quiet, then the overflow.</b> The row used to carry four buttons of equal weight, which meant it carried none — nothing on it said which one you were meant to press.",
          ],
          sources: ["web/companion/src/components/ui/button.tsx", "web/companion/src/components/WorldRow.tsx"],
        },
      ],
    },
    {
      name: "Status",
      cards: [
        {
          slug: "custody-chip",
          name: "Custody chip",
          subtitle: "Six states, from one call — the service's three plus this machine's",
          viewport: { width: 900, height: 640 },
          intent: `Reliquary knows three custody states because it only knows who holds a world.
            The companion knows three more, because it also knows what is on <i>this</i> disk:
            the hold is yours and the save is here (<b>You hold this world</b>), the hold is
            yours but the download is still on its way (<b>Yours — fetching</b>), and the world
            this folder was linked to is no longer on the service at all (<b>Gone</b>).`,
          specimens: [
            {
              stage: "stack",
              caption: "Every state with the line that follows it",
              html: [
                [CHIP.free, "next in line: rook"],
                [CHIP.mine, "41h left on the hold"],
                [CHIP.fetching, "fetching it to this machine…"],
                [CHIP.held, "held by rook"],
                [CHIP.held, `held by rook ${countdown("2h 12m left")}`],
                [CHIP.expired, "held by rook — the hold expired"],
                [CHIP.gone, "world #14 is not on the service any more"],
              ]
                .map(([c, line]) => `          <div class="vk-card-line">${c}<span>${line}</span></div>`)
                .join("\n"),
            },
          ],
          rules: [
            "“Yours — fetching” offers no actions. A hold belonging to this account but to a <i>different session</i> offers nothing either — the download is still on its way here, and a button would race it.",
            "“Gone” is ember but not an error the player caused: the link survives so they can see which folder it pointed at before deciding.",
            "Custody is computed <b>once</b> — <code>custodyOf()</code> returns <code>{state, holder, expiresAt, claimedBy}</code> and the chip, the sentence, the group and the primary action are all derived from that one record. Nothing else decides either; a “Free” chip beside a disabled Check out is the defect this shape exists to prevent.",
            "<b>Gold means <i>you</i> hold it.</b> Someone else's hold is grey on a well. That one difference is what makes your world findable in a list of any length without reading a label.",
            "A free world's line is empty rather than “nobody holds this world” — that restated the Free chip from the other side of the row.",
            "The countdown appears only under three hours. Holds last 48, and a timer running for two days is noise pretending to be urgency.",
          ],
          sources: ["web/companion/src/components/CustodyChip.tsx", "web/companion/src/lib/types.ts"],
        },
        {
          slug: "connection",
          name: "Window chrome",
          subtitle: "Reachable, as whom, how fresh — said exactly once",
          viewport: { width: 980, height: 700 },
          intent: `Sync state used to be reported three times: the header dot, a "you're up to
            date" line at the bottom of the page, and the scan trail's own summary. Three
            readings of one fact drift, and nothing tells a player which is current. It is
            said <b>once</b> now — a dot and a relative time — and the scan trail, the tried
            paths and both build versions moved to Settings › Diagnostics, which the status
            bar links to. Freshness runs on a clock of its own, because the poll it describes
            may answer with an unchanged timestamp, and an age that stops moving reads as a
            frozen app.`,
          specimens: [
            {
              stage: "block",
              caption: "Header and tabs",
              html: `          <header class="vk-header">
            <div><div class="vk-header-name" style="font-size:22px">Artificer Companion</div><div class="vk-header-tag" style="font-size:12.5px">this machine, syncing as hazel</div></div>
            <div class="vk-header-right">
              <span class="vk-conn vk-mono" style="font-size:11px;color:rgb(var(--mist))"><span class="vk-dot vk-dot--live"></span>synced 2 min ago</span>
              <button class="vk-btn vk-btn--quiet">${sm(ICON.refresh)}Sync now</button>
              <button class="vk-btn vk-btn--quiet vk-btn--icon" aria-label="Settings">${sm(ICON.settings)}</button>
            </div>
          </header>
          <div class="vk-tabs">
            <button class="vk-tab vk-tab--on">Worlds</button>
            <button class="vk-tab">Games</button>
            <button class="vk-tab">Activity</button>
            <button class="vk-tab">Conflicts<span class="vk-tab-badge">1</span></button>
            <button class="vk-tab">Settings</button>
          </div>`,
            },
            {
              stage: "block",
              caption: "Not connected, and the vault unreachable",
              html: `          <header class="vk-header" style="margin-bottom:14px">
            <div><div class="vk-header-name" style="font-size:22px">Artificer Companion</div><div class="vk-header-tag" style="font-size:12.5px">this machine — not connected to a vault yet</div></div>
            <div class="vk-header-right">
              <button class="vk-btn vk-btn--quiet vk-btn--icon" aria-label="Settings">${sm(ICON.settings)}</button>
            </div>
          </header>
          <header class="vk-header">
            <div><div class="vk-header-name" style="font-size:22px">Artificer Companion</div><div class="vk-header-tag" style="font-size:12.5px">this machine, syncing as hazel</div></div>
            <div class="vk-header-right">
              <span class="vk-conn vk-mono" style="font-size:11px;color:rgb(var(--mist))"><span class="vk-dot" style="background:rgb(var(--ember))"></span>the vault is unreachable</span>
              <button class="vk-btn vk-btn--quiet">${sm(ICON.refresh)}Sync now</button>
              <button class="vk-btn vk-btn--quiet vk-btn--icon" aria-label="Settings">${sm(ICON.settings)}</button>
            </div>
          </header>`,
            },
            {
              stage: "block",
              caption: "The status bar: what this machine holds, and the way into diagnostics",
              html: `          <footer class="vk-footer" style="font-family:inherit;font-size:12px"><span>6 worlds · 13 games installed, 1 linked</span><a class="vk-footer-last" href="#" style="color:rgb(var(--goldhi))">Diagnostics</a></footer>`,
            },
            {
              stage: "block",
              caption: "Offline is a variant of Worlds, not a tab — the Worlds tab stays active",
              html: `          <div class="vk-offline">
            ${ICON.wifiOff.replace('class="vk-i"', 'class="vk-i vk-i--lg"')}
            <div class="vk-offline-body">
              <div class="vk-offline-lead">Working offline — the vault is unreachable</div>
              <div class="vk-offline-sub">Keep playing the world you already hold. The hold stands until the vault answers again.</div>
              <div class="vk-mono" style="margin-top:6px;font-size:11px;color:rgb(var(--ember))">dial tcp: lookup vault.example.com: no such host</div>
            </div>
            ${btn("quiet", "Retry now")}
          </div>`,
            },
          ],
          rules: [
            "<b>One place.</b> The dot and the relative time are the whole sync report; everything else that used to say it is gone or has moved to Diagnostics.",
            "Three dot states, not two: ok when connected, ember when the last sync errored, mist when no vault is configured at all.",
            "The error text is the transport's own words. “Could not connect” sends a player to the wrong place; a DNS failure names itself.",
            "The Conflicts badge is drawn only when the count is non-zero. A “0” beside Conflicts is a number nobody needs.",
            "“Sync now” exists for being certain rather than patient — the page keeps itself current while open, and this is how a player hears out loud that the service cannot be reached.",
          ],
          sources: [
            "web/companion/src/components/HeaderBar.tsx",
            "web/companion/src/components/TabBar.tsx",
            "web/companion/src/components/Offline.tsx",
          ],
        },
        {
          slug: "panel-failure",
          name: "Section headers and panel failure",
          subtitle: "A panel that fails, fails alone — and says which",
          viewport: { width: 900, height: 520 },
          intent: `One section throwing used to take the whole page with it: the shelf, the scan
            trail and the version line all sat downstream of a single unguarded read, so one
            null became three bugs that looked unrelated. Each panel now has its own boundary,
            and a failed one <b>names itself</b> and says the rest still works.`,
          specimens: [
            {
              stage: "block",
              caption: "Section header, with and without controls",
              html: `          <div class="vk-section" style="margin-bottom:14px"><h2 class="vk-section-h">Linked worlds</h2><span class="vk-section-hint">the worlds this machine can check out</span><div class="vk-section-acts">${btn("quiet", "Link a game…", " vk-btn--sm")}</div></div>
          <div class="vk-section"><h2 class="vk-section-h">Your games</h2><span class="vk-section-hint">found on this machine</span></div>`,
            },
            {
              stage: "block",
              caption: "A panel that failed",
              html: `          <div class="vk-panelfail">The worlds panel failed: Cannot read properties of undefined (reading 'saveDirs')
            <div class="vk-panelfail-sub">The rest of this page still works. Reopening the companion usually clears it.</div>
          </div>`,
            },
          ],
          rules: [
            "The boundary message includes the thrown error. A player pasting it into a bug report should be pasting something actionable.",
            "It names the panel, because “something went wrong” next to four panels identifies none of them.",
            "It says what still works. A failure that looks total gets the app closed instead of reported.",
          ],
          sources: ["web/companion/src/components/Panel.tsx"],
        },
      ],
    },
    {
      name: "Patterns",
      cards: [
        {
          slug: "shelf",
          name: "The games library",
          subtitle: "Linked first, and every tile says what linking costs",
          viewport: { width: 1000, height: 780 },
          intent: `Setup, visited rarely, and its own tab — it used to take about 70% of the
            window, above the worlds it exists to create. The old grid had no search and no
            filter, put the one linked tile among twelve dimmed ones, and captioned eleven of
            thirteen with "not linked", which the dim treatment already said. Linked games have
            their own section now, and in place of that caption each unlinked tile says what
            linking it would cost: whether a folder is already known, or whether you will be
            picking it yourself.`,
          specimens: [
            {
              stage: "block",
              caption: "Toolbar",
              html: `          <div class="vk-toolbar">
            <div class="vk-search">${sm(ICON.search)}<input placeholder="Search installed games" /></div>
            <div class="vk-seg">
              <button class="vk-seg-item vk-seg-item--on">All 13</button>
              <button class="vk-seg-item">Linked 1</button>
              <button class="vk-seg-item">Unlinked 12</button>
            </div>
            <div class="vk-toolbar-acts">${btn("quiet", "Rescan")}${btn("quiet", "Link a folder by hand…")}</div>
          </div>`,
            },
            {
              stage: "block",
              caption: "Linked first, then not linked",
              html: `          <div class="vk-group" style="margin-bottom:18px">
            <div class="vk-group-head"><div class="vk-group-label vk-group-label--gold">Linked · 2</div></div>
            <div class="vk-shelf">
${tile("Dragonwilds", "1 world · Ashwood Hollow", true)}
${tile("Enshrouded", "1 world · Cinderfall", true, " vk-tile--active")}
            </div>
          </div>
          <div class="vk-group">
            <div class="vk-group-head"><div class="vk-group-label">Not linked · 4</div><div class="vk-group-sub">A save folder is already known for 2 of these.</div></div>
            <div class="vk-shelf">
${tile("Palworld", "save folder known", false)}
${tile("RuneScape: Dragonwilds", "save folder known", false)}
${tile("Valheim", "pick the folder yourself", false)}
${tile("Voyagers of Nera", "pick the folder yourself", false)}
            </div>
            <div style="margin-top:12px;font-size:12.5px;color:rgb(var(--mist))">4 entries were hidden as duplicates or launchers. <a href="#" style="color:rgb(var(--goldhi))">Show them</a></div>
          </div>`,
            },
          ],
          rules: [
            "Tiles are memo'd and keyed by the game's identity. The page polls every five seconds, and an <code>&lt;img&gt;</code> that remounts re-fetches — rebuilding identical tiles made every cover flicker, and filtering must not remount the tiles that survive.",
            "Names <b>wrap to two lines</b> and never truncate. “RuneScape: Dragon…” is not a name.",
            "The <code>Link</code> affordance sits on the tile, so clickability is stated where it is acted on rather than in a sentence under the whole grid.",
            "Hidden entries are a line of text below the grid, never a tile inside it: a control shaped like a game gets clicked by accident. Steam's redistributables, runtimes and controller configs start out here.",
            "A tile's tooltip carries the first save directory, so hovering answers “which install is this?” without a click.",
          ],
          sources: ["web/companion/src/components/GameTile.tsx", "web/companion/src/components/GamesTab.tsx"],
        },
        {
          slug: "world-row",
          name: "Worlds, grouped by custody",
          subtitle: "What you hold, what you can take, what someone else has",
          viewport: { width: 1020, height: 900 },
          intent: `Worlds is the whole page. Grouping by <b>what you can do with a world</b>
            replaces the per-row status prose the old page carried, and the group holding the
            world locked to this machine is bordered in gold — which is what makes it findable
            at a glance in a list of any length. Group membership, the chip and the primary
            action are all derived from the same <code>custodyOf</code> record, so they cannot
            disagree.`,
          specimens: [
            {
              stage: "block",
              caption: "The three groups",
              html: [
                group({
                  label: "Checked out to you",
                  sub: "locked to this machine until you check it in",
                  gold: true,
                  rows: worldRow({
                    name: "Cinderfall",
                    game: "Enshrouded",
                    chip: CHIP.mine,
                    line: "41h left on the hold",
                    meta: "v18 · 240 MB · 4 min ago",
                    acts: `<button class="vk-btn vk-btn--primary">${sm(ICON.play)}Play</button>${btn("quiet", "Check in")}`,
                  }),
                }),
                group({
                  label: "Free to take · 2",
                  rows:
                    worldRow({
                      name: "Ashwood Hollow",
                      game: "Dragonwilds",
                      chip: CHIP.free,
                      line: "",
                      meta: "v41 · 1.8 GB · 18:02",
                      acts: `<button class="vk-btn vk-btn--primary">${sm(ICON.play)}Check out &amp; play</button>${btn("quiet", "Check out only")}`,
                    }) +
                    worldRow({
                      name: "Tidewatch",
                      game: "Len's Island",
                      chip: CHIP.free,
                      line: "you're next",
                      meta: "v6 · 310 MB · Sunday",
                      acts: `<button class="vk-btn vk-btn--primary">${sm(ICON.play)}Check out &amp; play</button>${btn("quiet", "Check out only")}`,
                    }),
                }),
                group({
                  label: "Held by someone else · 2",
                  rows:
                    worldRow({
                      name: "Verdant Reach",
                      game: "Palworld",
                      chip: CHIP.held,
                      line: `held by rook ${countdown("2h 12m left")}`,
                      meta: "v7 · 210 MB · 2h ago",
                      dim: true,
                      acts: btn("quiet", "Ask for it back"),
                    }) +
                    worldRow({
                      name: "Stormhold",
                      game: "Sea of Stars",
                      chip: CHIP.expired,
                      line: "held by mira — the hold expired",
                      meta: "v23 · 90 MB · Tuesday",
                      dim: true,
                      acts: `${btn("primary", "Take over expired hold")}${btn("quiet", "Ask for it back")}`,
                    }),
                }),
              ].join("\n"),
            },
            {
              stage: "block",
              caption: "The overflow — the rare verbs, and the save path that used to sit in the row",
              html: `          <div class="vk-menu">
            <div style="border-bottom:1px solid rgb(var(--edge));padding:8px 12px;font-family:var(--mono);font-size:11px;color:rgb(var(--mist));word-break:break-all">C:\\Users\\hazel\\Saved Games\\Enshrouded\\Cinderfall</div>
            <div class="vk-menuitem">Renew hold</div>
            <div class="vk-menuitem is-highlighted">Checkpoint now</div>
            <div class="vk-menuitem">Open save folder</div>
            <div class="vk-menuitem">Copy save path</div>
            <div class="vk-menuitem">Rename or move…</div>
            <div class="vk-menusep"></div>
            <div class="vk-menuitem vk-menuitem--danger">Unlink</div>
          </div>`,
            },
            {
              stage: "block",
              caption: "The only pointer from Worlds to the games library",
              html: `          <div class="vk-well" style="font-size:12.5px">13 games installed on this machine, 1 linked to a world. <a href="#" style="color:rgb(var(--goldhi))">Open the games library</a></div>`,
            },
          ],
          rules: [
            "The chip moved off the end of the title row and onto the line below it, beside the sentence it qualifies: the name identifies, the custody line explains, and they no longer sit in opposite corners.",
            "<b>The save path left the row.</b> It is a debug fact, not a daily one — it sits at the head of the overflow menu, still one click away in full.",
            "The cover of a world someone else holds drops to 0.75 opacity. Nothing here is yours to take, and the row says so before it is read.",
            "“Take over expired hold” confirms with what it costs — the old holder's late check-in is kept and flagged, not lost.",
            "“Yours — fetching” offers no actions: the hold is this account's but on another session, and a button would race the download.",
            "Offline, every verb that would <i>take</i> a world is withdrawn and the worlds nobody here holds are hidden behind a stated reason. Play stays — that is the whole point of the state.",
          ],
          sources: [
            "web/companion/src/components/WorldsTab.tsx",
            "web/companion/src/components/WorldRow.tsx",
            "web/companion/src/components/OverflowMenu.tsx",
          ],
        },
        {
          slug: "scan-trail",
          name: "Scan trail",
          subtitle: "Where it looked, and what it found",
          viewport: { width: 940, height: 560 },
          intent: `Diagnostics, and nowhere else. This used to sit in the page chrome under the
            games grid, where it competed with the worlds for the eye and said the sync state a
            second time; it lives in Settings › Diagnostics now, with the tried paths, the
            linked save folders and both build versions — what a bug report needs, and what
            nothing else needs. Discovery is a guess about someone else's machine, so it shows its working.
            Every probe is listed with its source, the path it tried, what it resolved to, and
            why it did not — collapsed by default, because the answer is usually "it worked",
            and there in full when it is not.`,
          specimens: [
            {
              stage: "block",
              caption: "Expanded",
              html: `          <details class="vk-trail" open>
            <summary>where the companion looked — 6 probes, 3 resolved</summary>
            <div class="vk-trail-row"><span class="vk-trail-hit">✓</span> <b>steam</b> <span>C:\\Program Files (x86)\\Steam\\steamapps\\libraryfolders.vdf</span><div class="vk-trail-to">→ D:\\SteamLibrary\\steamapps\\common</div></div>
            <div class="vk-trail-row"><span class="vk-trail-hit">✓</span> <b>registry</b> <span>HKCU\\Software\\Dragonwilds\\InstallPath</span><div class="vk-trail-to">→ D:\\SteamLibrary\\steamapps\\common\\Dragonwilds</div></div>
            <div class="vk-trail-row"><span>·</span> <b>epic</b> <span>C:\\ProgramData\\Epic\\UnrealEngineLauncher\\LauncherInstalled.dat</span><div class="vk-trail-note">not found — Epic does not appear to be installed</div></div>
            <div class="vk-trail-row"><span>·</span> <b>xbox</b> <span>C:\\XboxGames</span><div class="vk-trail-note">not found</div></div>
          </details>`,
            },
          ],
          rules: [
            "A resolved probe is <code>ok</code>; an unresolved one is mist with an ember note. The note says why, not just that.",
            "Paths are shown whole and wrap. This list exists to be compared against what the player knows about their own disk.",
            "The trail renders nothing when there are no probes — an empty disclosure is a dead control.",
          ],
          sources: [
            "web/companion/src/components/ScanTrail.tsx",
            "web/companion/src/components/SettingsTab.tsx",
          ],
        },
        {
          slug: "first-run",
          name: "No worlds yet",
          subtitle: "Derived from “nothing linked”, never from a stored flag",
          viewport: { width: 980, height: 640 },
          intent: `The machine is connected to a vault and holds no worlds. Keying this on
            <b>no linked worlds</b> rather than on a first-run flag means it comes back on its
            own if every link is removed — it is a description of the machine, not a memory of
            one. It says what is already true before it asks for anything, because both facts a
            player needs in order to trust the next step — the vault answered, and these games
            were found — are already known by the time this renders.`,
          specimens: [
            {
              stage: "block",
              caption: "The three steps",
              html: `          <div style="max-width:620px;margin:0 auto;display:flex;flex-direction:column;gap:22px">
            <div style="text-align:center">
              <div class="vk-h1" style="font-size:21px">No worlds on this machine yet</div>
              <div style="margin:8px auto 0;max-width:46ch;font-size:14px;color:rgb(var(--mist))">A world is one save folder the vault holds for your group. Link a game's save folder and the vault starts keeping its history.</div>
            </div>
            <div class="vk-group-body">
              <div class="vk-step">
                <span class="vk-step-mark vk-step-mark--done">✓</span>
                <div class="vk-step-body"><div class="vk-step-title">Signed in as hazel</div><div class="vk-step-sub">https://vault.example.com</div></div>
              </div>
              <div class="vk-step">
                <span class="vk-step-mark vk-step-mark--done">✓</span>
                <div class="vk-step-body"><div class="vk-step-title">Found 13 installed games</div><div class="vk-step-sub">Across 4 libraries. A save folder is already known for 7 of them.</div></div>
                ${btn("quiet", "Review")}
              </div>
              <div class="vk-step vk-step--now">
                <span class="vk-step-mark vk-step-mark--now">3</span>
                <div class="vk-step-body"><div class="vk-step-title">Link a game to make your first world</div><div class="vk-step-sub">Pick the game and the companion suggests the save folder. You confirm it.</div></div>
                ${btn("primary", "Choose a game", " vk-btn--lg")}
              </div>
            </div>
          </div>`,
            },
          ],
          rules: [
            "Done steps are marked in <code>ok</code>; the step still to do is gold and its row carries a well's ground, so “where am I” is answered before anything is read.",
            "Both call-to-action buttons go to the same place — the games library — because linking a game is the only way to make a world. Two doors to one room is not a choice.",
            "The screen states the vault URL and the library count as facts, not as a summary: if either is wrong, that is the thing the player needs to see before linking anything.",
          ],
          sources: ["web/companion/src/components/NoWorlds.tsx"],
        },
        {
          slug: "update-banner",
          name: "Update banner",
          subtitle: "A different build is available, and what it will do",
          viewport: { width: 940, height: 460 },
          intent: `The companion keeps itself current, which means it can replace itself while a
            player is looking at it. The banner is gold-on-held rather than an alert colour —
            this is good news — and it says plainly that installing <b>replaces this one and
            restarts</b>, because an app that vanishes mid-session without warning reads as a
            crash.`,
          specimens: [
            {
              stage: "block",
              caption: "Available",
              html: `          <div class="vk-update">
            ${ICON.arrowUp.replace('class="vk-i"', 'class="vk-i vk-i--lg"')}
            <div class="vk-update-body"><span class="vk-update-lead">A different companion build is available.</span> <span class="vk-mono" style="color:rgb(var(--mist))">v0.9.5</span><span style="color:rgb(var(--mist))"> — it replaces this one and restarts.</span></div>
            ${btn("primary", "Install and restart")}
            ${btn("quiet", "Not now")}
          </div>`,
            },
          ],
          rules: [
            "“A <i>different</i> build”, not “a newer build”: the service is the source of truth, and a rollback is a legitimate thing for it to publish.",
            "Gold, not ember. This is not a problem being reported.",
            "The banner is dismissible and never blocks. A player mid-hold must be able to finish and check in first.",
          ],
          sources: ["web/companion/src/components/UpdateBanner.tsx", "docs/companion.md"],
        },
      ],
    },
  ],
};
