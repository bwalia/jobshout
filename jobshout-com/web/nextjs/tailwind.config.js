/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        paper: "#eef2f6",
        ink: "#0c1b2a",
        mute: "#5a6b7d",
        line: "#c5d0dc",
        signal: "#00a8b8",
        shout: "#ff4d1a",
        mist: "#eef2f6",
        slate: "#5a6b7d",
        accent: "#00a8b8",
      },
      fontFamily: {
        display: ["var(--font-display)", "Georgia", "serif"],
        sans: ["var(--font-sans)", "system-ui", "sans-serif"],
      },
      maxWidth: {
        board: "68rem",
      },
      keyframes: {
        "signal-pulse": {
          "0%, 100%": { transform: "scaleX(0.35)", opacity: "0.45" },
          "50%": { transform: "scaleX(1)", opacity: "1" },
        },
        "hero-in": {
          from: { opacity: "0", transform: "translateY(12px)" },
          to: { opacity: "1", transform: "translateY(0)" },
        },
      },
      animation: {
        "signal-pulse": "signal-pulse 3.2s ease-in-out infinite",
        "hero-in": "hero-in 0.7s ease-out both",
      },
    },
  },
  plugins: [],
};
