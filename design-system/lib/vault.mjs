// The vault: the design language reliquary and the Artificer Companion share.
//
// They are two halves of one system — the service holds the worlds, the
// companion moves them — so they wear one palette and one set of primitives.
// `web/reliquary/src/index.css` and `web/companion/src/index.css` declare the
// same eleven tokens, word for word and comment for comment; keeping them in
// one module here is what makes that shared identity visible rather than a
// coincidence two files happen to agree on.
//
// `docs/reliquary-ui-rebuild.md` §"Design language" is normative for both.
//
// Class prefix: `vk-`. The vault kit, not either app.

/** The palette. `value` is the declaration form both apps use — an RGB triple
 *  rather than a hex string, so Tailwind's opacity modifiers work on it
 *  (`border-ember/40`, `ring-gold/60`): a `var(--x)` holding "#d4735e" cannot
 *  be given an alpha, an `rgb(var(--x) / <alpha-value>)` can. */
export const vaultColors = [
  { name: "ink", value: "16 13 23", hex: "#100d17", use: "the page ground" },
  { name: "well", value: "20 16 29", hex: "#14101d", use: "sidebar, header and inset strips — a shade under the page, so a panel on it still reads as raised" },
  { name: "panel", value: "26 21 36", hex: "#1a1524", use: "cards, dialogs, menus" },
  { name: "edge", value: "47 39 64", hex: "#2f2740", use: "every border and divider" },
  { name: "parchment", value: "232 224 207", hex: "#e8e0cf", use: "body text" },
  { name: "mist", value: "148 141 163", hex: "#948da3", use: "secondary text, quiet buttons, labels" },
  { name: "gold", value: "201 168 96", hex: "#c9a860", use: "accent, headings, the primary button, the focus ring" },
  { name: "goldhi", value: "227 198 127", hex: "#e3c67f", use: "accent hover, links, the HEAD badge" },
  { name: "ok", value: "127 196 106", hex: "#7fc46a", use: "free custody, the live dot, success" },
  { name: "ember", value: "212 115 94", hex: "#d4735e", use: "danger, conflict, an expired hold" },
  { name: "rune", value: "157 127 196", hex: "#9d7fc4", use: "game tags, info, the CHECKPOINT badge" },
];

/** Compounds of the palette rather than members of it: each is an accent laid
 *  over the ground at low weight. These are the only literal colors either app
 *  writes outside its `:root`. */
export const vaultDerived = {
  mono: "ui-monospace, SFMono-Regular, Menlo, monospace",
  // The vault's voice: names, headings, actions. The page's own face is
  // the interface one (vaultChrome.body) — there is far more small
  // secondary text than there is voice, and Georgia's serifs smear at
  // 12px on a dark ground. Both apps follow the same split, because they
  // are two halves of one flow rather than two products that match.
  voice: 'Georgia, Gelasio, "Times New Roman", serif',
  "primary-fill": "linear-gradient(to bottom, #2a2416, #1e1a10)",
  "fill-free": "#14200f",
  "fill-held": "#23180c",
  "fill-expired": "#26130e",
  "fill-cover": "linear-gradient(to bottom right, #221b2e, #14101d)",
  "fill-login": "radial-gradient(ellipse at 50% 30%, #1a1524 0%, #100d17 65%)",
};

/** How the preview chrome reads the vault's tokens. */
export const vaultChrome = {
  bg: "rgb(var(--ink))",
  panel: "rgb(var(--panel))",
  raise: "rgb(var(--well))",
  line: "rgb(var(--edge))",
  fg: "rgb(var(--parchment))",
  "fg-soft": "rgb(var(--parchment) / 0.9)",
  muted: "rgb(var(--mist))",
  accent: "rgb(var(--gold))",
  "accent-hi": "rgb(var(--goldhi))",
  body: '"Segoe UI Variable Text", "Segoe UI", system-ui, -apple-system, "Helvetica Neue", Arial, sans-serif',
  mono: "var(--mono)",
};

/** Swatch grid for a Foundations palette card. */
export const swatchesFrom = (colors) => (names) =>
  `          <div class="ds-swatches">\n` +
  names
    .map((n) => {
      const t = colors.find((c) => c.name === n);
      return `            <div class="ds-swatch">
              <div class="ds-swatch-chip" style="background: rgb(var(--${t.name}))"></div>
              <div class="ds-swatch-body">
                <div class="ds-swatch-name">--${t.name}</div>
                <div class="ds-swatch-hex">${t.hex}</div>
                <div class="ds-swatch-use">${t.use}</div>
              </div>
            </div>`;
    })
    .join("\n") +
  `\n          </div>`;

