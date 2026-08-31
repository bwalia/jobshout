import type { LucideIcon } from "lucide-react";
import {
  Bot,
  LayoutDashboard,
  MessageSquare,
  FolderKanban,
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
  Archive,
} from "lucide-react";

export type PanelId =
  | "chat"
  | "dashboard"
  | "projects"
  | "task-board"
  | "task-manager"
  | "artifacts"
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
  { id: "projects", label: "Projects", href: "/panel/projects", icon: FolderKanban },
  { id: "task-board", label: "Task Board", href: "/panel/task-board", icon: Kanban },
  { id: "task-manager", label: "Task Manager", href: "/panel/task-manager", icon: ListTree },
  { id: "artifacts", label: "Artifacts", href: "/panel/artifacts", icon: Archive },
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
  return legacyPanelFromPath(pathname) ?? "chat";
}

const LEGACY_PREFIXES: { prefix: string; id: PanelId }[] = [
  { prefix: "/task-manager", id: "task-manager" },
  { prefix: "/agent-board", id: "task-board" },
  { prefix: "/llm-providers", id: "llm-providers" },
  { prefix: "/org-builder", id: "org-builder" },
  { prefix: "/marketplace", id: "marketplace" },
  { prefix: "/plugins", id: "plugins-skills" },
  { prefix: "/skills", id: "plugins-skills" },
  { prefix: "/scheduler", id: "scheduler" },
  { prefix: "/sprints", id: "sprints" },
  { prefix: "/sessions", id: "sessions" },
  { prefix: "/workflows", id: "workflows" },
  { prefix: "/artifacts", id: "artifacts" },
  { prefix: "/articles", id: "artifacts" },
  { prefix: "/images", id: "task-manager" },
  { prefix: "/dashboard", id: "dashboard" },
  { prefix: "/metrics", id: "dashboard" },
  { prefix: "/settings", id: "settings" },
  { prefix: "/projects", id: "projects" },
  { prefix: "/agents", id: "task-manager" },
  { prefix: "/tasks", id: "task-board" },
];

function legacyPanelFromPath(pathname: string): PanelId | null {
  for (const { prefix, id } of LEGACY_PREFIXES) {
    if (pathname === prefix || pathname.startsWith(`${prefix}/`)) return id;
  }
  return null;
}

/** Workflows — top-level Automations item (Cursor "Codebase" analogue). */
export const AUTOMATIONS_HREF = "/panel/workflows";

/**
 * Always-visible sidebar items. Dashboard opens the workspace; clicking it
 * again while you are on Dashboard tucks APP_NAV_PANELS away underneath it.
 */
export const SIDEBAR_PRIMARY: { id: string; label: string; href: string; icon: LucideIcon }[] = [
  { id: "automations", label: "Automations", href: AUTOMATIONS_HREF, icon: Bot },
  { id: "dashboard", label: "Dashboard", href: "/panel/dashboard", icon: LayoutDashboard },
];

/** Extra menus shown once Dashboard (or any panel) is open. Dashboard stays in primary. */
export const APP_NAV_PANELS: PanelDef[] = PANELS.filter(
  (p) => p.id !== "chat" && p.id !== "dashboard" && p.id !== "workflows"
);

export function isAppNavPath(pathname: string): boolean {
  if (pathname === "/chat" || pathname.startsWith("/chat/")) return false;
  return true;
}

export function rememberPanelTransition(from: PanelId, to: PanelId) {
  if (from === to) return;
  try {
    sessionStorage.setItem("jobshout-panel-from", from);
    sessionStorage.setItem("jobshout-panel-to", to);
  } catch {
    /* ignore */
  }
}

/** Old route → new location (for redirects + migration checklist). */
export const ROUTE_MIGRATION: { from: string; to: string; note: string }[] = [
  { from: "/", to: "/chat", note: "Chat is home" },
  { from: "/dashboard", to: "/panel/dashboard", note: "Merged into Dashboard panel" },
  { from: "/metrics", to: "/panel/dashboard", note: "Merged into Dashboard panel" },
  { from: "/agent-board", to: "/panel/task-board", note: "Agents view inside Task Board" },
  { from: "/tasks", to: "/panel/task-board", note: "Merged into Task Board" },
  { from: "/task-manager", to: "/panel/task-manager", note: "Task Manager panel" },
  { from: "/projects", to: "/panel/projects", note: "Projects panel" },
  { from: "/projects/[id]", to: "/panel/projects?project=[id]", note: "Project tasks under Projects" },
  { from: "/agents", to: "/panel/task-manager", note: "Agents list in Task Manager" },
  { from: "/agents/[id]", to: "/agents/[id]", note: "Rich agent profile kept, linked from Task Manager" },
  { from: "/agents/pentest", to: "/panel/task-manager?agent=pentest", note: "Security Tester in Task Manager" },
  { from: "/agents/review", to: "/panel/task-manager?agent=review", note: "PR Reviewer in Task Manager" },
  { from: "/agents/mail", to: "/panel/task-manager?agent=mail", note: "Mail Agent in Task Manager" },
  { from: "/articles", to: "/panel/task-manager?agent=articles", note: "Articles bot in Task Manager" },
  { from: "/articles/[runId]", to: "/articles/[runId]", note: "Article run detail kept" },
  { from: "/artifacts", to: "/panel/artifacts", note: "Artifacts library panel" },
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
