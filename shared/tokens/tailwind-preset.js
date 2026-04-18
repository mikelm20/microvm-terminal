// Tailwind preset generated from tokens.json.
// Source of truth: shared/tokens/tokens.json. Do not hard-code values elsewhere.
//
// Consumed via:
//   presets: [require("@learn/shared-tokens/tailwind-preset")]
// in apps/web/tailwind.config.js and the NativeWind config in apps/mobile.
//
// CommonJS, because tailwind.config.js in NativeWind is evaluated as CJS.

const tokens = require("./tokens.json");

function flatten(prefix, obj, out) {
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}-${k}` : k;
    if (v && typeof v === "object" && !Array.isArray(v)) {
      flatten(key, v, out);
    } else {
      out[key] = v;
    }
  }
  return out;
}

const colorTree = {
  surface: tokens.color.surface,
  ink: tokens.color.ink,
  flame: tokens.color.flame,
  signal: tokens.color.signal,
  chromatic: tokens.color.chromatic,
};

// Tailwind expects a nested color object. Each leaf becomes a class like
// `bg-surface-substrate`, `text-ink-primary`, `border-flame-edge`, etc.
const colors = {};
for (const [group, entries] of Object.entries(colorTree)) {
  colors[group] = { ...entries };
}

// Custom font sizes keyed by role. Consumed as `text-display-role`, etc.
// We namespace with `-role` to avoid colliding with Tailwind's default scale.
const fontSize = {};
for (const [role, spec] of Object.entries(tokens.type.role)) {
  fontSize[`${role}-role`] = [
    spec.size,
    {
      lineHeight: spec.lineHeight,
      letterSpacing: spec.tracking,
      fontWeight: String(spec.weight),
    },
  ];
}

const fontFamily = {
  sans: tokens.type.family.sans.split(",").map((s) => s.trim()),
  mono: tokens.type.family.mono.split(",").map((s) => s.trim()),
};

const spacing = { ...tokens.space };

const borderRadius = { ...tokens.radius };

const boxShadow = { ...tokens.shadow };

// Motion tokens are exposed as both CSS variables (via theme.extend) and as
// Tailwind utilities via custom transition-duration + transition-timing-function.
const transitionDuration = {};
for (const [k, v] of Object.entries(tokens.motion.duration)) {
  transitionDuration[k] = v;
}
const transitionTimingFunction = {};
for (const [k, v] of Object.entries(tokens.motion.curve)) {
  transitionTimingFunction[k] = v;
}

/** @type {import("tailwindcss").Config} */
module.exports = {
  content: [],
  theme: {
    extend: {
      colors,
      fontFamily,
      fontSize,
      spacing,
      borderRadius,
      boxShadow,
      transitionDuration,
      transitionTimingFunction,
    },
  },
  plugins: [
    // Expose tokens as CSS variables on :root for app code that prefers var()
    // access over Tailwind classes.
    function tokensAsCssVars({ addBase }) {
      const vars = {};
      const flat = flatten("", tokens, {});
      for (const [k, v] of Object.entries(flat)) {
        if (typeof v === "string" || typeof v === "number") {
          vars[`--${k}`] = String(v);
        }
      }
      addBase({ ":root": vars });
    },
  ],
};
