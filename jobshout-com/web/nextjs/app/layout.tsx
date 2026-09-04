import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  title: "JobShout.com — AI-native job marketplace",
  description:
    "The AI-native global employment marketplace where humans and AI agents discover, match, interview and hire.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <header className="border-b border-slate/15 bg-white/80 backdrop-blur">
          <div className="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
            <Link href="/" className="font-display text-xl tracking-tight">
              JobShout<span className="text-accent">.com</span>
            </Link>
            <nav className="flex items-center gap-5 text-sm font-medium text-slate">
              <Link href="/jobs" className="hover:text-ink">
                Find a Job
              </Link>
              <a href="#hire" className="hover:text-ink">
                Hire Talent
              </a>
              <a
                href="https://github.com/bwalia/jobshout"
                className="rounded-full bg-ink px-3 py-1.5 text-white hover:bg-slate"
              >
                Build with JobShout
              </a>
            </nav>
          </div>
        </header>
        <main>{children}</main>
        <footer className="mt-24 border-t border-slate/15 py-10 text-center text-sm text-slate">
          JobShout.com · AI employment infrastructure · Part of the JobShout monorepo
        </footer>
      </body>
    </html>
  );
}
