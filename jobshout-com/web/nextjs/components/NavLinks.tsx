"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

/**
 * LinkedIn-simple primary nav. Hire / Projects / Credits / Teams stay reachable
 * from account surfaces and deep links — not in the first chrome.
 */
const LINKS = [
  { href: "/", label: "Home", match: (p: string) => p === "/" },
  { href: "/jobs", label: "Jobs", match: (p: string) => p === "/jobs" || p.startsWith("/jobs/") },
  {
    href: "/members",
    label: "Talents",
    match: (p: string) =>
      p === "/members" || p.startsWith("/members/") || p.startsWith("/following"),
  },
  { href: "/groups", label: "Groups", match: (p: string) => p === "/groups" || p.startsWith("/groups/") },
  {
    href: "/agents",
    label: "Agents",
    match: (p: string) =>
      p === "/agents" ||
      p.startsWith("/agents/") ||
      p === "/showcase" ||
      p.startsWith("/showcase/"),
  },
  {
    href: "/insights",
    label: "Insights",
    match: (p: string) => p === "/insights" || p.startsWith("/insights/"),
  },
  {
    href: "/messages",
    label: "Messages",
    match: (p: string) => p === "/messages" || p.startsWith("/messages/"),
  },
];

export function NavLinks({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = usePathname();

  return (
    <>
      {LINKS.map((link) => {
        const active = link.match(pathname);
        return (
          <Link
            key={link.href}
            href={link.href}
            onClick={onNavigate}
            aria-current={active ? "page" : undefined}
            className={`relative whitespace-nowrap rounded-pill px-3 py-2 text-sm transition-colors duration-200 ${
              active ? "font-semibold text-ink" : "font-medium text-mute hover:text-ink"
            }`}
          >
            {link.label}
            {active ? (
              <span
                aria-hidden
                className="absolute inset-x-3 -bottom-0.5 h-[2px] rounded-full bg-shout"
              />
            ) : null}
          </Link>
        );
      })}
    </>
  );
}

export function MobileNavLinks({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = usePathname();

  return (
    <>
      {LINKS.map((link) => {
        const active = link.match(pathname);
        return (
          <Link
            key={link.href}
            href={link.href}
            onClick={onNavigate}
            aria-current={active ? "page" : undefined}
            className={`flex min-h-[44px] items-center rounded-xl px-3 text-base transition-colors duration-200 ${
              active ? "bg-raised font-semibold text-ink" : "font-medium text-body hover:bg-raised"
            }`}
          >
            {link.label}
          </Link>
        );
      })}
    </>
  );
}
