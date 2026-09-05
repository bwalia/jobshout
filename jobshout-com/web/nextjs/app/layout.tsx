import type { Metadata } from "next";
import { Fraunces, Outfit } from "next/font/google";
import { AuthProvider } from "@/components/AuthProvider";
import { SiteHeader } from "@/components/SiteHeader";
import "./globals.css";

const display = Fraunces({
  subsets: ["latin"],
  variable: "--font-display",
  display: "swap",
  axes: ["opsz"],
});

const sans = Outfit({
  subsets: ["latin"],
  variable: "--font-sans",
  display: "swap",
});

export const metadata: Metadata = {
  title: "JobShout.com — AI-native job marketplace",
  description:
    "The AI-native employment marketplace where Career Agents find roles, Hiring Agents find people, and humans approve every move.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${display.variable} ${sans.variable}`}>
      <body>
        <AuthProvider>
          <SiteHeader />
          <main>{children}</main>
          <footer className="mt-28 border-t border-line/70">
            <div className="mx-auto flex max-w-board flex-col gap-2 px-6 py-10 text-sm text-mute sm:flex-row sm:items-center sm:justify-between">
              <p className="font-display text-base text-ink">JobShout.com</p>
              <p>Agents propose. Humans decide.</p>
            </div>
          </footer>
        </AuthProvider>
      </body>
    </html>
  );
}
