"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

/** Phase 1 nav: Home · Work · Network · Discover (+ legacy shortcuts). */
const LINKS = [
  { href: "/dashboard", label: "Home" },
  { href: "/jobs", label: "Jobs" },
  { href: "/projects", label: "Projects" },
  { href: "/members", label: "Network" },
  { href: "/showcase", label: "Showcase" },
  { href: "/agents", label: "Agents" },
  { href: "/insights", label: "Insights" },
  { href: "/messages", label: "Messages" },
  { href: "/post-job", label: "Post a job" },
];

export function NavLinks({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = usePathname();

  return (
    <>
      {LINKS.map((link) => {
        const active = pathname === link.href || pathname.startsWith(`${link.href}/`);
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
        const active = pathname === link.href || pathname.startsWith(`${link.href}/`);
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
