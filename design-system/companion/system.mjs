// The Artificer Companion's design system, as data.
//
// The companion is the player-side half of save sync: it discovers installed
// games, links their save folders to worlds in the vault, and moves the saves.
// It shares reliquary's palette and primitives verbatim — one system, two
// halves — so the tokens and the `vk-*` kit come from `../lib/vault.mjs` and
// this file holds only what the player-side app draws: the shelf, the linked
// world row, the header that says whether the vault is reachable, and the scan
// trail that says where it looked.
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

.vk-shelf { display: grid; grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: 14px; }
.vk-tile {
  display: flex; flex-direction: column; overflow: hidden; border-radius: 6px;
  border: 1px solid; background: rgb(var(--ink)); text-align: left; cursor: pointer;
  padding: 0; font-family: inherit; color: inherit;
  transition: filter 0.15s, border-color 0.15s, transform 0.15s;
}
.vk-tile:hover { transform: translateY(-2px); border-color: rgb(var(--gold)); }
.vk-tile--linked { border-color: rgb(var(--gold)); box-shadow: inset 0 0 0 1px rgb(var(--gold) / 0.25); }
.vk-tile--unlinked { border-color: rgb(var(--edge)); filter: grayscale(1) brightness(0.72); }
.vk-tile--unlinked:hover { filter: grayscale(0.25) brightness(1); }
.vk-tile--active { border-color: rgb(var(--goldhi)); }
.vk-tile-art {
  width: 100%; aspect-ratio: 3 / 4; display: flex; align-items: center; justify-content: center;
  background: var(--fill-cover); padding: 6px; text-align: center; font-size: 11px;
  line-height: 1.2; color: rgb(var(--mist));
}
.vk-tile-cap { padding: 6px 7px 8px; font-size: 12px; line-height: 1.2; }
.vk-tile-cap b { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.vk-tile-state { font-size: 10px; text-transform: uppercase; letter-spacing: 0.1em; }
.vk-tile-state--linked { color: rgb(var(--gold)); }
.vk-tile-state--unlinked { color: rgb(var(--mist)); }
.vk-tile--add {
  align-items: center; justify-content: center; gap: 6px; border-style: dashed;
  border-color: rgb(var(--edge)); padding: 10px; text-align: center;
  font-size: 12px; color: rgb(var(--mist)); background: none;
}
.vk-tile--add:hover { border-color: rgb(var(--gold)); color: rgb(var(--parchment)); transform: none; }

.vk-worldrow {
  display: flex; align-items: flex-start; gap: 14px; border: 1px solid rgb(var(--edge));
  background: rgb(var(--panel)); border-radius: 8px; padding: 14px 16px;
}
.vk-worldrow-art {
  width: 56px; aspect-ratio: 3 / 4; flex: none; display: flex; align-items: center;
  justify-content: center; border: 1px solid rgb(var(--edge)); border-radius: 4px;
  background: var(--fill-cover); padding: 4px; text-align: center; font-size: 9px;
  line-height: 1.15; color: rgb(var(--mist));
}
.vk-worldrow-main { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 6px; }
.vk-worldrow-titlerow { display: flex; flex-wrap: wrap; align-items: baseline; gap: 8px; }
.vk-worldrow-name { font-size: 16px; font-weight: 700; }
.vk-worldrow-game { font-size: 12px; color: rgb(var(--rune)); }
.vk-worldrow-titlerow .vk-chip { margin-left: auto; }
.vk-worldrow-line { font-size: 13px; color: rgb(var(--mist)); }
.vk-dir { word-break: break-all; font-family: var(--mono); font-size: 11px; color: rgb(var(--mist)); }
.vk-worldrow-acts { display: flex; flex-wrap: wrap; gap: 8px; }

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

const CHIP = {
  free: chip("free", "Free", ICON.lockOpen),
  mine: chip("held", "You hold this world", ICON.lock),
  fetching: chip("held", "Yours — fetching", ICON.lock),
  held: chip("held", "Held", ICON.lock),
  expired: chip("expired", "Hold expired", ICON.clock),
  gone: chip("expired", "Gone from the service"),
};

const tile = (name, state, linked, extra = "") =>
  `            <button class="vk-tile ${linked ? "vk-tile--linked" : "vk-tile--unlinked"}${extra}">
              <div class="vk-tile-art">${name}</div>
              <div class="vk-tile-cap"><b>${name}</b><span class="vk-tile-state vk-tile-state--${
                linked ? "linked" : "unlinked"
              }">${state}</span></div>
            </button>`;

const worldRow = ({ name, game, chip: c, line, dir, acts }) => `
          <div class="vk-worldrow">
            <div class="vk-worldrow-art">${game}</div>
            <div class="vk-worldrow-main">
              <div class="vk-worldrow-titlerow">
                <span class="vk-worldrow-name">${name}</span>
                <span class="vk-worldrow-game">${game}</span>
                ${c}
              </div>
              <div class="vk-worldrow-line">${line}</div>
              <div class="vk-dir">${dir}</div>
              <div class="vk-worldrow-acts">${acts}</div>
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
              caption: "The action row of a world you hold",
              html: `          ${btn("primary", "Check in")}\n          ${btn("quiet", "Checkpoint now")}\n          ${btn("quiet", "Renew hold")}\n          <button class="vk-btn vk-btn--quiet">${sm(ICON.play)}Play</button>`,
            },
          ],
          rules: [
            "“&amp; play” appears only when the setting is on <i>and</i> the world has something to start. A world linked by hand from a folder has no app id, so the button goes back to promising the save alone.",
            "Checking out is two halves of one intention, and both are reported: a save on disk with a game that would not start is a real outcome — “checked out, but the game did not start” beats a failure toast.",
            "“Checkpoint now” shows only for worlds the service keeps checkpoints for. A button that always 501s is a lie about the feature.",
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
          subtitle: "Six states — the service's three, plus this machine's",
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
                [CHIP.free, "nobody holds this world · next claim: rook"],
                [CHIP.mine, "until 21:14 · save is on this machine"],
                [CHIP.fetching, "fetching it to this machine…"],
                [CHIP.held, "held by rook until 18:40"],
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
            "Custody is computed once, from the link, the world, the signed-in user and whether the vault is configured — the chip and the buttons read the same call.",
          ],
          sources: ["web/companion/src/components/CustodyChip.tsx", "web/companion/src/lib/types.ts"],
        },
        {
          slug: "connection",
          name: "Connection",
          subtitle: "Reachable, as whom, how fresh",
          viewport: { width: 940, height: 620 },
          intent: `Said once at the top rather than in a panel at the bottom: whether the vault
            is reachable, as whom, how old what you are looking at is, and the two things you
            might do about it. Freshness runs on a clock of its own, because the poll it
            describes may answer with an unchanged timestamp — an age that stops moving reads
            as a frozen app.`,
          specimens: [
            {
              stage: "block",
              caption: "Connected",
              html: `          <header class="vk-header">
            ${ICON.laptop.replace('class="vk-i"', 'class="vk-i vk-i--xl"')}
            <div><div class="vk-header-name">Artificer Companion</div><div class="vk-header-tag">shared world saves, synced from this machine</div></div>
            <div class="vk-header-right">
              <span class="vk-conn"><span class="vk-dot vk-dot--live"></span>Connected as <b>hazel</b></span>
              <span class="vk-mono" style="font-size:12px;color:rgb(var(--mist))">checked 12s ago</span>
              <button class="vk-btn vk-btn--quiet">${sm(ICON.refresh)}Sync now</button>
              <button class="vk-btn vk-btn--quiet vk-btn--icon" aria-label="Settings">${sm(ICON.settings)}</button>
            </div>
          </header>`,
            },
            {
              stage: "block",
              caption: "Not connected, and connected but erroring",
              html: `          <header class="vk-header" style="margin-bottom:14px">
            ${ICON.laptop.replace('class="vk-i"', 'class="vk-i vk-i--xl"')}
            <div><div class="vk-header-name">Artificer Companion</div><div class="vk-header-tag">shared world saves, synced from this machine</div></div>
            <div class="vk-header-right">
              <span class="vk-conn"><span class="vk-dot vk-dot--down"></span><span style="color:rgb(var(--mist))">Not connected</span></span>
              <button class="vk-btn vk-btn--quiet vk-btn--icon" aria-label="Settings">${sm(ICON.settings)}</button>
            </div>
          </header>
          <header class="vk-header">
            ${ICON.laptop.replace('class="vk-i"', 'class="vk-i vk-i--xl"')}
            <div><div class="vk-header-name">Artificer Companion</div><div class="vk-header-tag">shared world saves, synced from this machine</div></div>
            <div class="vk-header-right">
              <span class="vk-conn"><span class="vk-dot" style="background:rgb(var(--ember))"></span>Connected as <b>hazel</b></span>
              <span class="vk-mono" style="font-size:12px;color:rgb(var(--mist))">transfer in progress…</span>
              <button class="vk-btn vk-btn--quiet">${sm(ICON.refresh)}Sync now</button>
            </div>
            <div class="vk-header-err">dial tcp: lookup vault.example.com: no such host</div>
          </header>`,
            },
            {
              stage: "block",
              caption: "The footer: both builds, and the last thing that happened",
              html: `          <footer class="vk-footer"><span>companion v0.9.4 · service v1.9.2</span><span class="vk-footer-last">last action: checked in Ashwood Hollow</span></footer>`,
            },
          ],
          rules: [
            "Three dot states, not two: ok when connected, ember when the last sync errored, mist when no vault is configured at all.",
            "The error text is the transport's own words. “Could not connect” sends a player to the wrong place; a DNS failure names itself.",
            "“Sync now” exists for being certain rather than patient — the page keeps itself current while open, and this is how a player hears out loud that the service cannot be reached.",
          ],
          sources: ["web/companion/src/components/HeaderBar.tsx", "web/companion/src/lib/format.ts"],
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
              html: `          <div class="vk-panelfail">The shelf panel failed: Cannot read properties of undefined (reading 'saveDirs')
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
          name: "The shelf",
          subtitle: "Linked in colour, unlinked greyed",
          viewport: { width: 940, height: 620 },
          intent: `Every game the companion found on this machine. Linked games are in colour
            with a gold ring and their world's name as the caption; unlinked ones are greyed
            and un-grey on hover. Telling the two apart at a glance is the entire point of the
            shelf — everything else on the page is a consequence of a link existing.`,
          specimens: [
            {
              stage: "block",
              caption: "Mixed shelf",
              html: `          <div class="vk-shelf">
${tile("Dragonwilds", "Ashwood Hollow", true)}
${tile("Enshrouded", "Cinderfall", true, " vk-tile--active")}
${tile("Palworld", "not linked", false)}
${tile("Valheim", "not linked", false)}
${tile("Terraria", "hidden", false)}
            <button class="vk-tile vk-tile--add">${ICON.folder}<span>Link a folder by hand</span><span style="color:rgb(var(--goldhi))">show hidden</span></button>
          </div>`,
            },
          ],
          rules: [
            "Tiles are memo'd and keyed by the game's identity. The page polls every five seconds, and an <code>&lt;img&gt;</code> that remounts re-fetches — rebuilding identical tiles made every cover flicker.",
            "The caption of a linked tile is the <i>world's</i> name, not the game's: the link is the fact worth showing.",
            "A tile's tooltip carries the first save directory, so hovering answers “which install is this?” without a click.",
          ],
          sources: ["web/companion/src/components/GameTile.tsx", "web/companion/src/components/Shelf.tsx"],
        },
        {
          slug: "world-row",
          name: "Linked world row",
          subtitle: "The world, the folder, and one action",
          viewport: { width: 940, height: 760 },
          intent: `One linked world: what it is, who holds it, <b>the folder on this machine</b>,
            and the single action its custody state calls for. The folder line is what makes
            this the companion's row and not reliquary's card — it is the only thing here about
            the disk rather than the world.`,
          specimens: [
            {
              stage: "stack",
              caption: "Free — check out, and check out without playing",
              html: worldRow({
                name: "Ashwood Hollow",
                game: "Dragonwilds",
                chip: CHIP.free,
                line: "nobody holds this world",
                dir: "C:\\Users\\hazel\\AppData\\Local\\Dragonwilds\\Saved\\SaveGames\\AshwoodHollow",
                acts: `${btn("primary", "Check out &amp; play")}${btn("quiet", "Check out")}`,
              }),
            },
            {
              stage: "stack",
              caption: "Yours — check in, checkpoint, renew, play",
              html: worldRow({
                name: "Cinderfall",
                game: "Enshrouded",
                chip: CHIP.mine,
                line: "until 21:14 · save is on this machine",
                dir: "C:\\Users\\hazel\\Saved Games\\Enshrouded\\Cinderfall",
                acts: `${btn("primary", "Check in")}${btn("quiet", "Checkpoint now")}${btn("quiet", "Renew hold")}<button class="vk-btn vk-btn--quiet">${sm(ICON.play)}Play</button>`,
              }),
            },
            {
              stage: "stack",
              caption: "Expired — taking over is a confirm, because someone else's work is in the balance",
              html: worldRow({
                name: "Verdant Reach",
                game: "Palworld",
                chip: CHIP.expired,
                line: "held by rook — the hold expired",
                dir: "C:\\Users\\hazel\\Documents\\Palworld\\Saved\\VerdantReach",
                acts: `${btn("primary", "Take over expired hold")}`,
              }),
            },
            {
              stage: "stack",
              caption: "Gone — the link stays so the folder is still identifiable",
              html: worldRow({
                name: "world #14",
                game: "Dragonwilds",
                chip: CHIP.gone,
                line: "world #14 is not on the service any more",
                dir: "C:\\Users\\hazel\\AppData\\Local\\Dragonwilds\\Saved\\SaveGames\\OldWorld",
                acts: `${btn("danger", "Unlink")}`,
              }),
            },
          ],
          rules: [
            "The chip sits at the end of the title row, not before the name: the name identifies, the chip qualifies.",
            "The verbs and their visibility are the old vanilla page's <code>linkedRow()</code>, rule for rule.",
            "“Take over expired hold” confirms with what it costs — the old holder's late check-in is kept and flagged, not lost.",
          ],
          sources: ["web/companion/src/components/WorldRow.tsx", "web/companion/src/components/ConfirmDialog.tsx"],
        },
        {
          slug: "scan-trail",
          name: "Scan trail",
          subtitle: "Where it looked, and what it found",
          viewport: { width: 940, height: 560 },
          intent: `Discovery is a guess about someone else's machine, so it shows its working.
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
          sources: ["web/companion/src/components/ScanTrail.tsx"],
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
