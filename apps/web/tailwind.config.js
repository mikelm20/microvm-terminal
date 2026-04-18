/** @type {import('tailwindcss').Config} */
module.exports = {
  presets: [require("@learn/shared-tokens/tailwind-preset")],
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "./lib/**/*.{ts,tsx}",
  ],
};
