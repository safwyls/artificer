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
        // Literal Palworld palette tokens, for spots that need a specific
        // hue rather than a semantic role (dark sidebar panel, per-category
        // stat colors, rarity accent) — everything else uses the semantic
        // tokens above so existing components didn't need rewriting.
        brand: {
          red: "#E8491D",
          amber: "#F2A93B",
        },
        pal: {
          green: "#4A9D7C",
          blue: "#5B9BD5",
        },
        ink: {
          DEFAULT: "#2B2420",
          light: "#3D342D",
          soft: "#544A40",
          muted: "#5F5850",
        },
        paper: "#F5EDE1",
        legendary: "#8B3A9E",
        // Passive-skill tier colors, pixel-sampled from the game's own tier
        // icons (via game8's passive table): slate is the dark ground the
        // game draws passive rows on; ice/gold/red are the tier 1 / 2–3 /
        // negative chevrons; aqua-on-indigo is the Rainbow tier and
        // aqua-on-violet the World Tree tier.
        tier: {
          slate: "#1B2725",
          ice: "#E9F8FA",
          gold: "#FFE083",
          red: "#FF4649",
          aqua: "#7AFFF2",
          indigo: "#334383",
          violet: "#52359D",
        },
      },
      fontFamily: {
        display: ["Baloo 2", "sans-serif"],
        body: ["Manrope", "sans-serif"],
        mono: ["JetBrains Mono", "monospace"],
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
