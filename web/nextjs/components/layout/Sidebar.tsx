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
  Workflow,
  Puzzle,
  ChevronLeft,
  Zap,
} from "lucide-react";
import { cn } from "@/lib/utils/cn";

interface NavItem {
  label: string;
  href: string;
  icon: React.ElementType;
}

interface NavSection {
  title: string;
  items: NavItem[];
}

const NAV_SECTIONS: NavSection[] = [
  {
    title: "Overview",
    items: [
      { label: "Dashboard", href: "/dashboard", icon: LayoutDashboard },
    ],
  },
  {
    title: "AI Team",
    items: [
      { label: "Agents", href: "/agents", icon: Bot },
      { label: "Sessions", href: "/sessions", icon: MessageSquare },
      { label: "Scheduler", href: "/scheduler", icon: Clock },
    ],
  },
  {
    title: "Work",
    items: [
      { label: "Projects", href: "/projects", icon: FolderKanban },
      { label: "Task Manager", href: "/task-manager", icon: ListTree },
    ],
  },
  {
    title: "Automation",
    items: [
      { label: "Workflows", href: "/workflows", icon: Workflow },
      { label: "Plugins", href: "/plugins", icon: Puzzle },
    ],
  },
  {
    title: "Platform",
    items: [
      { label: "LLM Providers", href: "/llm-providers", icon: Cpu },
      { label: "Org Builder", href: "/org-builder", icon: Network },
      { label: "Marketplace", href: "/marketplace", icon: Store },
      { label: "Metrics", href: "/metrics", icon: BarChart3 },
      { label: "Settings", href: "/settings", icon: Settings },
    ],
  },
];

interface SidebarProps {
  collapsed: boolean;
  onToggle: () => void;
}

export function Sidebar({ collapsed, onToggle }: SidebarProps) {
  const pathname = usePathname();

  return (
    <aside
      className={cn(
        "fixed left-0 top-0 z-30 flex h-screen flex-col border-r border-sidebar-border bg-sidebar transition-all duration-300",
        collapsed ? "w-[72px]" : "w-64"
      )}
    >
      {/* Brand header */}
      <div className="flex h-16 items-center justify-between border-b border-sidebar-border px-4">
        <div className="flex items-center gap-2.5 overflow-hidden">
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Zap className="h-4 w-4" />
          </span>
          {!collapsed && (
            <span className="text-base font-semibold tracking-tight text-white whitespace-nowrap">
              Jobshout
            </span>
          )}
        </div>
        <button
          onClick={onToggle}
          className={cn(
            "flex h-6 w-6 items-center justify-center rounded-md text-sidebar-foreground transition-all hover:bg-sidebar-muted hover:text-white",
            collapsed && "mx-auto rotate-180"
          )}
          aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
        >
          <ChevronLeft className="h-4 w-4" />
        </button>
      </div>

      {/* Navigation */}
      <nav className="flex-1 overflow-y-auto scrollbar-thin px-3 py-4">
        {NAV_SECTIONS.map((section) => (
          <div key={section.title} className="mb-6">
            {!collapsed && (
              <p className="mb-2 px-3 text-[11px] font-semibold uppercase tracking-wider text-sidebar-foreground/50">
                {section.title}
              </p>
            )}
            <ul className="space-y-0.5">
              {section.items.map(({ label, href, icon: Icon }) => {
                const isActive =
                  pathname === href || pathname.startsWith(`${href}/`);

                return (
                  <li key={href}>
                    <Link
                      href={href}
                      title={collapsed ? label : undefined}
                      className={cn(
                        "flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm font-medium transition-colors",
                        isActive
                          ? "border-l-2 border-primary bg-primary/10 text-white"
                          : "border-l-2 border-transparent text-sidebar-foreground hover:bg-sidebar-muted hover:text-white",
                        collapsed && "justify-center px-0"
                      )}
                    >
                      <Icon className="h-4 w-4 shrink-0" />
                      {!collapsed && <span>{label}</span>}
                    </Link>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </nav>

      {/* Footer */}
      <div className="border-t border-sidebar-border px-4 py-3">
        {!collapsed ? (
          <p className="text-[11px] text-sidebar-foreground/40">
            Jobshout v0.3.0
          </p>
        ) : (
          <p className="text-center text-[10px] text-sidebar-foreground/40">
            v0.3
          </p>
        )}
      </div>
    </aside>
  );
}
