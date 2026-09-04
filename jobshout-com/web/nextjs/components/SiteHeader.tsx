import Link from "next/link";
import { getServerSession } from "next-auth";
import { authOptions } from "@/lib/auth";

export async function SiteHeader() {
  const session = await getServerSession(authOptions());

  return (
    <header className="border-b border-slate/15 bg-white/80 backdrop-blur">
      <div className="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
        <Link href="/" className="font-display text-xl tracking-tight">
          JobShout<span className="text-accent">.com</span>
        </Link>
        <nav className="flex items-center gap-5 text-sm font-medium text-slate">
          <Link href="/jobs" className="hover:text-ink">
            Find a Job
          </Link>
          <Link href="/profile" className="hover:text-ink">
            My profile
          </Link>
          <Link href="/login?callbackUrl=%2Fprofile" className="hover:text-ink">
            Hire Talent
          </Link>
          {session?.user ? (
            <span className="hidden max-w-[10rem] truncate text-ink sm:inline">
              {session.user.name || session.user.email}
            </span>
          ) : null}
          <Link
            href={session ? "/api/auth/signout" : "/login"}
            className="rounded-full bg-ink px-3 py-1.5 text-white hover:bg-slate"
          >
            {session ? "Sign out" : "Sign in"}
          </Link>
        </nav>
      </div>
    </header>
  );
}
