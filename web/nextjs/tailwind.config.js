/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: ["class"],
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "./lib/**/*.{ts,tsx}",
  ],
  theme: {
    extend: {
      fontFamily: {
        // JIRA / Confluence-style: Inter for body, Inter Tight for display
        // (headings, big numerics). Falls back to system stack if Google
        // Fonts is unreachable.
        sans: [
          "var(--font-sans)",
          "-apple-system",
          "BlinkMacSystemFont",
          "Segoe UI",
          "Roboto",
          "Helvetica",
          "Arial",
          "sans-serif",
        ],
        display: [
          "var(--font-display)",
          "var(--font-sans)",
          "-apple-system",
          "BlinkMacSystemFont",
          "Segoe UI",
          "Roboto",
          "sans-serif",
        ],
        // Telemetry voice — agent ids, timestamps, metrics.
        mono: [
          "var(--font-mono)",
          "ui-monospace",
          "SFMono-Regular",
          "Menlo",
          "Monaco",
          "Consolas",
          "monospace",
        ],
      },
      // JIRA's tighter type scale: smaller, denser, more lines per screen.
      fontSize: {
        "2xs": ["10px", { lineHeight: "14px", letterSpacing: "0.02em" }],
        xs: ["11px", { lineHeight: "16px", letterSpacing: "0.005em" }],
        sm: ["13px", { lineHeight: "20px" }],
        base: ["14px", { lineHeight: "22px" }],
        lg: ["16px", { lineHeight: "24px" }],
        xl: ["18px", { lineHeight: "28px", letterSpacing: "-0.01em" }],
        "2xl": ["22px", { lineHeight: "30px", letterSpacing: "-0.015em" }],
        "3xl": ["28px", { lineHeight: "36px", letterSpacing: "-0.02em" }],
      },
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
        sidebar: {
          DEFAULT: "hsl(var(--sidebar))",
          foreground: "hsl(var(--sidebar-foreground))",
          accent: "hsl(var(--sidebar-accent))",
          muted: "hsl(var(--sidebar-muted))",
          border: "hsl(var(--sidebar-border))",
        },
        // JIRA-aligned status palette — used by the agent board and any other
        // ticket-style surface to keep state colours consistent.
        status: {
          todo:     "hsl(var(--status-todo))",
          progress: "hsl(var(--status-progress))",
          review:   "hsl(var(--status-review))",
          done:     "hsl(var(--status-done))",
          blocked:  "hsl(var(--status-blocked))",
          idle:     "hsl(var(--status-idle))",
        },
        // Signal Room accent + live-status hues (the broadcast language).
        signal: {
          DEFAULT: "hsl(var(--signal))",
          live:    "hsl(var(--signal-live))",
          warn:    "hsl(var(--signal-warn))",
          error:   "hsl(var(--signal-error))",
          info:    "hsl(var(--signal-info))",
        },
      },
      borderRadius: {
        xl: "var(--radius)",
        lg: "calc(var(--radius) - 2px)",
        md: "calc(var(--radius) - 4px)",
        sm: "calc(var(--radius) - 6px)",
      },
      boxShadow: {
        "card": "0 1px 3px 0 rgba(0, 0, 0, 0.04), 0 1px 2px -1px rgba(0, 0, 0, 0.04)",
        "card-hover": "0 4px 6px -1px rgba(0, 0, 0, 0.07), 0 2px 4px -2px rgba(0, 0, 0, 0.05)",
        // Amber signal glow for the primary CTA / focused control.
        "signal": "0 0 0 1px hsl(var(--signal) / 0.35), 0 4px 20px -4px hsl(var(--signal) / 0.4)",
        // Soft green glow for a live/broadcasting element.
        "glow-live": "0 0 16px -2px hsl(var(--signal-live) / 0.55)",
      },
    },
  },
  plugins: [require("tailwindcss-animate"), require("@tailwindcss/typography")],
};
