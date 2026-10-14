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
        sans: [
          "var(--font-sans)",
          "Helvetica Neue",
          "Arial",
          "sans-serif",
        ],
        display: [
          "var(--font-display)",
          "var(--font-sans)",
          "-apple-system",
          "BlinkMacSystemFont",
          "sans-serif",
        ],
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
      // Every size moved up a step. `base` was 14px, so anything written as
      // text-base — which is most of the app — rendered below the 16px that
      // browsers and iOS treat as normal body text. That single number was
      // why the dashboard and chat read small no matter what the components
      // asked for; fixing it here fixes every page at once.
      // rem, not px, so the "Text size" setting (html font-size) scales
      // every label. 2xs was 11px; 12px is the smallest we let text go.
      fontSize: {
        "2xs": ["0.75rem", { lineHeight: "1rem", letterSpacing: "0.02em" }],
        xs: ["0.8125rem", { lineHeight: "1.125rem" }],
        sm: ["0.875rem", { lineHeight: "1.3125rem" }],
        base: ["1rem", { lineHeight: "1.5rem" }],
        lg: ["1.125rem", { lineHeight: "1.6875rem" }],
        xl: ["1.25rem", { lineHeight: "1.8125rem", letterSpacing: "-0.01em" }],
        "2xl": ["1.5625rem", { lineHeight: "2.0625rem", letterSpacing: "-0.015em" }],
        "3xl": ["2rem", { lineHeight: "2.5rem", letterSpacing: "-0.02em" }],
        "4xl": ["2.5rem", { lineHeight: "2.9375rem", letterSpacing: "-0.022em" }],
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
        card: "0 1px 2px 0 rgba(0, 0, 0, 0.04)",
        "card-hover": "0 2px 8px -2px rgba(0, 0, 0, 0.08)",
        // Kept as no-ops so existing class names don't break; glows removed.
        signal: "none",
        "glow-live": "none",
      },
    },
  },
  plugins: [require("tailwindcss-animate"), require("@tailwindcss/typography")],
};
