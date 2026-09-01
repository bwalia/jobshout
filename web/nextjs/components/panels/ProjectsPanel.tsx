"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import {
  Archive,
  ArrowLeft,
  Calendar,
  CheckCircle2,
  FolderKanban,
  LayoutGrid,
  List,
  MoreHorizontal,
  Pause,
  Plus,
  RefreshCw,
  Search,
  Star,
  Trash2,
} from "lucide-react";
import { KanbanBoard } from "@/components/kanban/KanbanBoard";
import { CreateTaskDialog } from "@/components/kanban/CreateTaskDialog";
import { TaskDetailModal } from "@/components/kanban/TaskDetailModal";
import { NewProjectDialog } from "@/components/task-manager/NewProjectDialog";
import { formatDateOnly } from "@/lib/dates";
import {
  useDeleteProject,
  useProject,
  useProjects,
  useUpdateProject,
  projectKeys,
} from "@/lib/hooks/useProjects";
import { taskKeys, useAllTasks, useProjectTasks } from "@/lib/hooks/useTasks";
import { THEME_BADGE, STATUS_DOT } from "@/lib/status-colors";
import { PRIORITY_OPTIONS, STATUS_OPTIONS, statusLabel } from "@/lib/task-labels";
import type { Priority, ProjectStatus, TaskStatus } from "@/lib/types/common";
import type { Project, Task } from "@/lib/types/project";
import { cn } from "@/lib/utils/cn";

const STARRED_KEY = "jobshout-starred-projects";

const ACCENTS = [
  "#6366f1",
  "#22c55e",
  "#3b82f6",
  "#f97316",
  "#ec4899",
  "#14b8a6",
  "#8b5cf6",
  "#eab308",
];

const STATUS_META: Record<
  ProjectStatus,
  { label: string; icon: React.ElementType; badge: string }
> = {
  active: {
    label: "Active",
    icon: CheckCircle2,
    badge: THEME_BADGE.success,
  },
  paused: {
    label: "Paused",
    icon: Pause,
    badge: THEME_BADGE.warning,
  },
  completed: {
    label: "Completed",
    icon: CheckCircle2,
    badge: THEME_BADGE.info,
  },
  archived: {
    label: "Archived",
    icon: Archive,
    badge: THEME_BADGE.muted,
  },
};

const PRIORITY_BADGE: Record<Priority, string> = {
  low: THEME_BADGE.muted,
  medium: THEME_BADGE.info,
  high: THEME_BADGE.orange,
  critical: THEME_BADGE.danger,
};

const FILTER_TABS: {
  value: ProjectStatus | "all" | "starred";
  label: string;
  icon?: React.ElementType;
}[] = [
  { value: "all", label: "All" },
  { value: "starred", label: "Starred", icon: Star },
  { value: "active", label: "Active", icon: CheckCircle2 },
  { value: "paused", label: "Paused", icon: Pause },
  { value: "completed", label: "Completed", icon: CheckCircle2 },
  { value: "archived", label: "Archived", icon: Archive },
];

type FilterTab = (typeof FILTER_TABS)[number]["value"];
type LayoutMode = "grid" | "list";
type ProjectView = "board" | "tasks";

function accentFor(id: string): string {
  let h = 0;
  for (let i = 0; i < id.length; i++) {
    h = (h * 31 + id.charCodeAt(i)) >>> 0;
  }
  return ACCENTS[h % ACCENTS.length];
}

function readStarred(): Set<string> {
  try {
    const raw = localStorage.getItem(STARRED_KEY);
    if (!raw) return new Set();
    const parsed = JSON.parse(raw) as unknown;
    if (!Array.isArray(parsed)) return new Set();
    return new Set(parsed.filter((x): x is string => typeof x === "string"));
  } catch {
    return new Set();
  }
}

function writeStarred(ids: Set<string>) {
  try {
    localStorage.setItem(STARRED_KEY, JSON.stringify(Array.from(ids)));
  } catch {
    /* ignore quota / private mode */
  }
}

function formatDue(iso: string | null): string | null {
  if (!iso) return null;
  return formatDateOnly(iso, { month: "short", day: "numeric", year: "numeric" });
}

