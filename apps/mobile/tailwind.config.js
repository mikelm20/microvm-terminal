const tokens = require("../../shared/tokens/tokens.json");

/** Derive a Tailwind color map from the token palette. */
function buildColors() {
  return {
    substrate: tokens.color.surface.substrate,
    raised: tokens.color.surface.raised,
    sunken: tokens.color.surface.sunken,
    divider: tokens.color.surface.divider,
    "ink-primary": tokens.color.ink.primary,
    "ink-secondary": tokens.color.ink.secondary,
    "ink-tertiary": tokens.color.ink.tertiary,
    "ink-quiet": tokens.color.ink.quiet,
    "flame-primary": tokens.color.flame.primary,
    "flame-edge": tokens.color.flame.edge,
    "flame-glow": tokens.color.flame.glow,
    success: tokens.color.signal.success,
    attention: tokens.color.signal.attention,
    "burgundy-top": tokens.color.chromatic.burgundyTop,
    "burgundy-mid": tokens.color.chromatic.burgundyMid,
    "burgundy-bottom": tokens.color.chromatic.burgundyBottom,
    ember: tokens.color.chromatic.ember,
    deep: tokens.color.chromatic.deep,
    warm: tokens.color.chromatic.warm,
    "warm-hi": tokens.color.chromatic.warmHi,
  };
}

/** @type {import('tailwindcss').Config} */
module.exports = {
  content: [
    "./app/**/*.{js,jsx,ts,tsx}",
    "./components/**/*.{js,jsx,ts,tsx}",
  ],
  presets: [require("nativewind/preset")],
  theme: {
    extend: {
      colors: buildColors(),
      borderRadius: {
        chip: tokens.radius.chip,
        card: tokens.radius.card,
        pill: tokens.radius.pill,
      },
      fontFamily: {
        sans: tokens.type.family.sans.split(",").map((s) => s.trim()),
        mono: tokens.type.family.mono.split(",").map((s) => s.trim()),
      },
      spacing: {
        "space-2": tokens.space["2"],
        "space-3": tokens.space["3"],
        "space-4": tokens.space["4"],
        "space-6": tokens.space["6"],
        "space-8": tokens.space["8"],
        "space-12": tokens.space["12"],
        "space-16": tokens.space["16"],
      },
    },
  },
  plugins: [],
};
