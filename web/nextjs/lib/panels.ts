import type { LucideIcon } from "lucide-react";
import {
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
  ShieldAlert,
  Gauge,
  Bot,
  Activity,
  BarChart3,
} from "lucide-react";

export type PanelId =
  | "chat"
  | "dashboard"
  | "projects"
  | "agents"
  | "tasks"
  | "runs"
  | "usage"
  | "task-board"
  | "task-manager"
  | "security-tester"
  | "artifacts"
  | "scheduler"
  | "sprints"
  | "sessions"
  | "workflows"
  | "org-builder"
  | "marketplace"
  | "plugins-skills"
  | "llm-providers"
  | "llm-benchmarks"
  | "settings";

export interface PanelDef {
  id: PanelId;
  label: string;
  href: string;
  icon: LucideIcon;
  /**
   * Hidden from the left nav, but still a valid /panel route. Reached via the
   * dashboard card, command palette, or a direct link rather than a nav tab.
   */
  hidden?: boolean;
}

/** Menu order — Chat first, then Dashboard, then the rest. */
export const PANELS: PanelDef[] = [
  { id: "chat", label: "Chat", href: "/chat", icon: MessageSquare },
  { id: "dashboard", label: "Dashboard", href: "/panel/dashboard", icon: LayoutDashboard },
  { id: "projects", label: "Projects", href: "/panel/projects", icon: FolderKanban },
  { id: "tasks", label: "Tasks", href: "/panel/tasks", icon: Kanban },
  { id: "runs", label: "Runs", href: "/panel/runs", icon: Activity },
  { id: "usage", label: "Usage", href: "/panel/usage", icon: BarChart3, hidden: true },
  // Folded into Tasks; kept as a redirect for old links.
  { id: "task-board", label: "Task Board", href: "/panel/task-board", icon: Kanban, hidden: true },
  // Agents section lives in lib/nav.ts (sidebar). Hidden here so we don't teach
  // a second "Agents / Task Manager" destination alongside All agents.
  { id: "agents", label: "All agents", href: "/panel/agents", icon: Bot, hidden: true },
  // Legacy redirect → agent Workspace. Do not surface in nav or palette copy.
  { id: "task-manager", label: "All agents (legacy)", href: "/panel/task-manager", icon: ListTree, hidden: true },
  { id: "security-tester", label: "Security Tester", href: "/panel/security-tester", icon: ShieldAlert, hidden: true },
  // Labels match lib/nav.ts so panel transitions and migration notes stay consistent.
  { id: "artifacts", label: "Outputs", href: "/panel/artifacts", icon: Archive },
  { id: "scheduler", label: "Schedules", href: "/panel/scheduler", icon: Clock },
  { id: "sprints", label: "Sprints", href: "/panel/sprints", icon: Goal },
  { id: "sessions", label: "Sessions", href: "/panel/sessions", icon: History },
  { id: "workflows", label: "Workflows", href: "/panel/workflows", icon: Workflow },
  { id: "org-builder", label: "Org chart", href: "/panel/org-builder", icon: Network },
  { id: "marketplace", label: "Marketplace", href: "/panel/marketplace", icon: Store },
  { id: "plugins-skills", label: "Skills & plugins", href: "/panel/plugins-skills", icon: Puzzle },
  { id: "llm-providers", label: "Models & providers", href: "/panel/llm-providers", icon: Cpu },
  { id: "llm-benchmarks", label: "LLM Benchmarks", href: "/panel/llm-providers?tab=benchmarks", icon: Gauge, hidden: true },
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
  { prefix: "/agents", id: "agents" },
  { prefix: "/tasks", id: "tasks" },
];

function legacyPanelFromPath(pathname: string): PanelId | null {
  for (const { prefix, id } of LEGACY_PREFIXES) {
    if (pathname === prefix || pathname.startsWith(`${prefix}/`)) return id;
  }
  return null;
}

/** Workflows — always-visible sidebar item (same destination as the Workflows panel). */
export const AUTOMATIONS_HREF = "/panel/workflows";

/**
 * Always-visible sidebar items. Dashboard opens the workspace; clicking it
 * again while you are on Dashboard tucks APP_NAV_PANELS away underneath it.
 */
export const SIDEBAR_PRIMARY: { id: string; label: string; href: string; icon: LucideIcon }[] = [
  { id: "workflows", label: "Workflows", href: AUTOMATIONS_HREF, icon: Workflow },
  { id: "dashboard", label: "Dashboard", href: "/panel/dashboard", icon: LayoutDashboard },
];

/** Extra menus shown once Dashboard (or any panel) is open. Dashboard stays in primary. */
export const APP_NAV_PANELS: PanelDef[] = PANELS.filter(
  (p) => p.id !== "chat" && p.id !== "dashboard" && p.id !== "workflows" && !p.hidden
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
  { from: "/task-manager", to: "/panel/task-manager", note: "Legacy; redirects to agent Workspace or All agents" },
  { from: "/projects", to: "/panel/projects", note: "Projects panel" },
  { from: "/projects/[id]", to: "/panel/projects?project=[id]", note: "Project tasks under Projects" },
  { from: "/agents", to: "/panel/agents", note: "All agents list" },
  { from: "/agents/[id]", to: "/panel/agents/[id]", note: "Agent profile under /panel" },
  { from: "/agents/pentest", to: "/panel/task-manager?agent=pentest", note: "→ Security Tester Workspace" },
  { from: "/agents/review", to: "/panel/task-manager?agent=review", note: "→ PR Reviewer Workspace" },
  { from: "/agents/mail", to: "/panel/task-manager?agent=mail", note: "→ Mail Agent Workspace" },
  { from: "/articles", to: "/panel/task-manager?agent=articles", note: "→ Article Writer Workspace" },
  { from: "/articles/[runId]", to: "/articles/[runId]", note: "Article run detail kept" },
  { from: "/artifacts", to: "/panel/artifacts", note: "Outputs panel" },
  { from: "/images", to: "/panel/task-manager?agent=images", note: "→ Images Workspace" },
  { from: "/sessions", to: "/panel/sessions", note: "Model sessions panel" },
  { from: "/scheduler", to: "/panel/scheduler", note: "Schedules panel" },
  { from: "/sprints", to: "/panel/sprints", note: "Sprints panel" },
  { from: "/workflows", to: "/panel/workflows", note: "Workflows panel" },
  { from: "/org-builder", to: "/panel/org-builder", note: "Org chart panel" },
  { from: "/marketplace", to: "/panel/marketplace", note: "Marketplace panel" },
  { from: "/plugins", to: "/panel/plugins-skills", note: "Skills & plugins" },
  { from: "/skills", to: "/panel/plugins-skills", note: "Skills & plugins" },
  { from: "/llm-providers", to: "/panel/llm-providers", note: "Models & providers" },
  { from: "/settings", to: "/panel/settings", note: "Settings panel" },
];
