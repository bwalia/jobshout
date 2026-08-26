import type { Metadata } from "next";
import { Sora, JetBrains_Mono } from "next/font/google";
import "@/styles/globals.css";
import { Providers } from "./providers";

// Clean chat-first type system:
//   - Sora (variable) → UI/body + display (--font-sans / --font-display)
//   - JetBrains Mono  → logs, run output, telemetry (--font-mono)
const sora = Sora({
  subsets: ["latin"],
  variable: "--font-sans",
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
      className={`${sora.variable} ${jetbrainsMono.variable}`}
      style={
        {
          ["--font-display" as string]: "var(--font-sans)",
        } as React.CSSProperties
      }
    >
      <body className="font-sans antialiased">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
