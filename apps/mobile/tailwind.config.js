// Tailwind config consumed by NativeWind.
// Colors, type, spacing, motion come from shared/tokens.
const tokens = require("@learn/shared-tokens/json");

/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./app/**/*.{ts,tsx,js,jsx}",
    "./components/**/*.{ts,tsx,js,jsx}",
    "./lib/**/*.{ts,tsx,js,jsx}",
  ],
  presets: [require("nativewind/preset")],
  theme: {
    extend: {
      colors: {
        substrate: tokens.color.surface.substrate,
        raised: tokens.color.surface.raised,
        sunken: tokens.color.surface.sunken,
        divider: tokens.color.surface.divider,
        ink: {
          primary: tokens.color.ink.primary,
          secondary: tokens.color.ink.secondary,
          tertiary: tokens.color.ink.tertiary,
          quiet: tokens.color.ink.quiet,
        },
        flame: {
          primary: tokens.color.flame.primary,
          edge: tokens.color.flame.edge,
          glow: tokens.color.flame.glow,
        },
        signal: {
          success: tokens.color.signal.success,
          attention: tokens.color.signal.attention,
        },
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "SF Mono", "monospace"],
      },
      borderRadius: {
        chip: tokens.radius.chip,
        card: tokens.radius.card,
        pill: tokens.radius.pill,
      },
      spacing: tokens.space,
    },
  },
  plugins: [],
};