function panelHref(next: {
  project?: string | null;
  view?: ProjectView | null;
  task?: string | null;
}): string {
  const params = new URLSearchParams();
  if (next.project) params.set("project", next.project);
  if (next.view && next.view !== "board") params.set("view", next.view);
  if (next.task) params.set("task", next.task);
  const qs = params.toString();
  return qs ? `/panel/projects?${qs}` : "/panel/projects";
}

export function ProjectsPanel() {
  const searchParams = useSearchParams();
  const projectId = searchParams.get("project");
  const viewParam = searchParams.get("view");
  const taskId = searchParams.get("task");
  const view: ProjectView = viewParam === "tasks" ? "tasks" : "board";

  if (projectId) {
    return (
      <ProjectDetail
        projectId={projectId}
        view={view}
        taskId={taskId}
      />
    );
  }

  return <ProjectListing />;
}

function ProjectListing() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { data, isLoading, isError, isFetching, refetch } = useProjects({
    per_page: 100,
  });
  const { data: tasksResp } = useAllTasks();
  const updateProject = useUpdateProject();
  const deleteProject = useDeleteProject();

  const projects = useMemo(() => data?.data ?? [], [data]);
  const [createOpen, setCreateOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [activeTab, setActiveTab] = useState<FilterTab>("all");
  const [layout, setLayout] = useState<LayoutMode>("grid");
  const [starred, setStarred] = useState<Set<string>>(new Set());
  const [menuId, setMenuId] = useState<string | null>(null);

  useEffect(() => {
    setStarred(readStarred());
  }, []);

  const taskStats = useMemo(() => {
    const counts = new Map<string, { total: number; done: number }>();
    for (const t of tasksResp?.data ?? []) {
      const cur = counts.get(t.project_id) ?? { total: 0, done: 0 };
      cur.total += 1;
      if (t.status === "done") cur.done += 1;
      counts.set(t.project_id, cur);
    }
    return counts;
  }, [tasksResp]);

  const filtered = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    return projects.filter((p) => {
      if (activeTab === "starred" && !starred.has(p.id)) return false;
      if (activeTab !== "all" && activeTab !== "starred" && p.status !== activeTab) {
        return false;
      }
      if (!q) return true;
      return (
        p.name.toLowerCase().includes(q) ||
        (p.description ?? "").toLowerCase().includes(q)
      );
    });
  }, [projects, searchQuery, activeTab, starred]);

  const stats = useMemo(
    () => ({
      total: projects.length,
      active: projects.filter((p) => p.status === "active").length,
      completed: projects.filter((p) => p.status === "completed").length,
      starred: starred.size,
    }),
    [projects, starred]
  );

  const openProject = useCallback(
    (project: Project) => {
      router.push(panelHref({ project: project.id }));
    },
    [router]
  );

  const toggleStar = useCallback((project: Project) => {
    setStarred((prev) => {
      const next = new Set(prev);
      if (next.has(project.id)) next.delete(project.id);
      else next.add(project.id);
      writeStarred(next);
      return next;
    });
  }, []);

  const archiveProject = useCallback(
    async (project: Project) => {
      if (!window.confirm(`Archive "${project.name}"?`)) return;
      await updateProject.mutateAsync({
        id: project.id,
        payload: { status: "archived" },
      });
    },
    [updateProject]
  );

  const removeProject = useCallback(
    async (project: Project) => {
      if (
        !window.confirm(
          `Delete "${project.name}"? This cannot be undone.`
        )
      ) {
        return;
      }
      await deleteProject.mutateAsync(project.id);
    },
    [deleteProject]
  );

  const isFiltered = Boolean(searchQuery.trim()) || activeTab !== "all";

  return (
    <div className="mx-auto max-w-7xl space-y-6 p-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Projects</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Your projects — open one to see its tasks
          </p>
        </div>
        <button
          type="button"
          onClick={() => setCreateOpen(true)}
          className="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-3.5 text-sm font-medium text-primary-foreground hover:opacity-90"
        >
          <Plus className="h-4 w-4" /> New Project
        </button>
      </div>

      {projects.length > 0 && (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          <StatCard value={stats.total} label="Total Projects" />
          <StatCard
            value={stats.active}
            label="Active"
            valueClass="text-emerald-600 dark:text-emerald-400"
          />
          <StatCard
            value={stats.completed}
            label="Completed"
            valueClass="text-blue-600 dark:text-blue-400"
          />
          <StatCard
            value={stats.starred}
            label="Starred"
            valueClass="text-amber-600 dark:text-amber-400"
          />
        </div>
      )}

      <div className="rounded-xl border border-border bg-card p-4">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="flex items-center gap-1 overflow-x-auto pb-1 lg:pb-0">
            {FILTER_TABS.map((tab) => {
              const Icon = tab.icon;
              return (
                <button
                  key={tab.value}
                  type="button"
                  onClick={() => setActiveTab(tab.value)}
                  className={cn(
                    "flex items-center gap-1.5 whitespace-nowrap rounded-lg px-3 py-2 text-sm font-medium transition-colors",
                    activeTab === tab.value
                      ? "bg-primary/10 text-primary"
                      : "text-muted-foreground hover:bg-secondary hover:text-foreground"
                  )}
                >
                  {Icon && <Icon className="h-3.5 w-3.5" />}
                  {tab.label}
                </button>
              );
            })}
          </div>

          <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
            <div className="relative flex-1 sm:flex-initial">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <input
                type="search"
                placeholder="Search projects…"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="h-9 w-full rounded-md border border-input bg-background pl-9 pr-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:w-64"
              />
            </div>
            <div className="flex items-center gap-2">
              <div className="flex rounded-lg bg-secondary p-0.5">
                <button
                  type="button"
                  title="Grid view"
                  onClick={() => setLayout("grid")}
                  className={cn(
                    "rounded-md p-2 transition-colors",
                    layout === "grid"
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  <LayoutGrid className="h-4 w-4" />
                </button>
                <button
                  type="button"
                  title="List view"
                  onClick={() => setLayout("list")}
                  className={cn(
                    "rounded-md p-2 transition-colors",
                    layout === "list"
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  <List className="h-4 w-4" />
                </button>
              </div>
              <button
                type="button"
                title="Refresh"
                onClick={() => {
                  void refetch();
                  void queryClient.invalidateQueries({ queryKey: taskKeys.all });
                  void queryClient.invalidateQueries({ queryKey: projectKeys.all });
                }}
                disabled={isFetching}
                className="rounded-lg p-2 text-muted-foreground hover:bg-secondary hover:text-foreground disabled:opacity-50"
              >
                <RefreshCw className={cn("h-4 w-4", isFetching && "animate-spin")} />
              </button>
            </div>
          </div>
        </div>
      </div>

      {isError && (
        <div className="rounded-xl border border-destructive/40 bg-destructive/10 p-4">
          <p className="text-sm text-destructive">Failed to load projects.</p>
          <button
            type="button"
            onClick={() => void refetch()}
            className="mt-2 h-8 rounded-md border border-border px-3 text-sm hover:bg-secondary"
          >
            Try again
          </button>
        </div>
      )}

      {isLoading && projects.length === 0 && <ListingSkeleton layout={layout} />}

      {!isLoading && projects.length === 0 && (
        <EmptyState
          onCreate={() => setCreateOpen(true)}
        />
      )}

      {!isLoading && projects.length > 0 && filtered.length === 0 && (
        <EmptyState
          filtered={isFiltered}
          onClear={() => {
            setSearchQuery("");
            setActiveTab("all");
          }}
        />
      )}

      {filtered.length > 0 && (
        layout === "list" ? (
          <div className="space-y-3">
            {filtered.map((project) => (
              <ProjectCardItem
                key={project.id}
                project={project}
                layout="list"
                isStarred={starred.has(project.id)}
                stats={taskStats.get(project.id)}
                menuOpen={menuId === project.id}
                onOpen={() => openProject(project)}
                onStar={() => toggleStar(project)}
                onArchive={() => void archiveProject(project)}
                onDelete={() => void removeProject(project)}
                onToggleMenu={() =>
                  setMenuId((id) => (id === project.id ? null : project.id))
                }
                onCloseMenu={() => setMenuId(null)}
              />
            ))}
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {filtered.map((project) => (
              <ProjectCardItem
                key={project.id}
                project={project}
                layout="grid"
                isStarred={starred.has(project.id)}
                stats={taskStats.get(project.id)}
                menuOpen={menuId === project.id}
                onOpen={() => openProject(project)}
                onStar={() => toggleStar(project)}
                onArchive={() => void archiveProject(project)}
                onDelete={() => void removeProject(project)}
                onToggleMenu={() =>
                  setMenuId((id) => (id === project.id ? null : project.id))
                }
                onCloseMenu={() => setMenuId(null)}
              />
            ))}
          </div>
        )
      )}

      {createOpen && (
        <NewProjectDialog
          onClose={() => setCreateOpen(false)}
          onCreated={(p) => {
            setCreateOpen(false);
            router.push(panelHref({ project: p.id }));
          }}
        />
      )}
    </div>
  );
}

function ProjectDetail({
  projectId,
  view,
  taskId,
}: {
  projectId: string;
  view: ProjectView;
  taskId: string | null;
}) {
  const router = useRouter();
  const { data: project, isLoading, isError } = useProject(projectId);
  const { data: tasksResp, isLoading: tasksLoading } = useProjectTasks(projectId);
  const tasks = useMemo(() => tasksResp?.data ?? [], [tasksResp]);
  const [createOpen, setCreateOpen] = useState(false);
  const selected = tasks.find((t) => t.id === taskId) ?? null;

  function go(next: { view?: ProjectView | null; task?: string | null }) {
    router.replace(
      panelHref({
        project: projectId,
        view: next.view === undefined ? view : next.view,
        task: next.task === undefined ? taskId : next.task,
      }),
      { scroll: false }
    );
  }

  const statusMeta = project
    ? STATUS_META[project.status] ?? STATUS_META.active
    : null;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border bg-background px-6 py-4">
        <div className="flex min-w-0 items-center gap-3">
          <button
            type="button"
            onClick={() => router.push("/panel/projects")}
            className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-secondary hover:text-foreground"
            aria-label="Back to projects"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div className="min-w-0">
            {isLoading ? (
              <div className="h-6 w-48 animate-pulse rounded bg-muted" />
            ) : (
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="truncate text-xl font-bold tracking-tight">
                  {project?.name ?? "Project"}
                </h1>
                {statusMeta && (
                  <StatusPill meta={statusMeta} />
                )}
              </div>
            )}
            {project?.description && (
              <p className="mt-0.5 line-clamp-1 text-sm text-muted-foreground">
                {project.description}
              </p>
            )}
          </div>
        </div>

        <div className="flex items-center gap-2">
          <div className="flex items-center rounded-lg border border-border bg-background">
            <button
              type="button"
              onClick={() => go({ view: "board", task: null })}
              className={cn(
                "inline-flex items-center gap-1.5 rounded-lg px-3 py-2 text-sm font-medium",
                view === "board"
                  ? "bg-secondary text-foreground"
                  : "text-muted-foreground hover:text-foreground"
              )}
            >
              <LayoutGrid className="h-4 w-4" />
              Board
            </button>
            <button
              type="button"
              onClick={() => go({ view: "tasks", task: null })}
              className={cn(
                "inline-flex items-center gap-1.5 rounded-lg px-3 py-2 text-sm font-medium",
                view === "tasks"
                  ? "bg-secondary text-foreground"
                  : "text-muted-foreground hover:text-foreground"
              )}
            >
              <List className="h-4 w-4" />
              Tasks
            </button>
          </div>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-hidden">
        {isError ? (
          <div className="flex h-full items-center justify-center p-6">
            <div className="rounded-xl border border-destructive/50 bg-destructive/10 p-8 text-center">
              <p className="font-medium text-destructive">Failed to load project</p>
              <button
                type="button"
                onClick={() => router.push("/panel/projects")}
                className="mt-4 inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground"
              >
                Back to Projects
              </button>
            </div>
          </div>
        ) : view === "board" ? (
          <KanbanBoard
            projectId={projectId}
            projectName={project?.name}
            onOpenTask={(task) => go({ task: task.id })}
          />
        ) : (
          <div className="h-full overflow-auto p-4">
            <TaskList
              tasks={tasks}
              isLoading={tasksLoading}
              onOpen={(task) => go({ task: task.id })}
              onCreate={() => setCreateOpen(true)}
            />
          </div>
        )}
      </div>

      {createOpen && (
        <CreateTaskDialog
          projectId={projectId}
          onClose={() => setCreateOpen(false)}
        />
      )}
      {selected && (
        <TaskDetailModal
          task={selected}
          onClose={() => go({ task: null })}
        />
      )}
    </div>
  );
}

function TaskList({
  tasks,
  isLoading,
  onOpen,
  onCreate,
}: {
  tasks: Task[];
  isLoading: boolean;
  onOpen: (task: Task) => void;
  onCreate: () => void;
}) {
  const grouped = useMemo(() => {
    const map = new Map<TaskStatus, Task[]>();
    for (const s of STATUS_OPTIONS) map.set(s.value, []);
    for (const t of tasks) {
      (map.get(t.status) ?? map.get("backlog")!).push(t);
    }
    return STATUS_OPTIONS.map((s) => ({
      ...s,
      items: map.get(s.value) ?? [],
    })).filter((g) => g.items.length > 0);
  }, [tasks]);

  if (isLoading) {
    return (
      <p className="flex h-40 items-center justify-center text-sm text-muted-foreground">
        Loading tasks…
      </p>
    );
  }

  if (tasks.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border py-16 text-center">
        <p className="text-sm font-medium">No tasks yet</p>
        <p className="mt-1 text-sm text-muted-foreground">
          Create a task to start tracking work in this project.
        </p>
        <button
          type="button"
          onClick={onCreate}
          className="mt-4 inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground"
        >
          <Plus className="h-4 w-4" /> New task
        </button>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <p className="text-sm text-muted-foreground">
        {tasks.length} task{tasks.length === 1 ? "" : "s"}
      </p>
      {grouped.map((group) => (
        <section key={group.value}>
          <h2 className="mb-2 flex items-center gap-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            <span className={cn("h-2 w-2 rounded-full", STATUS_DOT[group.value])} />
            {group.label}
            <span className="font-mono text-[10px]">{group.items.length}</span>
          </h2>
          <ul className="space-y-1.5">
            {group.items.map((task) => (
              <li key={task.id}>
                <button
                  type="button"
                  onClick={() => onOpen(task)}
                  className="flex w-full items-center gap-3 rounded-lg border border-border bg-card px-3 py-2.5 text-left transition-colors hover:bg-secondary/60"
                >
                  <span
                    className={cn(
                      "h-2 w-2 shrink-0 rounded-full",
                      STATUS_DOT[task.status]
                    )}
                  />
                  <span className="min-w-0 flex-1 truncate text-sm font-medium">
                    {task.title}
                  </span>
                  <span
                    className={cn(
                      "hidden rounded-full px-2 py-0.5 text-[11px] font-medium sm:inline-flex",
                      PRIORITY_BADGE[task.priority] ?? THEME_BADGE.muted
                    )}
                  >
                    {PRIORITY_OPTIONS.find((p) => p.value === task.priority)?.label ??
                      task.priority}
                  </span>
                  <span className="hidden w-24 shrink-0 text-right text-xs text-muted-foreground md:block">
                    {formatDue(task.due_date) ?? statusLabel(task.status)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

function ProjectCardItem({
  project,
  layout,
  isStarred,
  stats,
  menuOpen,
  onOpen,
  onStar,
  onArchive,
  onDelete,
  onToggleMenu,
  onCloseMenu,
}: {
  project: Project;
  layout: LayoutMode;
  isStarred: boolean;
  stats?: { total: number; done: number };
  menuOpen: boolean;
  onOpen: () => void;
  onStar: () => void;
  onArchive: () => void;
  onDelete: () => void;
  onToggleMenu: () => void;
  onCloseMenu: () => void;
}) {
  const status = STATUS_META[project.status] ?? STATUS_META.active;
  const StatusIcon = status.icon;
  const color = accentFor(project.id);
  const due = formatDue(project.due_date);
  const total = stats?.total ?? 0;
  const done = stats?.done ?? 0;

  function handleClick(e: React.MouseEvent) {
    if ((e.target as HTMLElement).closest("[data-action]")) return;
    onOpen();
  }

  const menu = (
    <div className="relative" data-action="true">
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          onToggleMenu();
        }}
        className={cn(
          "rounded-lg p-1.5 text-muted-foreground hover:bg-secondary hover:text-foreground",
          layout === "grid" && "opacity-0 transition-opacity group-hover:opacity-100"
        )}
        aria-label="Project actions"
      >
        <MoreHorizontal className="h-4 w-4" />
      </button>
      {menuOpen && (
        <>
          <div className="fixed inset-0 z-10" onClick={onCloseMenu} />
          <div
            className={cn(
              "absolute right-0 z-20 w-44 rounded-lg border border-border bg-card py-1 shadow-lg",
              layout === "grid" ? "bottom-full mb-1" : "top-full mt-1"
            )}
          >
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onCloseMenu();
                onOpen();
              }}
              className="flex w-full items-center gap-2 px-3 py-2 text-sm hover:bg-secondary"
            >
              <FolderKanban className="h-3.5 w-3.5" /> Open
            </button>
            {project.status !== "archived" && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  onCloseMenu();
                  onArchive();
                }}
                className="flex w-full items-center gap-2 px-3 py-2 text-sm hover:bg-secondary"
              >
                <Archive className="h-3.5 w-3.5" /> Archive
              </button>
            )}
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onCloseMenu();
                onDelete();
              }}
              className="flex w-full items-center gap-2 px-3 py-2 text-sm text-destructive hover:bg-destructive/10"
            >
              <Trash2 className="h-3.5 w-3.5" /> Delete
            </button>
          </div>
        </>
      )}
    </div>
  );

  if (layout === "list") {
    return (
      <div
        onClick={handleClick}
        className="group cursor-pointer rounded-xl border border-border bg-card transition-shadow hover:shadow-md"
      >
        <div className="flex items-center gap-4 p-4">
          <div
            className="h-12 w-1 shrink-0 rounded-full"
            style={{ backgroundColor: color }}
          />
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <h3 className="truncate font-semibold">{project.name}</h3>
              <span
                className={cn(
                  "inline-flex shrink-0 items-center rounded-full px-2 py-0.5 text-xs font-medium",
                  status.badge
                )}
              >
                {status.label}
              </span>
            </div>
            {project.description && (
              <p className="mt-1 truncate text-sm text-muted-foreground">
                {project.description}
              </p>
            )}
          </div>
          <div className="hidden items-center gap-6 text-sm text-muted-foreground md:flex">
            <span className="flex items-center gap-1.5">
              <CheckCircle2 className="h-3.5 w-3.5" />
              {total} tasks
            </span>
            {due && (
              <span className="flex items-center gap-1.5">
                <Calendar className="h-3.5 w-3.5" />
                {due}
              </span>
            )}
          </div>
          <div className="flex items-center gap-1" data-action="true">
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onStar();
              }}
              className={cn(
                "rounded-lg p-2 transition-colors",
                isStarred
                  ? "text-amber-500 hover:bg-amber-50 dark:hover:bg-amber-500/10"
                  : "text-muted-foreground hover:bg-secondary hover:text-amber-500"
              )}
              aria-label={isStarred ? "Unstar project" : "Star project"}
            >
              <Star className="h-4 w-4" fill={isStarred ? "currentColor" : "none"} />
            </button>
            {menu}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div
      onClick={handleClick}
      className="group cursor-pointer overflow-hidden rounded-xl border border-border bg-card transition-shadow hover:shadow-md"
    >
      <div className="h-2" style={{ backgroundColor: color }} />
      <div className="p-5">
        <div className="mb-3 flex items-start justify-between gap-2">
          <div className="min-w-0 flex-1">
            <h3 className="truncate font-semibold">{project.name}</h3>
            <span
              className={cn(
                "mt-1 inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium",
                status.badge
              )}
            >
              <StatusIcon className="h-3 w-3" />
              {status.label}
            </span>
          </div>
          <button
            type="button"
            data-action="true"
            onClick={(e) => {
              e.stopPropagation();
              onStar();
            }}
            className={cn(
              "rounded-lg p-1.5 transition-colors",
              isStarred
                ? "text-amber-500"
                : "text-muted-foreground opacity-0 group-hover:opacity-100 hover:text-amber-500"
            )}
            aria-label={isStarred ? "Unstar project" : "Star project"}
          >
            <Star className="h-4 w-4" fill={isStarred ? "currentColor" : "none"} />
          </button>
        </div>

        {project.description && (
          <p className="mb-4 line-clamp-2 text-sm text-muted-foreground">
            {project.description}
          </p>
        )}

        {total > 0 && (
          <div className="mb-4">
            <div className="mb-1 flex items-center justify-between text-xs text-muted-foreground">
              <span>Progress</span>
              <span>
                {done}/{total} tasks
              </span>
            </div>
            <div className="h-1.5 overflow-hidden rounded-full bg-secondary">
              <div
                className="h-full rounded-full bg-primary transition-all"
                style={{ width: `${Math.round((done / total) * 100)}%` }}
              />
            </div>
          </div>
        )}

        <div className="mb-4 flex items-center gap-3 text-sm text-muted-foreground">
          <span
            className={cn(
              "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium capitalize",
              PRIORITY_BADGE[project.priority] ?? THEME_BADGE.muted
            )}
          >
            {project.priority}
          </span>
          <span className="flex items-center gap-1">
            <CheckCircle2 className="h-3.5 w-3.5" />
            {total} tasks
          </span>
        </div>

        <div className="flex items-center justify-between border-t border-border pt-3">
          {due ? (
            <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <Calendar className="h-3 w-3" />
              Due {due}
            </span>
          ) : (
            <span />
          )}
          {menu}
        </div>
      </div>
    </div>
  );
}

function StatusPill({
  meta,
}: {
  meta: { label: string; icon: React.ElementType; badge: string };
}) {
  const Icon = meta.icon;
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium",
        meta.badge
      )}
    >
      <Icon className="h-3 w-3" />
      {meta.label}
    </span>
  );
}

