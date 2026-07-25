import type { Metadata } from "next";
import { Inter, Space_Grotesk, JetBrains_Mono } from "next/font/google";
import "@/styles/globals.css";
import { Providers } from "./providers";

// "Signal Room" type system:
//   - Inter          → UI/body, dense and legible at small sizes (--font-sans)
//   - Space Grotesk  → display, the technical/characterful headline voice (--font-display)
//   - JetBrains Mono → all telemetry: agent ids, timestamps, metrics (--font-mono)
// The mono-for-data pairing is the signature typographic move — the "ops
// console" voice. (Swap JetBrains for Geist Mono if you prefer.)
const inter = Inter({
  subsets: ["latin"],
  variable: "--font-sans",
  display: "swap",
});

const spaceGrotesk = Space_Grotesk({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
  variable: "--font-display",
  display: "swap",
});

const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-mono",
  display: "swap",
});

export const metadata: Metadata = {
  title: "JobShout — Mission Control for AI Agents",
  description:
    "Mission control for autonomous AI agents. Dispatch agents, orchestrate multi-agent work, watch the live fleet, and step in when it matters.",
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
      className={`${inter.variable} ${spaceGrotesk.variable} ${jetbrainsMono.variable}`}
    >
      <body className="font-sans antialiased">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
