"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  Bot,
  FolderKanban,
  Network,
  Store,
  BarChart3,
  Settings,
  Cpu,
  Clock,
  ListTree,
  MessageSquare,
} from "lucide-react";
import { cn } from "@/lib/utils/cn";

interface NavItem {
  label: string;
  href: string;
  icon: React.ElementType;
}

const NAV_ITEMS: NavItem[] = [
  { label: "Dashboard", href: "/dashboard", icon: LayoutDashboard },
  { label: "Agents", href: "/agents", icon: Bot },
  { label: "Projects", href: "/projects", icon: FolderKanban },
  { label: "Task Manager", href: "/task-manager", icon: ListTree },
  { label: "Scheduler", href: "/scheduler", icon: Clock },
  { label: "Sessions", href: "/sessions", icon: MessageSquare },
  { label: "LLM Providers", href: "/llm-providers", icon: Cpu },
  { label: "Org Builder", href: "/org-builder", icon: Network },
  { label: "Marketplace", href: "/marketplace", icon: Store },
  { label: "Metrics", href: "/metrics", icon: BarChart3 },
  { label: "Settings", href: "/settings", icon: Settings },
];

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="flex h-full w-60 flex-shrink-0 flex-col border-r border-border bg-card">
      {/* Brand */}
      <div className="flex h-14 items-center px-5 border-b border-border">
        <span className="text-lg font-semibold tracking-tight text-foreground">
          Jobshout
        </span>
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto px-3 py-4">
        <ul className="space-y-1">
          {NAV_ITEMS.map(({ label, href, icon: Icon }) => {
            // Mark active when the pathname starts with the nav href so that
            // nested routes (e.g. /agents/123) also keep the parent item active.
            const isActive =
              pathname === href || pathname.startsWith(`${href}/`);

            return (
              <li key={href}>
                <Link
                  href={href}
                  className={cn(
                    "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                    isActive
                      ? "bg-accent text-accent-foreground"
                      : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                  )}
                >
                  <Icon className="h-4 w-4 flex-shrink-0" />
                  {label}
                </Link>
              </li>
            );
          })}
        </ul>
      </nav>

      {/* Footer – version tag */}
      <div className="px-5 py-3 border-t border-border">
        <p className="text-xs text-muted-foreground">v0.1.0</p>
      </div>
    </aside>
  );
}