function StatCard({
  value,
  label,
  valueClass,
}: {
  value: number;
  label: string;
  valueClass?: string;
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className={cn("text-2xl font-bold", valueClass)}>{value}</div>
      <div className="text-sm text-muted-foreground">{label}</div>
    </div>
  );
}

function EmptyState({
  filtered,
  onCreate,
  onClear,
}: {
  filtered?: boolean;
  onCreate?: () => void;
  onClear?: () => void;
}) {
  if (filtered) {
    return (
      <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border py-16 text-center">
        <Search className="mb-3 h-8 w-8 text-muted-foreground" />
        <h3 className="text-lg font-semibold">No projects found</h3>
        <p className="mt-1 max-w-sm text-sm text-muted-foreground">
          Nothing matches these filters. Try a different search or tab.
        </p>
        {onClear && (
          <button
            type="button"
            onClick={onClear}
            className="mt-4 h-9 rounded-md border border-border px-4 text-sm hover:bg-secondary"
          >
            Clear filters
          </button>
        )}
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-border py-16 text-center">
      <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-primary/10">
        <FolderKanban className="h-8 w-8 text-primary" />
      </div>
      <h3 className="text-lg font-semibold">No projects yet</h3>
      <p className="mt-1 max-w-md text-sm text-muted-foreground">
        Create a project to organise tasks, then open it to work the board.
      </p>
      {onCreate && (
        <button
          type="button"
          onClick={onCreate}
          className="mt-6 inline-flex h-10 items-center gap-2 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground"
        >
          <Plus className="h-4 w-4" /> Create your first project
        </button>
      )}
    </div>
  );
}

function ListingSkeleton({ layout }: { layout: LayoutMode }) {
  if (layout === "list") {
    return (
      <div className="space-y-3">
        {Array.from({ length: 5 }).map((_, i) => (
          <div
            key={i}
            className="h-20 animate-pulse rounded-xl border border-border bg-muted"
          />
        ))}
      </div>
    );
  }
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {Array.from({ length: 8 }).map((_, i) => (
        <div
          key={i}
          className="h-48 animate-pulse rounded-xl border border-border bg-muted"
        />
      ))}
    </div>
  );
}
