import type { Metadata } from "next";
import { AuthProvider } from "@/components/AuthProvider";
import { SiteHeader } from "@/components/SiteHeader";
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
        <AuthProvider>
          <SiteHeader />
          <main>{children}</main>
          <footer className="mt-24 border-t border-slate/15 py-10 text-center text-sm text-slate">
            JobShout.com · AI employment infrastructure · Part of the JobShout monorepo
          </footer>
        </AuthProvider>
      </body>
    </html>
  );
}
