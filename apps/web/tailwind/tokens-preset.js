// Interim local Tailwind preset derived from shared/tokens/tokens.json.
// Agent-Tooling owns the canonical shared/tokens/tailwind-preset.js.
// When that lands, apps/web/tailwind.config.js swaps to:
//   presets: [require('@learn/shared-tokens/tailwind-preset')]
// and this file is deleted. Tracked in issue #16.

const tokens = require("../../../shared/tokens/tokens.json");

const color = tokens.color;
const type = tokens.type;

/** @type {import('tailwindcss').Config} */
module.exports = {
  theme: {
    extend: {
      colors: {
        learn: {
          substrate: color.surface.substrate,
          raised: color.surface.raised,
          sunken: color.surface.sunken,
          divider: color.surface.divider,
          ink: {
            primary: color.ink.primary,
            secondary: color.ink.secondary,
            tertiary: color.ink.tertiary,
            quiet: color.ink.quiet,
          },
          flame: {
            DEFAULT: color.flame.primary,
            primary: color.flame.primary,
            edge: color.flame.edge,
            glow: color.flame.glow,
          },
          success: color.signal.success,
          attention: color.signal.attention,
          burgundyTop: color.chromatic.burgundyTop,
          burgundyMid: color.chromatic.burgundyMid,
          burgundyBottom: color.chromatic.burgundyBottom,
          warm: color.chromatic.warm,
          warmHi: color.chromatic.warmHi,
          ember: color.chromatic.ember,
          orange: color.chromatic.orange,
          rust: color.chromatic.rust,
          deep: color.chromatic.deep,
        },
      },
      fontFamily: {
        sans: type.family.sans.split(",").map((s) => s.trim()),
        mono: type.family.mono.split(",").map((s) => s.trim()),
      },
      fontSize: {
        "learn-display": [type.role.display.size, { lineHeight: type.role.display.lineHeight, letterSpacing: type.role.display.tracking }],
        "learn-title": [type.role.title.size, { lineHeight: type.role.title.lineHeight, letterSpacing: type.role.title.tracking }],
        "learn-body": [type.role.body.size, { lineHeight: type.role.body.lineHeight, letterSpacing: type.role.body.tracking }],
        "learn-meta": [type.role.meta.size, { lineHeight: type.role.meta.lineHeight, letterSpacing: type.role.meta.tracking }],
        "learn-caps": [type.role.caps.size, { lineHeight: type.role.caps.lineHeight, letterSpacing: type.role.caps.tracking }],
        "learn-mono": [type.role.mono.size, { lineHeight: type.role.mono.lineHeight, letterSpacing: type.role.mono.tracking }],
      },
      spacing: {
        "learn-2": tokens.space["2"],
        "learn-3": tokens.space["3"],
        "learn-4": tokens.space["4"],
        "learn-6": tokens.space["6"],
        "learn-8": tokens.space["8"],
        "learn-12": tokens.space["12"],
        "learn-16": tokens.space["16"],
      },
      borderRadius: {
        "learn-chip": tokens.radius.chip,
        "learn-card": tokens.radius.card,
        "learn-pill": tokens.radius.pill,
      },
      boxShadow: {
        "learn-lift": tokens.shadow.lift,
        "learn-glow": tokens.shadow.glow,
      },
      backgroundImage: {
        "learn-burgundy": `linear-gradient(180deg, ${color.chromatic.burgundyTop} 0%, ${color.chromatic.burgundyMid} 60%, ${color.chromatic.burgundyBottom} 100%)`,
        "learn-flame": `linear-gradient(135deg, ${color.flame.glow} 0%, ${color.flame.primary} 50%, ${color.flame.edge} 100%)`,
      },
      transitionTimingFunction: {
        "learn-enter": tokens.motion.curve.enter,
        "learn-exit": tokens.motion.curve.exit,
        "learn-ambient": tokens.motion.curve.ambient,
      },
      transitionDuration: {
        "learn-snap": tokens.motion.duration.snap.replace("ms", ""),
        "learn-base": tokens.motion.duration.base.replace("ms", ""),
        "learn-settle": tokens.motion.duration.settle.replace("ms", ""),
        "learn-moment": tokens.motion.duration.moment.replace("ms", ""),
      },
    },
  },
  plugins: [],
};
