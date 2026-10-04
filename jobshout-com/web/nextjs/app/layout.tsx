import type { Metadata, Viewport } from "next";
import { JetBrains_Mono, Poppins, Unbounded } from "next/font/google";
import { AuthProvider } from "@/components/AuthProvider";
import { BetaBanner } from "@/components/BetaBanner";
import { SiteFooter } from "@/components/SiteFooter";
import { SiteHeader } from "@/components/SiteHeader";
import "./globals.css";

// The Workstation type system, shared with workstation.co.uk: Poppins for body
// and UI, Unbounded for h1/h2 and display text, JetBrains Mono for code.
const display = Unbounded({
  subsets: ["latin"],
  weight: ["500", "600", "700", "800"],
  variable: "--font-display",
  display: "swap",
});

const sans = Poppins({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700", "800"],
  variable: "--font-sans",
  display: "swap",
});

const mono = JetBrains_Mono({
  subsets: ["latin"],
  weight: ["400", "500"],
  variable: "--font-mono",
  display: "swap",
  preload: false,
});

export const metadata: Metadata = {
  title: {
    default: "JobShout.com — find work, post work",
    template: "%s · JobShout.com",
  },
  description:
    "The AI-native employment marketplace. Search open roles, apply in a couple of minutes, and post jobs that reach candidates who actually match.",
};

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#FAFAF9" },
    { media: "(prefers-color-scheme: dark)", color: "#0B0C0E" },
  ],
};

/**
 * Resolves the theme before first paint so the page never flashes the wrong
 * one. Kept inline and tiny — it must run ahead of any stylesheet.
 */
const NO_FLASH_THEME = `
(function () {
  try {
    var stored = localStorage.getItem('jobshout-theme');
    var dark = stored ? stored === 'dark'
      : window.matchMedia('(prefers-color-scheme: dark)').matches;
    document.documentElement.classList.toggle('dark', dark);
  } catch (e) {}
})();`;

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${display.variable} ${sans.variable} ${mono.variable}`} suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: NO_FLASH_THEME }} />
      </head>
      <body>
        <a
          href="#main"
          className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:rounded-pill focus:bg-shout focus:px-5 focus:py-3 focus:text-sm focus:font-semibold focus:text-[rgb(var(--on-brand))]"
        >
          Skip to content
        </a>
        <AuthProvider>
          <div className="flex min-h-screen flex-col">
            <BetaBanner />
            <SiteHeader />
            <main id="main" className="flex-1">
              {children}
            </main>
            <SiteFooter />
          </div>
        </AuthProvider>
      </body>
    </html>
  );
}
