# Reliquary icon — install

Files in this project mirror their repo paths, so they can be copied straight over:

    web/reliquary/public/favicon.svg              32-grid mark on a rounded ink tile
    web/reliquary/public/mark.svg                 same mark, transparent (for in-app use)
    web/reliquary/public/apple-touch-icon-180.png
    web/reliquary/public/icon-512.png

Vite serves `public/` at the web root. Add to `web/reliquary/index.html`, next to the
existing `theme-color` meta:

```html
<link rel="icon" type="image/svg+xml" href="/favicon.svg" />
<link rel="apple-touch-icon" sizes="180x180" href="/apple-touch-icon-180.png" />
```

`web/reliquary/embed.go` embeds the built `dist`, so no Go change is needed.

Optional: replace the `ShieldPlus` login mark in `src/pages/Login.tsx` with the same
geometry, so the login screen and the tab agree. `mark.svg` uses `#c9a860` directly;
swap the fills/strokes to `currentColor` if you inline it as a component under
`text-gold`.

Note: the mark carries three beads and a two-line cornice, which is dense for 16px.
`favicon.svg` is the version to judge — if it reads muddy in your tab bar, the
straightforward simplification is dropping the thin band under the cornice.
