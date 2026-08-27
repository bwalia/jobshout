import type { LucideIcon } from "lucide-react";
import {
  LayoutDashboard,
  MessageSquare,
  Kanban,
  ListTree,
  Clock,
  Goal,
  Workflow,
  Network,
  Store,
  Puzzle,
  Cpu,
  Settings,
  History,
} from "lucide-react";

export type PanelId =
  | "chat"
  | "dashboard"
  | "task-board"
  | "task-manager"
  | "scheduler"
  | "sprints"
  | "sessions"
  | "workflows"
  | "org-builder"
  | "marketplace"
  | "plugins-skills"
  | "llm-providers"
  | "settings";

export interface PanelDef {
  id: PanelId;
  label: string;
  href: string;
  icon: LucideIcon;
}

/** Menu order — Chat first, then Dashboard, then the rest. */
export const PANELS: PanelDef[] = [
  { id: "chat", label: "Chat", href: "/chat", icon: MessageSquare },
  { id: "dashboard", label: "Dashboard", href: "/panel/dashboard", icon: LayoutDashboard },
  { id: "task-board", label: "Task Board", href: "/panel/task-board", icon: Kanban },
  { id: "task-manager", label: "Task Manager", href: "/panel/task-manager", icon: ListTree },
  { id: "scheduler", label: "Scheduler", href: "/panel/scheduler", icon: Clock },
  { id: "sprints", label: "Sprints", href: "/panel/sprints", icon: Goal },
  { id: "sessions", label: "Sessions", href: "/panel/sessions", icon: History },
  { id: "workflows", label: "Workflows", href: "/panel/workflows", icon: Workflow },
  { id: "org-builder", label: "Org Builder", href: "/panel/org-builder", icon: Network },
  { id: "marketplace", label: "Marketplace", href: "/panel/marketplace", icon: Store },
  { id: "plugins-skills", label: "Plugins & Skills", href: "/panel/plugins-skills", icon: Puzzle },
  { id: "llm-providers", label: "LLM Providers", href: "/panel/llm-providers", icon: Cpu },
  { id: "settings", label: "Settings", href: "/panel/settings", icon: Settings },
];

export function panelIndex(id: PanelId | null | undefined): number {
  if (!id) return -1;
  return PANELS.findIndex((p) => p.id === id);
}

export function panelFromPath(pathname: string): PanelId {
  if (pathname === "/chat" || pathname.startsWith("/chat/")) return "chat";
  if (pathname.startsWith("/panel/")) {
    const slug = pathname.split("/")[2] as PanelId | undefined;
    if (slug && PANELS.some((p) => p.id === slug)) return slug;
  }
  return "chat";
}

/** Old route → new location (for redirects + migration checklist). */
export const ROUTE_MIGRATION: { from: string; to: string; note: string }[] = [
  { from: "/", to: "/chat", note: "Chat is home" },
  { from: "/dashboard", to: "/panel/dashboard", note: "Merged into Dashboard panel" },
  { from: "/metrics", to: "/panel/dashboard", note: "Merged into Dashboard panel" },
  { from: "/agent-board", to: "/panel/task-board", note: "Agents view inside Task Board" },
  { from: "/tasks", to: "/panel/task-board", note: "Merged into Task Board" },
  { from: "/task-manager", to: "/panel/task-manager", note: "Task Manager panel" },
  { from: "/projects", to: "/panel/task-manager", note: "Projects live in Task Manager" },
  { from: "/projects/[id]", to: "/panel/task-board?project=[id]", note: "Per-project board in Task Board" },
  { from: "/agents", to: "/panel/task-manager", note: "Agents list in Task Manager" },
  { from: "/agents/[id]", to: "/agents/[id]", note: "Rich agent profile kept, linked from Task Manager" },
  { from: "/agents/pentest", to: "/panel/task-manager?agent=pentest", note: "Security Tester in Task Manager" },
  { from: "/agents/review", to: "/panel/task-manager?agent=review", note: "PR Reviewer in Task Manager" },
  { from: "/agents/mail", to: "/panel/task-manager?agent=mail", note: "Mail Agent in Task Manager" },
  { from: "/articles", to: "/panel/task-manager?agent=articles", note: "Articles bot in Task Manager" },
  { from: "/articles/[runId]", to: "/articles/[runId]", note: "Article run detail kept" },
  { from: "/images", to: "/panel/task-manager?agent=images", note: "Images bot in Task Manager" },
  { from: "/sessions", to: "/panel/sessions", note: "Session Manager panel" },
  { from: "/scheduler", to: "/panel/scheduler", note: "Scheduler panel" },
  { from: "/sprints", to: "/panel/sprints", note: "Sprints panel" },
  { from: "/workflows", to: "/panel/workflows", note: "Workflows panel" },
  { from: "/org-builder", to: "/panel/org-builder", note: "Org Builder panel" },
  { from: "/marketplace", to: "/panel/marketplace", note: "Marketplace panel" },
  { from: "/plugins", to: "/panel/plugins-skills", note: "Merged Plugins & Skills" },
  { from: "/skills", to: "/panel/plugins-skills", note: "Merged Plugins & Skills" },
  { from: "/llm-providers", to: "/panel/llm-providers", note: "LLM Providers panel" },
  { from: "/settings", to: "/panel/settings", note: "Settings panel" },
];
