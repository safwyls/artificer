// Preview chrome: the page furniture every card wears, whatever the app.
//
// App-agnostic on purpose — the chrome frames a specimen without competing
// with it, so it is drawn entirely from the tokens the app supplies. A card
// for a light-themed console and a card for the vault use the same rules and
// come out looking like the app they describe.

export const svg = (body, cls) =>
  `<svg class="${cls}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${body}</svg>`;

/** The lucide glyphs the specimens use, as inline SVG — no icon package. */
export const icons = (cls) => ({
  lock: svg('<rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>', cls),
  lockOpen: svg('<rect width="18" height="11" x="3" y="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 9.9-1"/>', cls),
  clock: svg('<circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>', cls),
  more: svg('<circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/><circle cx="5" cy="12" r="1"/>', cls),
  globe: svg('<circle cx="12" cy="12" r="10"/><path d="M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20"/><path d="M2 12h20"/>', cls),
  laptop: svg('<path d="M20 16V7a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v9"/><path d="M20 16H4l-1.28 2.55a1 1 0 0 0 .9 1.45h16.76a1 1 0 0 0 .9-1.45z"/>', cls),
  users: svg('<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>', cls),
  image: svg('<rect width="18" height="18" x="3" y="3" rx="2" ry="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.09-3.09a2 2 0 0 0-2.82 0L6 21"/>', cls),
  database: svg('<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14a9 3 0 0 0 18 0V5"/><path d="M3 12a9 3 0 0 0 18 0"/>', cls),
  shield: svg('<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="M9 12h6"/><path d="M12 9v6"/>', cls),
  close: svg('<path d="M18 6 6 18"/><path d="m6 6 12 12"/>', cls),
  refresh: svg('<path d="M3 12a9 9 0 0 1 15-6.7L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-15 6.7L3 16"/><path d="M3 21v-5h5"/>', cls),
  settings: svg('<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.6 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.6h.09A1.65 1.65 0 0 0 10 3.09V3a2 2 0 1 1 4 0v.09A1.65 1.65 0 0 0 15 4.6a1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9v.09a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>', cls),
  arrowUp: svg('<circle cx="12" cy="12" r="10"/><path d="m16 12-4-4-4 4"/><path d="M12 16V8"/>', cls),
  folder: svg('<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z"/>', cls),
  play: svg('<polygon points="6 3 20 12 6 21 6 3"/>', cls),
  dotsVertical: svg('<circle cx="12" cy="5" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="12" cy="19" r="1"/>', cls),
  search: svg('<circle cx="11" cy="11" r="7"/><path d="m20 20-4.3-4.3"/>', cls),
  alert: svg('<path d="m21.7 18-8-14a2 2 0 0 0-3.4 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.7-3"/><path d="M12 9v4"/><path d="M12 17h.01"/>', cls),
  wifiOff: svg('<path d="M2 8.8a16 16 0 0 1 20 0"/><path d="M6.5 12.8a10 10 0 0 1 11 0"/><circle cx="12" cy="19" r="1"/><path d="m2 2 20 20"/>', cls),
});

export const chromeCSS = `
*, *::before, *::after { box-sizing: border-box; }

/* ---- preview chrome -------------------------------------------------- */
.ds-page {
  margin: 0;
  background: var(--ds-bg);
  color: var(--ds-fg);
  font-family: var(--ds-body);
  font-size: 15px;
  line-height: 1.5;
  -webkit-font-smoothing: antialiased;
}
.ds-head { border-bottom: 1px solid var(--ds-line); padding: 26px 32px 20px; }
.ds-eyebrow {
  margin: 0; font-family: var(--ds-mono); font-size: 11px;
  letter-spacing: 0.12em; text-transform: uppercase; color: var(--ds-muted);
}
.ds-title { margin: 6px 0 0; font-size: 22px; font-weight: normal; letter-spacing: 0.05em; color: var(--ds-accent); }
.ds-sub { margin: 3px 0 0; font-size: 13px; color: var(--ds-muted); }
.ds-intent { margin: 14px 0 0; max-width: 70ch; font-size: 14px; color: var(--ds-fg-soft); }
.ds-body { display: flex; flex-direction: column; gap: 28px; padding: 24px 32px 44px; }
.ds-caption {
  margin: 0 0 10px; font-family: var(--ds-mono); font-size: 10px;
  letter-spacing: 0.12em; text-transform: uppercase; color: var(--ds-muted);
}
.ds-stage { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; }
.ds-stage--block { display: block; }
.ds-stage--stack { display: flex; flex-direction: column; align-items: stretch; gap: 12px; }
.ds-stage--well {
  background: var(--ds-raise); border: 1px solid var(--ds-line);
  border-radius: 8px; padding: 18px;
}
.ds-meta { border-top: 1px solid var(--ds-line); padding-top: 16px; }
.ds-meta-h {
  margin: 0 0 8px; font-family: var(--ds-mono); font-size: 10px; font-weight: normal;
  letter-spacing: 0.12em; text-transform: uppercase; color: var(--ds-muted);
}
.ds-paths, .ds-rules { margin: 0; padding-left: 18px; }
.ds-paths li, .ds-rules li { font-size: 13px; color: var(--ds-fg-soft); margin-bottom: 4px; }
.ds-paths li { list-style: none; margin-left: -18px; }
code { font-family: var(--ds-mono); font-size: 12px; color: var(--ds-accent-hi); }
.ds-idx-group + .ds-idx-group { border-top: 1px solid var(--ds-line); padding-top: 20px; }
.ds-idx { margin: 0; padding: 0; list-style: none; display: grid; gap: 6px; }
.ds-idx li { display: flex; flex-wrap: wrap; align-items: baseline; gap: 10px; }
.ds-idx-link { color: var(--ds-accent-hi); text-decoration: none; font-size: 15px; }
.ds-idx-link:hover { color: var(--ds-accent); text-decoration: underline; }
.ds-idx-sub { font-size: 12px; color: var(--ds-muted); }

/* ---- swatches -------------------------------------------------------- */
.ds-swatches { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 12px; }
.ds-swatch { border: 1px solid var(--ds-line); border-radius: 8px; overflow: hidden; background: var(--ds-panel); }
.ds-swatch-chip { height: 58px; border-bottom: 1px solid var(--ds-line); }
.ds-swatch-body { padding: 8px 10px 10px; }
.ds-swatch-name { font-family: var(--ds-mono); font-size: 12px; color: var(--ds-fg); }
.ds-swatch-hex { font-family: var(--ds-mono); font-size: 11px; color: var(--ds-muted); }
.ds-swatch-use { margin-top: 4px; font-size: 12px; color: var(--ds-muted); line-height: 1.35; }
`;
