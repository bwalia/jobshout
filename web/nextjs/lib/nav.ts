import type { LucideIcon } from "lucide-react";
import {
  Archive,
  Bot,
  Clock,
  Cpu,
  FolderKanban,
  Goal,
  History,
  Home,
  Kanban,
  Network,
  Puzzle,
  Settings,
  Store,
  Workflow,
} from "lucide-react";

/**
 * The sidebar, as data. Adding a page is one line here: the sidebar, the
 * breadcrumbs and the command palette all read this config.
 */

/** A failure count the sidebar shows as a red badge on an item. */
export type NavBadge = "failed-tasks" | "failed-agents";

export interface NavItem {
  id: string;
  label: string;
  href: string;
  icon: LucideIcon;
  /**
   * Path prefixes (no query) that make this item active, so nested routes
   * such as /panel/agents/123 highlight "All agents". Defaults to [href].
   */
  match?: string[];
  /** Words the command palette also matches on. */
  keywords?: string[];
  badge?: NavBadge;
}

export interface NavSection {
  id: string;
  label: string;
  items: NavItem[];
}

export const NAV_HOME: NavItem = {
  id: "dashboard",
  label: "Home",
  href: "/panel/dashboard",
  icon: Home,
  match: ["/panel/dashboard", "/dashboard", "/metrics"],
  keywords: ["dashboard", "overview"],
};

export const NAV_SECTIONS: NavSection[] = [
  {
    id: "work",
    label: "Work",
    items: [
      {
        id: "projects",
        label: "Projects",
        href: "/panel/projects",
        icon: FolderKanban,
        match: ["/panel/projects", "/projects"],
      },
      {
        id: "tasks",
        label: "Tasks",
        href: "/panel/task-board",
        icon: Kanban,
        match: ["/panel/tasks", "/panel/task-board", "/tasks"],
        keywords: ["board", "kanban", "task board"],
        badge: "failed-tasks",
      },
      {
        id: "sprints",
        label: "Sprints",
        href: "/panel/sprints",
        icon: Goal,
        match: ["/panel/sprints", "/sprints"],
      },
      {
        id: "outputs",
        label: "Outputs",
        href: "/panel/artifacts",
        icon: Archive,
        match: ["/panel/artifacts", "/articles", "/artifacts"],
        keywords: ["artifacts", "articles", "images"],
      },
    ],
  },
  {
    id: "agents",
    label: "Agents",
    items: [
      {
        id: "agents",
        label: "All agents",
        href: "/panel/task-manager",
        icon: Bot,
        match: ["/panel/agents", "/panel/task-manager", "/agents"],
        keywords: ["agents", "task manager", "specialists"],
        badge: "failed-agents",
      },
      {
        id: "org-builder",
        label: "Org chart",
        href: "/panel/org-builder",
        icon: Network,
        match: ["/panel/org-builder", "/org-builder"],
        keywords: ["org builder", "hierarchy"],
      },
      {
        id: "plugins-skills",
        label: "Skills & plugins",
        href: "/panel/plugins-skills",
        icon: Puzzle,
        match: ["/panel/plugins-skills", "/plugins", "/skills"],
      },
      {
        id: "marketplace",
        label: "Marketplace",
        href: "/panel/marketplace",
        icon: Store,
        match: ["/panel/marketplace", "/marketplace"],
      },
    ],
  },
  {
    id: "automation",
    label: "Automation",
    items: [
      {
        id: "workflows",
        label: "Workflows",
        href: "/panel/workflows",
        icon: Workflow,
        match: ["/panel/workflows", "/workflows"],
      },
      {
        id: "scheduler",
        label: "Schedules",
        href: "/panel/scheduler",
        icon: Clock,
        match: ["/panel/scheduler", "/scheduler"],
        keywords: ["scheduler", "cron"],
      },
    ],
  },
];

/** Shown at the foot of the Chats section: LLM context sessions, not chats. */
export const NAV_SESSIONS: NavItem = {
  id: "sessions",
  label: "Model sessions",
  href: "/panel/sessions",
  icon: History,
  match: ["/panel/sessions", "/sessions"],
  keywords: ["sessions", "context", "snapshots"],
};

/** Pinned to the bottom of the sidebar, so they never scroll away. */
export const NAV_BOTTOM: NavItem[] = [
  {
    id: "llm-providers",
    label: "Models & providers",
    href: "/panel/llm-providers",
    icon: Cpu,
    match: ["/panel/llm-providers", "/panel/llm-benchmarks", "/llm-providers"],
    keywords: ["llm", "providers", "ollama", "gemini", "benchmarks"],
  },
  {
    id: "settings",
    label: "Settings",
    href: "/panel/settings",
    icon: Settings,
    match: ["/panel/settings", "/settings"],
  },
];

/** Every item, in sidebar order. */
export const ALL_NAV_ITEMS: NavItem[] = [
  NAV_HOME,
  ...NAV_SECTIONS.flatMap((s) => s.items),
  NAV_SESSIONS,
  ...NAV_BOTTOM,
];

function matches(prefix: string, pathname: string): boolean {
  return pathname === prefix || pathname.startsWith(`${prefix}/`);
}

/** The nav item that owns pathname: the longest matching prefix wins. */
export function activeNavItem(pathname: string): NavItem | null {
  let best: NavItem | null = null;
  let bestLen = -1;
  for (const item of ALL_NAV_ITEMS) {
    for (const prefix of item.match ?? [item.href]) {
      if (matches(prefix, pathname) && prefix.length > bestLen) {
        best = item;
        bestLen = prefix.length;
      }
    }
  }
  return best;
}

/** The section an item belongs to, if any. */
export function sectionOf(itemId: string): NavSection | null {
  return NAV_SECTIONS.find((s) => s.items.some((i) => i.id === itemId)) ?? null;
}

/** Where the sidebar's "New task" button goes. */
export const NEW_TASK_HREF = "/panel/task-board";
