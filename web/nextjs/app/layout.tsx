import type { Metadata } from "next";
import { Inter, Inter_Tight } from "next/font/google";
import "@/styles/globals.css";
import { Providers } from "./providers";

// Body type — Inter is the closest free face to Atlassian's Charlie Sans.
// We pin a CSS variable so Tailwind / globals.css can compose it into the
// font stack instead of locking it into a single className.
const inter = Inter({
  subsets: ["latin"],
  variable: "--font-sans",
  display: "swap",
});

// Display type for headings + numerics — gives the dashboard the same
// confident heading weight as JIRA / Confluence without buying Charlie Sans.
const interTight = Inter_Tight({
  subsets: ["latin"],
  variable: "--font-display",
  display: "swap",
});

export const metadata: Metadata = {
  title: "Jobshout - AI Team Command Center",
  description:
    "Mission control for AI teams. Create agents, build teams, assign projects, track work, and automate workflows.",
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
      className={`${inter.variable} ${interTight.variable}`}
    >
      <body className="font-sans antialiased">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
