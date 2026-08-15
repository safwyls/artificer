/** @type {import('tailwindcss').Config} */
export default {
  darkMode: ["class"],
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    container: {
      center: true,
      padding: "1rem",
    },
    extend: {
      colors: {
        border: "hsl(var(--border))",
        input: "hsl(var(--input))",
        ring: "hsl(var(--ring))",
        background: "hsl(var(--background))",
        foreground: "hsl(var(--foreground))",
        primary: {
          DEFAULT: "hsl(var(--primary))",
          foreground: "hsl(var(--primary-foreground))",
        },
        secondary: {
          DEFAULT: "hsl(var(--secondary))",
          foreground: "hsl(var(--secondary-foreground))",
        },
        destructive: {
          DEFAULT: "hsl(var(--destructive))",
          foreground: "hsl(var(--destructive-foreground))",
        },
        muted: {
          DEFAULT: "hsl(var(--muted))",
          foreground: "hsl(var(--muted-foreground))",
        },
        accent: {
          DEFAULT: "hsl(var(--accent))",
          foreground: "hsl(var(--accent-foreground))",
        },
        popover: {
          DEFAULT: "hsl(var(--popover))",
          foreground: "hsl(var(--popover-foreground))",
        },
        card: {
          DEFAULT: "hsl(var(--card))",
          foreground: "hsl(var(--card-foreground))",
        },
        // Flametender literal palette, verbatim from docs/design.md.
        // The rule from the plan: stone is structure and interaction,
        // flame is *reserved* for live/active state and focus, spore for
        // danger. Semantic roles are mapped in index.css; these tokens
        // are for the spots that need the exact hue.
        ft: {
          void: "#101512",
          fog: "#182019",
          panel: "#1e2822",
          edge: "#2d3b32",
          stone: "#98917c",
          stonehi: "#c6bda4",
          flame: "#7fc3f0",
          flamehi: "#d3ecff",
          flamedim: "#31536b",
          spore: "#c95a4d",
          sporedim: "#66312a",
          bone: "#e2ded0",
          lichen: "#87947e",
          ok: "#82b378",
        },
      },
      fontFamily: {
        display: ["'Grenze Gotisch'", "serif"],
        body: ["Karla", "system-ui", "sans-serif"],
        mono: ["'IBM Plex Mono'", "monospace"],
        // Aliases kept so page code can name the theme explicitly.
        ftdisplay: ["'Grenze Gotisch'", "serif"],
        ftbody: ["Karla", "system-ui", "sans-serif"],
      },
      borderRadius: {
        lg: "var(--radius)",
        md: "calc(var(--radius) - 2px)",
        sm: "calc(var(--radius) - 4px)",
      },
      keyframes: {
        "accordion-down": {
          from: { height: "0" },
          to: { height: "var(--radix-accordion-content-height)" },
        },
        "accordion-up": {
          from: { height: "var(--radix-accordion-content-height)" },
          to: { height: "0" },
        },
      },
      animation: {
        "accordion-down": "accordion-down 0.2s ease-out",
        "accordion-up": "accordion-up 0.2s ease-out",
      },
    },
  },
  plugins: [require("tailwindcss-animate")],
};
