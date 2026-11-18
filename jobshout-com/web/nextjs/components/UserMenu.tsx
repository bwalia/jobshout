"use client";

import Link from "next/link";
import { signOut } from "next-auth/react";
import { useEffect, useRef, useState } from "react";

export type MenuUser = {
  name: string | null;
  email: string | null;
  image: string | null;
};

/** Links shown under the signed-in user, on desktop and in the mobile sheet. */
export const ACCOUNT_LINKS = [
  { href: "/members/me", label: "My profile" },
  { href: "/profile", label: "Matching profile" },
  { href: "/dashboard", label: "Dashboard" },
  { href: "/applications", label: "Applications" },
  { href: "/following", label: "Following" },
] as const;

/** Up to two initials from a name, or the first letter of an email. */
export function initialsFor(user: Pick<MenuUser, "name" | "email">): string {
  const name = user.name?.trim();
  if (name) {
    const parts = name.split(/\s+/).filter(Boolean);
    const first = parts[0]?.[0] ?? "";
    const last = parts.length > 1 ? parts[parts.length - 1][0] : "";
    return (first + last).toUpperCase() || "?";
  }
  return (user.email?.trim()[0] ?? "?").toUpperCase();
}

export function Avatar({ user, size = "md" }: { user: MenuUser; size?: "md" | "lg" }) {
  const [broken, setBroken] = useState(false);
  const box = size === "lg" ? "h-11 w-11 text-sm" : "h-8 w-8 text-xs";
  if (user.image && !broken) {
    return (
      // Provider avatars come from many hosts; next/image would need each one allow-listed.
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={user.image}
        alt=""
        referrerPolicy="no-referrer"
        onError={() => setBroken(true)}
        className={`${box} shrink-0 rounded-full border border-line object-cover`}
      />
    );
  }
  return (
    <span
      aria-hidden
      className={`${box} flex shrink-0 items-center justify-center rounded-full bg-shout font-semibold text-white`}
    >
      {initialsFor(user)}
    </span>
  );
}

function SignOutItem({ className, inMenu = false }: { className: string; inMenu?: boolean }) {
  const [busy, setBusy] = useState(false);
  return (
    <button
      type="button"
      role={inMenu ? "menuitem" : undefined}
      disabled={busy}
      className={className}
      onClick={() => {
        setBusy(true);
        void signOut({ callbackUrl: "/" });
      }}
    >
      {busy ? "Signing out…" : "Sign out"}
    </button>
  );
}

const itemClass =
  "flex w-full cursor-pointer items-center rounded-lg px-3 py-2 text-left text-sm text-body transition-colors duration-150 hover:bg-raised hover:text-ink disabled:cursor-wait disabled:opacity-60";

/** Avatar button in the header that opens the account menu (desktop). */
export function UserMenu({ user }: { user: MenuUser }) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (root.current && !root.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const label = user.name || user.email || "Account";

  return (
    <div ref={root} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={`Account menu for ${label}`}
        className="flex cursor-pointer items-center gap-2 rounded-pill border border-line py-1 pl-1 pr-1 text-sm text-body transition-colors duration-200 hover:border-edge hover:text-ink 2xl:pr-3.5"
      >
        <Avatar user={user} />
        <span className="hidden max-w-[10rem] truncate 2xl:inline">{label}</span>
      </button>

      {open ? (
        <div
          role="menu"
          className="absolute right-0 top-full z-50 mt-2 w-72 animate-rise-in rounded-2xl border border-line bg-surface p-2 shadow-pop"
        >
          <div className="flex items-center gap-3 px-3 py-2.5">
            <Avatar user={user} size="lg" />
            <div className="min-w-0">
              {user.name ? (
                <p className="truncate text-sm font-semibold text-ink">{user.name}</p>
              ) : null}
              {user.email ? <p className="truncate text-xs text-mute">{user.email}</p> : null}
            </div>
          </div>
          <div className="my-1 border-t border-line" />
          {ACCOUNT_LINKS.map((l) => (
            <Link
              key={l.href}
              href={l.href}
              role="menuitem"
              className={itemClass}
              onClick={() => setOpen(false)}
            >
              {l.label}
            </Link>
          ))}
          <div className="my-1 border-t border-line" />
          <SignOutItem className={itemClass} inMenu />
        </div>
      ) : null}
    </div>
  );
}

/** The same account block, flattened for the mobile menu sheet. */
export function MobileAccountSection({ user }: { user: MenuUser }) {
  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-3 px-3 py-2">
        <Avatar user={user} size="lg" />
        <div className="min-w-0">
          {user.name ? <p className="truncate text-sm font-semibold text-ink">{user.name}</p> : null}
          {user.email ? <p className="truncate text-xs text-mute">{user.email}</p> : null}
        </div>
      </div>
      {ACCOUNT_LINKS.map((l) => (
        <Link key={l.href} href={l.href} className={itemClass}>
          {l.label}
        </Link>
      ))}
      <SignOutItem className={itemClass} />
    </div>
  );
}
