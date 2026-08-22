// The console kit: what palcon, wildskeeper and flametender share.
//
// All three are the same application wearing three faces. Each maps its own
// literal palette onto shadcn's semantic tokens in its `index.css` — with the
// stated reason, in all three files, that "shared components need no
// per-component work" — so the components really are one set, and the theme
// really is only the token values plus a handful of signature flourishes.
//
// That is why this kit is written entirely against the semantic tokens
// (`--primary`, `--card`, `--border`, `--radius`, …) and never against a
// literal. Feed it palcon's values and it comes out light and rounded; feed it
// flametender's and it comes out mossy and square. Each console's `system.mjs`
// adds only its own flourishes: wildskeeper's clipped notches and brass
// corner brackets, flametender's terrace top-light and fog line, palcon's
// Pal Sphere ring.
//
// Class prefix: `con-`.

/** How the preview chrome reads a console's semantic tokens. */
export const consoleChrome = {
  bg: "hsl(var(--background))",
  panel: "hsl(var(--card))",
  raise: "hsl(var(--muted))",
  line: "hsl(var(--border))",
  fg: "hsl(var(--foreground))",
  "fg-soft": "hsl(var(--foreground) / 0.85)",
  muted: "hsl(var(--muted-foreground))",
  accent: "hsl(var(--accent))",
  "accent-hi": "hsl(var(--accent))",
  body: "var(--font-body)",
  mono: "var(--font-mono)",
};

/** Swatch grid over HSL-triple tokens. */
export const hslSwatches = (colors) => (names) =>
  `          <div class="ds-swatches">\n` +
  names
    .map((n) => {
      const t = colors.find((c) => c.name === n);
      return `            <div class="ds-swatch">
              <div class="ds-swatch-chip" style="background: hsl(var(--${t.name}))"></div>
              <div class="ds-swatch-body">
                <div class="ds-swatch-name">--${t.name}</div>
                <div class="ds-swatch-hex">${t.hex}</div>
                <div class="ds-swatch-use">${t.use}</div>
              </div>
            </div>`;
    })
    .join("\n") +
  `\n          </div>`;

/** Swatch grid over the literal palette a console's Tailwind config names. */
export const literalSwatches = (literals) => (names) =>
  `          <div class="ds-swatches">\n` +
  names
    .map((n) => {
      const t = literals.entries.find((c) => c.name === n);
      return `            <div class="ds-swatch">
              <div class="ds-swatch-chip" style="background: var(--${literals.prefix}-${t.name})"></div>
              <div class="ds-swatch-body">
                <div class="ds-swatch-name">${literals.prefix}.${t.name}</div>
                <div class="ds-swatch-hex">${t.hex}</div>
                <div class="ds-swatch-use">${t.use}</div>
              </div>
            </div>`;
    })
    .join("\n") +
  `\n          </div>`;

export const cbtn = (variant, label, extra = "") =>
  `<button class="con-btn con-btn--${variant}${extra}">${label}</button>`;

export const typeRow = (key, html) =>
  `          <div class="con-type-row"><div class="con-type-key">${key}</div><div>${html}</div></div>`;

/** The panel grammar all three draw: a title strip over a faint accent
 *  gradient, an optional right-aligned meta line, and a body. */
export const panel = (title, meta, body, cls = "") =>
  `          <section class="con-panel ${cls}">
            <div class="con-panel-h"><h3 class="con-panel-t">${title}</h3>${
              meta ? `<span class="con-panel-meta">${meta}</span>` : ""
            }</div>
            <div class="con-panel-b">${body}</div>
          </section>`;

/** The stat tile: small-caps label, display-face value, hint, optional meter.
 *  `warm` fills the meter with the structural accent instead of the live one —
 *  a capacity reading is not a liveness reading. */
export const stat = ({ label, value, unit, hint, meter, warm }) =>
  `            <div class="con-stat">
              <div class="con-stat-label">${label}</div>
              <div class="con-stat-value">${value}${unit ? `<small class="con-stat-unit">${unit}</small>` : ""}</div>
              ${hint ? `<div class="con-stat-hint">${hint}</div>` : ""}
              ${
                meter === undefined
                  ? ""
                  : `<div class="con-meter"><i class="con-meter-f${warm ? " con-meter-f--warm" : ""}" style="width:${meter}%"></i></div>`
              }
            </div>`;

