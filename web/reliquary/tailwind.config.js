/** @type {import('tailwindcss').Config} */
// One deliberate dark look: the vault. No light theme and no toggle, so
// there is exactly one palette — declared once in index.css as CSS
// variables and named here, rather than a light set with dark overrides.
export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {
      // Each token is an RGB triple rather than a hex string so Tailwind's
      // opacity modifiers work on it (border-ember/40, ring-gold/60): a
      // `var(--x)` holding "#d4735e" cannot be given an alpha, a
      // `rgb(var(--x) / <alpha-value>)` can.
      colors: {
        ink: "rgb(var(--ink) / <alpha-value>)",
        panel: "rgb(var(--panel) / <alpha-value>)",
        // The sidebar/inset ground: a shade under the page, so a panel
        // sitting on it still reads as raised.
        well: "rgb(var(--well) / <alpha-value>)",
        edge: "rgb(var(--edge) / <alpha-value>)",
        parchment: "rgb(var(--parchment) / <alpha-value>)",
        mist: "rgb(var(--mist) / <alpha-value>)",
        gold: "rgb(var(--gold) / <alpha-value>)",
        goldhi: "rgb(var(--goldhi) / <alpha-value>)",
        ok: "rgb(var(--ok) / <alpha-value>)",
        ember: "rgb(var(--ember) / <alpha-value>)",
        rune: "rgb(var(--rune) / <alpha-value>)",
      },
      fontFamily: {
        // Two faces, split by what each is for — the same rule the
        // companion follows, because they are two halves of one flow and
        // have to keep reading as one thing.
        //
        // `sans` is the interface: counts, hints, captions, field labels,
        // table cells. Georgia's serifs smear at 12px on a dark ground,
        // and most of what a vault page says is 12px.
        //
        // `serif` is the vault's voice, and stays Georgia: world names,
        // headings, buttons, the mark.
        sans: [
          "Segoe UI Variable Text",
          "Segoe UI",
          "system-ui",
          "-apple-system",
          "Helvetica Neue",
          "Arial",
          "sans-serif",
        ],
        serif: ["Georgia", "Gelasio", "Times New Roman", "serif"],
        mono: ["ui-monospace", "SFMono-Regular", "Menlo", "monospace"],
      },
      borderRadius: {
        panel: "8px",
      },
    },
  },
  plugins: [],
};
