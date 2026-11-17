"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

/**
 * Phone bottom bar — Groups and Insights sit in the primary five so they are
 * as reachable as Jobs. Talents / Agents stay in the header sheet + desktop nav.
 */
const TABS = [
  { href: "/", label: "Home", match: (p: string) => p === "/" },
  { href: "/jobs", label: "Jobs", match: (p: string) => p.startsWith("/jobs") },
  {
    href: "/groups",
    label: "Groups",
    match: (p: string) => p.startsWith("/groups"),
  },
  {
    href: "/insights",
    label: "Insights",
    match: (p: string) => p.startsWith("/insights"),
  },
  {
    href: "/messages",
    label: "Messages",
    match: (p: string) => p.startsWith("/messages"),
  },
];

/** Phone bottom bar — mirrors the simplified primary destinations. Hidden from md up. */
export function MobileBottomNav() {
  const pathname = usePathname();
  return (
    <nav
      aria-label="Primary"
      className="fixed inset-x-0 bottom-0 z-40 border-t border-line bg-canvas/95 backdrop-blur md:hidden"
    >
      <ul className="mx-auto flex max-w-lg items-stretch justify-between px-2 pb-[env(safe-area-inset-bottom)]">
        {TABS.map((tab) => {
          const active = tab.match(pathname);
          return (
            <li key={tab.href} className="flex-1">
              <Link
                href={tab.href}
                aria-current={active ? "page" : undefined}
                className={`flex min-h-[52px] flex-col items-center justify-center px-1 text-[11px] font-medium ${
                  active ? "text-shout" : "text-mute"
                }`}
              >
                {tab.label}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
