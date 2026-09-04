import type { Metadata } from "next";
import { Inter, Sora, JetBrains_Mono } from "next/font/google";
import "@/styles/globals.css";
import { Providers } from "./providers";

// Clean chat-first type system:
//   - Inter (variable) → UI and body text (--font-sans). Sora is a display
//     face: at reading sizes its low x-height and wide letterforms made long
//     agent replies tiring, so body text is set in Inter instead.
//   - Sora (variable)  → headings, where the character earns its keep
//     (--font-display)
//   - JetBrains Mono   → logs, run output, telemetry (--font-mono)
const inter = Inter({
  subsets: ["latin"],
  variable: "--font-sans",
  display: "swap",
});

const sora = Sora({
  subsets: ["latin"],
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
      className={`${inter.variable} ${sora.variable} ${jetbrainsMono.variable}`}
    >
      <body className="font-sans antialiased">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
