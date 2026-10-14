import type { Metadata } from "next";
import { Poppins, Unbounded, JetBrains_Mono } from "next/font/google";
import "@/styles/globals.css";
import { Providers } from "./providers";

// Type system — the Workstation one, shared with workstation.co.uk:
//   - Poppins        → UI and body text (--font-sans)
//   - Unbounded      → h1/h2 and display text (--font-display); up to 800 for
//                      the big numbers on the dashboard
//   - JetBrains Mono → logs, run output, telemetry (--font-mono)
const poppins = Poppins({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700", "800"],
  variable: "--font-sans",
  display: "swap",
});

const unbounded = Unbounded({
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
      className={`${poppins.variable} ${unbounded.variable} ${jetbrainsMono.variable}`}
    >
      <head>
        {/* Apply the saved text size before first paint, so pages don't
            jump from small to large. Mirrors lib/text-size.ts. */}
        <script
          dangerouslySetInnerHTML={{
            __html: `try{var s=localStorage.getItem("jobshout-text-size");if(s==="large"||s==="largest")document.documentElement.dataset.textSize=s}catch(e){}`,
          }}
        />
      </head>
      <body className="font-sans antialiased">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
