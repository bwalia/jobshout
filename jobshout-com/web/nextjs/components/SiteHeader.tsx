import Link from "next/link";
import { SignOutButton } from "@/components/SignOutButton";
import { getServerSession } from "next-auth";
import { Logo } from "@/components/Logo";
import { MobileMenu } from "@/components/MobileMenu";
import { NavLinks } from "@/components/NavLinks";
import { ThemeToggle } from "@/components/ThemeToggle";
import { buttonClass } from "@/components/ui";
import { BellIcon, UserCheckIcon, UserIcon } from "@/components/icons";
import { unreadNotifications } from "@/lib/notifications";
import { authOptions } from "@/lib/auth";

export async function SiteHeader() {
  const session = await getServerSession(authOptions());
  const who = session?.user?.name || session?.user?.email || null;

  const authControl = session ? (
    <SignOutButton className={buttonClass("secondary", "sm", "w-full xl:w-auto")} />
  ) : (
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

  const following = session ? (
    <Link href="/following" className={buttonClass("ghost", "sm", "w-full xl:w-auto")}>
      <UserCheckIcon className="h-4 w-4" />
      Following
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
          {who ? (
            <span className="hidden items-center gap-2 rounded-pill border border-line py-1.5 pl-2 pr-3.5 text-sm text-body 2xl:flex">
              <UserIcon className="h-4 w-4 text-mute" />
              <span className="max-w-[10rem] truncate">{who}</span>
            </span>
          ) : null}
          <ThemeToggle />
          {bell}
          {following ? <div className="hidden xl:block">{following}</div> : null}
          <div className="hidden xl:block">{authControl}</div>
          <Link href="/post-job" className={buttonClass("primary", "sm", "hidden sm:inline-flex")}>
            Post a job
          </Link>
          <MobileMenu>
            {following}
            {authControl}
          </MobileMenu>
        </div>
      </div>
    </header>
  );
}
