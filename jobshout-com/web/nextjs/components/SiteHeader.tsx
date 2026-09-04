import Link from "next/link";
import { getServerSession } from "next-auth";
import { authOptions } from "@/lib/auth";

export async function SiteHeader() {
  const session = await getServerSession(authOptions());

  return (
    <header className="sticky top-0 z-40 border-b border-line/60 bg-paper/85 backdrop-blur-md">
      <div className="mx-auto flex max-w-board items-center justify-between gap-4 px-6 py-4">
        <Link href="/" className="group relative font-display text-[1.35rem] leading-none tracking-tight text-ink">
          JobShout
          <span className="text-signal">.com</span>
          <span
            aria-hidden
            className="absolute -bottom-1 left-0 h-[2px] w-full origin-left bg-signal animate-signal-pulse group-hover:animate-none group-hover:opacity-100"
          />
        </Link>
        <nav className="flex items-center gap-1 text-sm text-mute sm:gap-5">
          <Link href="/jobs" className="hidden px-2 py-1 hover:text-ink sm:inline">
            Open roles
          </Link>
          <Link href="/profile" className="hidden px-2 py-1 hover:text-ink sm:inline">
            Profile
          </Link>
          {session?.user ? (
            <span className="hidden max-w-[9rem] truncate text-ink md:inline">
              {session.user.name || session.user.email}
            </span>
          ) : null}
          <Link
            href={session ? "/api/auth/signout" : "/login"}
            className="ml-1 border border-ink/15 bg-white/70 px-3 py-1.5 text-ink transition hover:border-signal hover:text-signal"
          >
            {session ? "Sign out" : "Sign in"}
          </Link>
        </nav>
      </div>
    </header>
  );
}
