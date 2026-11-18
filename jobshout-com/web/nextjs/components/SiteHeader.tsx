import Link from "next/link";
import { getServerSession } from "next-auth";
import { Logo } from "@/components/Logo";
import { MobileMenu } from "@/components/MobileMenu";
import { NavLinks } from "@/components/NavLinks";
import { ThemeToggle } from "@/components/ThemeToggle";
import { MobileAccountSection, UserMenu, type MenuUser } from "@/components/UserMenu";
import { buttonClass } from "@/components/ui";
import { BellIcon } from "@/components/icons";
import { unreadNotifications } from "@/lib/notifications";
import { authOptions } from "@/lib/auth";

export async function SiteHeader() {
  const session = await getServerSession(authOptions());
  const user: MenuUser | null = session?.user
    ? {
        name: session.user.name ?? null,
        email: session.user.email ?? null,
        image: session.user.image ?? null,
      }
    : null;

  const signIn = (
    <Link href="/login" className={buttonClass("secondary", "sm", "w-full xl:w-auto")}>
      Sign in
    </Link>
  );

  const email = session?.user?.email ?? null;
  const unread = email ? await unreadNotifications({ email, name: session?.user?.name ?? null }) : 0;
  const bell = email ? (
    <Link
      href="/notifications"
      className={buttonClass("ghost", "sm", "relative")}
      aria-label={unread > 0 ? `Notifications, ${unread} unread` : "Notifications"}
    >
      <BellIcon className="h-4 w-4" />
      {unread > 0 ? (
        <span className="absolute -right-0.5 -top-0.5 min-w-[1.1rem] rounded-full bg-shout px-1 text-center text-[10px] font-bold leading-[1.1rem] text-white">
          {unread > 99 ? "99+" : unread}
        </span>
      ) : null}
    </Link>
  ) : null;

  return (
    <header className="sticky top-0 z-40 border-b border-line bg-bg/85 backdrop-blur-xl">
      <div className="mx-auto flex h-[4.25rem] max-w-board items-center gap-4 px-5 sm:px-8">
        <Logo />

        <nav aria-label="Main" className="ml-6 hidden items-center gap-1 xl:flex">
          <NavLinks />
        </nav>

        <div className="ml-auto flex items-center gap-2">
          <ThemeToggle />
          {bell}
          <div className="hidden xl:block">{user ? <UserMenu user={user} /> : signIn}</div>
          <Link href="/post-job" className={buttonClass("primary", "sm", "hidden sm:inline-flex")}>
            Post a job
          </Link>
          <MobileMenu>{user ? <MobileAccountSection user={user} /> : signIn}</MobileMenu>
        </div>
      </div>
    </header>
  );
}
