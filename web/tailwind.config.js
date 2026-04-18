/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        learn: {
          // legacy app palette (used by the original sandbox page)
          bg: "#1a0f0a",
          surface: "#261510",
          cream: "#f2e6d5",
          accent: "#d97455",
          // mocks palette (landing, catalog, lesson player)
          burgundy: {
            top: "#b03a15",
            mid: "#8f2f10",
            bottom: "#6e2608",
          },
          warm: "#e6cba3",       // cream warm body text
          warmHi: "#fff6e6",     // cream highlight (titles)
          ember: "#ffc591",      // light flame
          orange: "#ff9b5a",     // mid flame
          rust: "#e6724a",       // dark flame / accent
          deep: "#2a1208",       // deep brown text on cream cards
          successLight: "#8fe09b",
          successMid: "#6fcf7a",
          successDark: "#58b864",
        },
      },
      fontFamily: {
        sans: [
          "Inter",
          "ui-sans-serif",
          "system-ui",
          "-apple-system",
          "Segoe UI",
          "Roboto",
          "sans-serif",
        ],
        mono: [
          "JetBrains Mono",
          "ui-monospace",
          "SFMono-Regular",
          "Menlo",
          "Monaco",
          "monospace",
        ],
      },
      backgroundImage: {
        "learn-burgundy":
          "linear-gradient(180deg, #b03a15 0%, #8f2f10 60%, #6e2608 100%)",
        "learn-flame":
          "linear-gradient(135deg, #ffc591 0%, #ff9b5a 50%, #e6724a 100%)",
        "learn-success":
          "linear-gradient(180deg, #6fcf7a 0%, #58b864 100%)",
      },
      keyframes: {
        slideUp: {
          from: { transform: "translateY(100%)" },
          to: { transform: "translateY(0)" },
        },
        pulseRing: {
          "0%, 100%": {
            boxShadow:
              "0 0 0 6px rgba(255, 197, 145, 0.25), 0 14px 36px rgba(230, 114, 74, 0.55)",
          },
          "50%": {
            boxShadow:
              "0 0 0 12px rgba(255, 197, 145, 0.12), 0 14px 36px rgba(230, 114, 74, 0.65)",
          },
        },
      },
      animation: {
        slideUp: "slideUp 0.4s ease-out",
        pulseRing: "pulseRing 2.4s ease-in-out infinite",
      },
    },
  },
  plugins: [],
};