export const swatches = swatchesFrom(vaultColors);

export const derivedSwatches = (derived) =>
  `          <div class="ds-swatches">\n` +
  Object.entries(derived)
    .filter(([n]) => n !== "mono")
    .map(
      ([n, v]) => `            <div class="ds-swatch">
              <div class="ds-swatch-chip" style="background: var(--${n})"></div>
              <div class="ds-swatch-body">
                <div class="ds-swatch-name">--${n}</div>
                <div class="ds-swatch-hex">${v.length > 34 ? v.slice(0, 32) + "…" : v}</div>
              </div>
            </div>`,
    )
    .join("\n") +
  `\n          </div>`;

/** A button, as both apps draw it. `extra` carries size and state classes. */
export const btn = (variant, label, extra = "") =>
  `<button class="vk-btn vk-btn--${variant}${extra}">${label}</button>`;

export const typeRow = (key, html) =>
  `          <div class="vk-type-row"><div class="vk-type-key">${key}</div><div>${html}</div></div>`;

export const vaultKit = `
/* ---- vault primitives -------------------------------------------- */
.vk-i { width: 14px; height: 14px; flex: none; }
.vk-i--sm { width: 12px; height: 12px; }
.vk-i--lg { width: 16px; height: 16px; }
.vk-mono { font-family: var(--mono); }

.vk-panel {
  border: 1px solid rgb(var(--edge)); background: rgb(var(--panel));
  border-radius: 8px; padding: 16px 20px;
}
.vk-well {
  border: 1px dashed rgb(var(--edge)); background: rgb(var(--well));
  border-radius: 8px; padding: 14px 20px; font-size: 13px; color: rgb(var(--mist));
}
.vk-h1 { margin: 0; font-size: 22px; font-weight: normal; letter-spacing: 0.05em; color: rgb(var(--gold)); }
.vk-label { display: block; font-size: 11px; text-transform: uppercase; letter-spacing: 0.1em; color: rgb(var(--mist)); }
.vk-input {
  width: 100%; border: 1px solid rgb(var(--edge)); border-radius: 4px;
  background: rgb(var(--ink)); padding: 8px 10px; font-family: inherit;
  font-size: 14px; color: rgb(var(--parchment));
}
.vk-input::placeholder { color: rgb(var(--mist) / 0.6); }
.vk-input:focus-visible, .vk-btn:focus-visible, .vk-navitem:focus-visible {
  outline: none; box-shadow: 0 0 0 2px rgb(var(--ink)), 0 0 0 4px rgb(var(--gold) / 0.6);
}

.vk-btn {
  font-family: var(--voice);
  display: inline-flex; align-items: center; justify-content: center; gap: 8px;
  border: 1px solid transparent; border-radius: 4px; white-space: nowrap;
  font-family: inherit; padding: 6px 16px; font-size: 13px; cursor: pointer;
  background: none; transition: color 0.12s ease, border-color 0.12s ease;
}
.vk-btn--primary {
  border-color: rgb(var(--gold)); background: var(--primary-fill);
  font-weight: 700; letter-spacing: 0.04em; color: rgb(var(--gold));
}
.vk-btn--primary:hover { color: rgb(var(--goldhi)); border-color: rgb(var(--goldhi)); }
.vk-btn--quiet { border-color: rgb(var(--edge)); color: rgb(var(--mist)); }
.vk-btn--quiet:hover { border-color: rgb(var(--gold) / 0.6); color: rgb(var(--parchment)); }
.vk-btn--danger { border-color: rgb(var(--ember) / 0.4); color: rgb(var(--mist)); }
.vk-btn--danger:hover { border-color: rgb(var(--ember)); color: rgb(var(--ember)); }
.vk-btn--sm { padding: 4px 12px; font-size: 12px; }
.vk-btn--lg { padding: 8px 18px; font-size: 14px; }
.vk-btn--icon { padding: 6px 10px; }
.vk-btn[disabled], .vk-btn.is-disabled { opacity: 0.5; pointer-events: none; }
.vk-btn.is-hover.vk-btn--primary { color: rgb(var(--goldhi)); border-color: rgb(var(--goldhi)); }
.vk-btn.is-hover.vk-btn--quiet { border-color: rgb(var(--gold) / 0.6); color: rgb(var(--parchment)); }
.vk-btn.is-hover.vk-btn--danger { border-color: rgb(var(--ember)); color: rgb(var(--ember)); }

.vk-badge {
  display: inline-block; border-radius: 3px; background: rgb(var(--ink));
  padding: 2px 8px; font-family: var(--mono); font-size: 10px; letter-spacing: 0.06em;
}
.vk-badge--head { color: rgb(var(--goldhi)); }
.vk-badge--conflict { color: rgb(var(--ember)); }
.vk-badge--checkpoint { color: rgb(var(--rune)); }
.vk-badge--muted { color: rgb(var(--mist)); }

.vk-chip {
  display: inline-flex; align-items: center; gap: 6px; border: 1px solid;
  border-radius: 999px; padding: 2px 12px; font-size: 12px;
}
.vk-chip--free { border-color: rgb(var(--ok)); background: var(--fill-free); color: rgb(var(--ok)); }
.vk-chip--held { border-color: rgb(var(--gold)); background: var(--fill-held); color: rgb(var(--goldhi)); }
.vk-chip--expired { border-color: rgb(var(--ember)); background: var(--fill-expired); color: rgb(var(--ember)); }

.vk-menu {
  min-width: 13rem; border: 1px solid rgb(var(--edge)); background: rgb(var(--panel));
  border-radius: 4px; padding: 4px 0; font-size: 13px;
  box-shadow: 0 20px 25px -5px rgb(0 0 0 / 0.4), 0 8px 10px -6px rgb(0 0 0 / 0.4);
}
.vk-menuitem { padding: 6px 12px; color: rgb(var(--mist)); cursor: pointer; }
.vk-menuitem.is-highlighted { background: rgb(var(--ink)); color: rgb(var(--parchment)); }
.vk-menuitem--danger.is-highlighted { background: rgb(var(--ember) / 0.1); color: rgb(var(--ember)); }
.vk-menusep { height: 1px; margin: 4px 0; background: rgb(var(--edge)); }

.vk-dialog {
  width: 100%; max-width: 28rem; position: relative;
  border: 1px solid rgb(var(--edge)); background: rgb(var(--panel));
  border-radius: 8px; padding: 24px; color: rgb(var(--parchment));
  box-shadow: 0 25px 50px -12px rgb(0 0 0 / 0.6);
}
.vk-dialog-title { margin: 0; font-size: 13px; font-weight: normal; text-transform: uppercase; letter-spacing: 0.12em; color: rgb(var(--gold)); }
.vk-dialog-body { margin: 12px 0 0; font-size: 14px; color: rgb(var(--parchment) / 0.9); }
.vk-dialog-foot { display: flex; justify-content: flex-end; gap: 8px; margin-top: 24px; }
.vk-dialog-x { position: absolute; right: 16px; top: 16px; color: rgb(var(--mist)); background: none; border: 0; cursor: pointer; padding: 0; }

.vk-toast {
  display: flex; flex-direction: column; gap: 2px; min-width: 18rem;
  border: 1px solid rgb(var(--edge)); background: rgb(var(--panel));
  border-radius: 4px; padding: 12px 14px; font-size: 13px; color: rgb(var(--parchment));
}
.vk-toast--success { border-color: rgb(var(--ok) / 0.5); }
.vk-toast--error { border-color: rgb(var(--ember) / 0.6); }
.vk-toast-desc { color: rgb(var(--mist)); }

.vk-dot { display: inline-block; width: 7px; height: 7px; border-radius: 999px; }
.vk-dot--live { background: rgb(var(--ok)); }
.vk-dot--down { background: rgb(var(--mist)); }

.vk-tableshell { overflow: hidden; border: 1px solid rgb(var(--edge)); background: rgb(var(--panel)); border-radius: 8px; }
.vk-tablehead {
  border-bottom: 1px solid rgb(var(--edge)); padding: 10px 18px;
  font-size: 10px; text-transform: uppercase; letter-spacing: 0.12em; color: rgb(var(--mist));
}
.vk-tablerow { display: flex; align-items: center; gap: 14px; padding: 12px 18px; border-bottom: 1px solid rgb(var(--edge)); }
.vk-tablerow:last-child { border-bottom: 0; }

.vk-cover { flex: none; border: 1px solid rgb(var(--edge)); border-radius: 5px; width: 84px; height: 112px; object-fit: cover; }
.vk-cover--fallback {
  display: flex; align-items: center; justify-content: center; text-align: center;
  background: var(--fill-cover); padding: 6px; font-size: 11px; line-height: 1.2; color: rgb(var(--mist));
}
.vk-cover--detail { width: 96px; height: 128px; }

.vk-type-row { display: flex; align-items: baseline; gap: 18px; border-bottom: 1px solid rgb(var(--edge)); padding: 12px 0; }
.vk-type-row:last-child { border-bottom: 0; }
.vk-type-key { width: 190px; flex: none; font-family: var(--mono); font-size: 11px; color: rgb(var(--mist)); }
.vk-focus-demo { display: inline-flex; }
`;
