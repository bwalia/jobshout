import type { Metadata } from "next";
import { Plus_Jakarta_Sans, Outfit, JetBrains_Mono } from "next/font/google";
import "@/styles/globals.css";
import { Providers } from "./providers";

// Type system:
//   - Plus Jakarta Sans → UI and body text (--font-sans). Chosen over Inter for
//     its taller x-height: at the same pixel size it reads noticeably larger,
//     which is most of what "make the text bigger" actually needs in a dense
//     dashboard.
//   - Outfit (variable)  → headings (--font-display). Geometric and openly
//     modern, with real weight up to 800 for the big numbers on the dashboard.
//   - JetBrains Mono     → logs, run output, telemetry (--font-mono)
const plusJakarta = Plus_Jakarta_Sans({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700", "800"],
  variable: "--font-sans",
  display: "swap",
});

const outfit = Outfit({
  subsets: ["latin"],
  weight: ["500", "600", "700", "800"],
  variable: "--font-display",
  display: "swap",
});

const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
  display: "swap",
});

export const metadata: Metadata = {
  title: "JobShout",
  description:
    "Chat-first workspace for autonomous AI agents — dispatch, orchestrate, and watch runs.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={`${plusJakarta.variable} ${outfit.variable} ${jetbrainsMono.variable}`}
    >
      <body className="font-sans antialiased">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
