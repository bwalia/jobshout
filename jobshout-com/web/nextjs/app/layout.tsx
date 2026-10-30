import type { Metadata, Viewport } from "next";
import localFont from "next/font/local";
import { AuthProvider } from "@/components/AuthProvider";
import { BetaBanner } from "@/components/BetaBanner";
import { JobSupportDock } from "@/components/job-support/JobSupportDock";
import { JobSupportProvider } from "@/components/job-support/JobSupportProvider";
import { SiteFooter } from "@/components/SiteFooter";
import { SiteHeader } from "@/components/SiteHeader";
import "./globals.css";

// Vendored latin cuts — Docker/CI builds must not fetch fonts.googleapis.com
// (that fetch flakes and fails the image publish). Same Workstation stack as
// workstation.co.uk: Poppins body/UI, Unbounded display, JetBrains Mono code.
const display = localFont({
  src: [
    { path: "../fonts/unbounded-latin-500-normal.woff2", weight: "500", style: "normal" },
    { path: "../fonts/unbounded-latin-600-normal.woff2", weight: "600", style: "normal" },
    { path: "../fonts/unbounded-latin-700-normal.woff2", weight: "700", style: "normal" },
    { path: "../fonts/unbounded-latin-800-normal.woff2", weight: "800", style: "normal" },
  ],
  variable: "--font-display",
  display: "swap",
});

const sans = localFont({
  src: [
    { path: "../fonts/poppins-latin-400-normal.woff2", weight: "400", style: "normal" },
    { path: "../fonts/poppins-latin-500-normal.woff2", weight: "500", style: "normal" },
    { path: "../fonts/poppins-latin-600-normal.woff2", weight: "600", style: "normal" },
    { path: "../fonts/poppins-latin-700-normal.woff2", weight: "700", style: "normal" },
    { path: "../fonts/poppins-latin-800-normal.woff2", weight: "800", style: "normal" },
  ],
  variable: "--font-sans",
  display: "swap",
});

const mono = localFont({
  src: [
    { path: "../fonts/jetbrains-mono-latin-400-normal.woff2", weight: "400", style: "normal" },
    { path: "../fonts/jetbrains-mono-latin-500-normal.woff2", weight: "500", style: "normal" },
  ],
  variable: "--font-mono",
  display: "swap",
  preload: false,
});

export const metadata: Metadata = {
  title: {
    default: "JobShout — find work, post work",
    template: "%s · JobShout",
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
          <JobSupportProvider>
            <div className="flex min-h-screen flex-col">
              <BetaBanner />
              <SiteHeader />
              <main id="main" className="flex-1">
                {children}
              </main>
              <SiteFooter />
            </div>
            <JobSupportDock />
          </JobSupportProvider>
        </AuthProvider>
      </body>
    </html>
  );
}
