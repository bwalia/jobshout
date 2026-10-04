"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ChevronRight } from "lucide-react";
import { activeNavItem } from "@/lib/nav";
import { useBreadcrumbStore, type Crumb } from "@/lib/store/breadcrumb-store";

/**
 * Where you are: the nav item that owns this page, then whatever the page adds
 * (an agent's name, a project, a tab). The last crumb is the current page.
 */
export function Breadcrumbs() {
  const pathname = usePathname();
  const trail = useBreadcrumbStore((s) => s.trail);
  const item = activeNavItem(pathname);

  const root: Crumb | null = pathname.startsWith("/chat")
    ? { label: "Chat", href: "/chat" }
    : item
      ? { label: item.label, href: item.href }
      : null;
  const crumbs = [...(root ? [root] : []), ...trail];
  if (crumbs.length === 0) return null;

  return (
    <nav aria-label="Breadcrumb" className="min-w-0 flex-1">
      <ol className="flex min-w-0 items-center gap-1 text-sm">
        {crumbs.map((c, i) => {
          const last = i === crumbs.length - 1;
          return (
            <li key={`${c.label}-${i}`} className="flex min-w-0 items-center gap-1">
              {i > 0 && <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-hidden />}
              {last || !c.href ? (
                <span
                  aria-current={last ? "page" : undefined}
                  className={last ? "truncate font-semibold text-foreground" : "truncate text-muted-foreground"}
                >
                  {c.label}
                </span>
              ) : (
                <Link
                  href={c.href}
                  className="truncate rounded text-muted-foreground hover:text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  {c.label}
                </Link>
              )}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