export const consoleKit = `
.con-mono { font-family: var(--font-mono); }
.con-display { font-family: var(--font-display); }

/* ---- controls -------------------------------------------------------- */
.con-btn {
  display: inline-flex; align-items: center; justify-content: center; gap: 8px;
  white-space: nowrap; border: 1px solid transparent; border-radius: calc(var(--radius) - 2px);
  height: 36px; padding: 0 16px; font-family: var(--font-body); font-size: 14px;
  font-weight: 500; cursor: pointer; background: none; transition: background-color 0.12s, color 0.12s;
}
.con-btn--default { background: hsl(var(--primary)); color: hsl(var(--primary-foreground)); box-shadow: 0 1px 2px rgb(0 0 0 / 0.2); }
.con-btn--default:hover, .con-btn--default.is-hover { background: hsl(var(--primary) / 0.9); }
.con-btn--destructive { background: hsl(var(--destructive)); color: hsl(var(--destructive-foreground)); box-shadow: 0 1px 2px rgb(0 0 0 / 0.2); }
.con-btn--destructive:hover, .con-btn--destructive.is-hover { background: hsl(var(--destructive) / 0.9); }
.con-btn--outline { border-color: hsl(var(--input)); color: hsl(var(--foreground)); box-shadow: 0 1px 2px rgb(0 0 0 / 0.15); }
.con-btn--outline:hover, .con-btn--outline.is-hover { background: hsl(var(--accent)); color: hsl(var(--accent-foreground)); }
.con-btn--secondary { background: hsl(var(--secondary)); color: hsl(var(--secondary-foreground)); box-shadow: 0 1px 2px rgb(0 0 0 / 0.15); }
.con-btn--secondary:hover, .con-btn--secondary.is-hover { background: hsl(var(--secondary) / 0.8); }
.con-btn--ghost { color: hsl(var(--foreground)); }
.con-btn--ghost:hover, .con-btn--ghost.is-hover { background: hsl(var(--accent)); color: hsl(var(--accent-foreground)); }
.con-btn--link { color: hsl(var(--primary)); text-underline-offset: 4px; }
.con-btn--link:hover, .con-btn--link.is-hover { text-decoration: underline; }
.con-btn--sm { height: 32px; padding: 0 12px; font-size: 12px; }
.con-btn--lg { height: 40px; padding: 0 32px; }
.con-btn--icon { height: 36px; width: 36px; padding: 0; }
.con-btn[disabled], .con-btn.is-disabled { opacity: 0.5; pointer-events: none; }
.con-btn svg { width: 16px; height: 16px; flex: none; }

.con-input {
  width: 100%; height: 36px; border: 1px solid hsl(var(--input)); border-radius: calc(var(--radius) - 2px);
  background: transparent; padding: 0 12px; font-family: var(--font-body); font-size: 14px;
  color: hsl(var(--foreground));
}
.con-input::placeholder { color: hsl(var(--muted-foreground)); }
.con-input:focus-visible, .con-btn:focus-visible {
  outline: none; box-shadow: 0 0 0 1px hsl(var(--ring));
}
.con-label { display: block; font-size: 13px; font-weight: 500; color: hsl(var(--foreground)); }

.con-badge {
  display: inline-flex; align-items: center; border-radius: 999px; border: 1px solid transparent;
  padding: 2px 10px; font-size: 11px; font-weight: 600; letter-spacing: 0.02em;
}
.con-badge--default { background: hsl(var(--primary)); color: hsl(var(--primary-foreground)); }
.con-badge--secondary { background: hsl(var(--secondary)); color: hsl(var(--secondary-foreground)); }
.con-badge--outline { border-color: hsl(var(--border)); color: hsl(var(--foreground)); }
.con-badge--destructive { background: hsl(var(--destructive)); color: hsl(var(--destructive-foreground)); }

/* ---- surfaces -------------------------------------------------------- */
.con-card {
  border: 1px solid hsl(var(--border)); background: hsl(var(--card)); color: hsl(var(--card-foreground));
  border-radius: var(--radius); box-shadow: 0 1px 2px rgb(0 0 0 / 0.12);
}
.con-card-h { display: flex; flex-direction: column; gap: 6px; padding: 16px; }
.con-card-t { margin: 0; font-size: 14px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.04em; color: hsl(var(--muted-foreground)); }
.con-card-b { padding: 0 16px 16px; }

.con-panel {
  overflow: hidden; border: 1px solid hsl(var(--border)); background: hsl(var(--card));
  border-radius: calc(var(--radius) + 2px);
}
.con-panel-h {
  display: flex; align-items: baseline; justify-content: space-between; gap: 12px;
  border-bottom: 1px solid hsl(var(--border));
  background: linear-gradient(to bottom, hsl(var(--accent) / 0.05), transparent);
  padding: 12px 16px;
}
.con-panel-t { margin: 0; font-size: 13px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.08em; color: hsl(var(--accent)); }
.con-panel-meta { font-size: 12px; color: hsl(var(--muted-foreground)); }
.con-panel-b { padding: 14px 16px; }
.con-note { margin: 10px 0 0; font-size: 12px; font-style: italic; color: hsl(var(--muted-foreground)); }

.con-stats { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 12px; }
.con-stat {
  border: 1px solid hsl(var(--border)); background: hsl(var(--card));
  border-radius: calc(var(--radius) + 2px); padding: 14px 16px 12px;
}
.con-stat-label { font-size: 11px; text-transform: uppercase; letter-spacing: 0.14em; color: hsl(var(--muted-foreground)); }
.con-stat-value { margin-top: 6px; font-family: var(--font-display); font-size: 26px; font-weight: 600; line-height: 1.05; color: hsl(var(--foreground)); }
.con-stat-unit { margin-left: 4px; font-family: var(--font-body); font-size: 14px; font-weight: 400; color: hsl(var(--muted-foreground)); }
.con-stat-hint { margin-top: 2px; font-size: 12px; color: hsl(var(--muted-foreground)); }
.con-meter { margin-top: 10px; height: 5px; overflow: hidden; border-radius: 3px; background: var(--meter-track); }
.con-meter-f { display: block; height: 100%; background: linear-gradient(to right, var(--live-dim), var(--live)); }
.con-meter-f--warm { background: linear-gradient(to right, var(--warm-dim), var(--warm)); }

/* ---- the rail and the sub-nav ---------------------------------------- */
.con-shell { display: flex; border: 1px solid hsl(var(--border)); border-radius: var(--radius); overflow: hidden; min-height: 320px; }
.con-rail {
  display: flex; width: 72px; flex: none; flex-direction: column; align-items: center; gap: 12px;
  border-right: 1px solid rgb(0 0 0 / 0.2); background: var(--rail-bg); padding: 16px 0;
}
.con-coin {
  display: flex; height: 44px; width: 44px; align-items: center; justify-content: center;
  border: 2px solid; border-radius: 999px; font-family: var(--font-display); font-size: 16px;
  font-weight: 600; cursor: pointer; background: none; transition: border-color 0.12s, color 0.12s;
}
.con-coin--idle { border-color: var(--coin-idle-border); background: var(--coin-idle-bg); color: var(--coin-idle-fg); }
.con-coin--idle:hover { border-color: var(--coin-hi); color: hsl(var(--foreground)); }
.con-coin--active { border-color: var(--live); background: hsl(var(--card)); color: hsl(var(--foreground)); box-shadow: var(--coin-glow); }
.con-coin--add { border-style: dashed; border-color: rgb(255 255 255 / 0.2); color: var(--coin-idle-fg); background: none; }
.con-coin--brand { height: 36px; width: 36px; border: 0; background: var(--brand-mark); color: var(--brand-mark-fg); font-size: 14px; }
.con-rail-spacer { flex: 1; }

.con-main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
.con-subnav { display: flex; gap: 4px; border-bottom: 1px solid hsl(var(--border)); padding: 10px 20px; overflow-x: auto; }
.con-subnav-item {
  white-space: nowrap; border-radius: 999px; padding: 5px 14px; font-size: 13px;
  color: hsl(var(--muted-foreground)); text-decoration: none; cursor: pointer;
}
.con-subnav-item.is-active { background: hsl(var(--secondary)); color: hsl(var(--foreground)); }
.con-body { display: flex; flex-direction: column; gap: 14px; padding: 18px 20px; }

/* ---- logs, tables, dialogs ------------------------------------------- */
.con-log {
  border: 1px solid hsl(var(--border)); background: var(--log-bg); border-radius: calc(var(--radius) - 2px);
  padding: 10px 12px; font-family: var(--font-mono); font-size: 12px; line-height: 1.55;
}
.con-log-line { white-space: pre-wrap; word-break: break-all; color: hsl(var(--foreground) / 0.7); }
.con-log-line--error { color: var(--danger); }
.con-log-line--warn { color: hsl(var(--accent)); }
.con-log-line--join { color: var(--live); }
.con-log-line--save { color: var(--ok); }

.con-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.con-table th {
  text-align: left; padding: 8px 12px; border-bottom: 1px solid hsl(var(--border));
  font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.08em;
  color: hsl(var(--muted-foreground));
}
.con-table td { padding: 9px 12px; border-bottom: 1px solid hsl(var(--border) / 0.6); }
.con-table tr:last-child td { border-bottom: 0; }

.con-dialog {
  position: relative; width: 100%; max-width: 30rem; border: 1px solid hsl(var(--border));
  background: hsl(var(--popover)); color: hsl(var(--popover-foreground));
  border-radius: var(--radius); padding: 22px; box-shadow: 0 25px 50px -12px rgb(0 0 0 / 0.5);
}
.con-dialog-t { margin: 0; font-size: 17px; font-weight: 600; }
.con-dialog-b { margin: 8px 0 0; font-size: 14px; color: hsl(var(--muted-foreground)); }
.con-dialog-f { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }

.con-dot { display: inline-block; width: 8px; height: 8px; border-radius: 999px; }
.con-dot--live { background: var(--live); }
.con-dot--ok { background: var(--ok); }
.con-dot--down { background: hsl(var(--muted-foreground)); }
.con-dot--danger { background: var(--danger); }

.con-type-row { display: flex; align-items: baseline; gap: 18px; border-bottom: 1px solid hsl(var(--border)); padding: 12px 0; }
.con-type-row:last-child { border-bottom: 0; }
.con-type-key { width: 200px; flex: none; font-family: var(--font-mono); font-size: 11px; color: hsl(var(--muted-foreground)); }
`;
