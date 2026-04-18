/** @type {import('tailwindcss').Config} */
// NOTE: this should become `presets: [require('@learn/shared-tokens/tailwind-preset')]`
// once Agent-Tooling lands shared/tokens/tailwind-preset.js (issue #16).
module.exports = {
  presets: [require("./tailwind/tokens-preset.js")],
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "./lib/**/*.{ts,tsx}",
  ],
};
