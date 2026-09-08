import Link from "next/link";
import { getServerSession } from "next-auth";
import { Logo } from "@/components/Logo";
import { MobileMenu } from "@/components/MobileMenu";
import { NavLinks } from "@/components/NavLinks";
import { ThemeToggle } from "@/components/ThemeToggle";
import { buttonClass } from "@/components/ui";
import { UserIcon } from "@/components/icons";
import { authOptions } from "@/lib/auth";

export async function SiteHeader() {
  const session = await getServerSession(authOptions());
  const who = session?.user?.name || session?.user?.email || null;

  const authControl = session ? (
    <Link
      href="/api/auth/signout"
      className={buttonClass("secondary", "sm", "w-full lg:w-auto")}
    >
      Sign out
    </Link>
  ) : (
    <Link href="/login" className={buttonClass("secondary", "sm", "w-full lg:w-auto")}>
      Sign in
    </Link>
  );

  return (
    <header className="sticky top-0 z-40 border-b border-line bg-bg/85 backdrop-blur-xl">
      <div className="mx-auto flex h-[4.25rem] max-w-board items-center gap-4 px-5 sm:px-8">
        <Logo />

        <nav aria-label="Main" className="ml-6 hidden items-center gap-1 lg:flex">
          <NavLinks />
        </nav>

        <div className="ml-auto flex items-center gap-2">
          {who ? (
            <span className="hidden items-center gap-2 rounded-pill border border-line py-1.5 pl-2 pr-3.5 text-sm text-body xl:flex">
              <UserIcon className="h-4 w-4 text-mute" />
              <span className="max-w-[10rem] truncate">{who}</span>
            </span>
          ) : null}
          <ThemeToggle />
          <div className="hidden lg:block">{authControl}</div>
          <Link href="/post-job" className={buttonClass("primary", "sm", "hidden sm:inline-flex")}>
            Post a job
          </Link>
          <MobileMenu>{authControl}</MobileMenu>
        </div>
      </div>
    </header>
  );
}
