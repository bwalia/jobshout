"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { AlertTriangle, LayoutGrid, List, Loader2, Plus, Search, X } from "lucide-react";
import { KanbanBoard } from "@/components/kanban/KanbanBoard";
import { TaskCardFace } from "@/components/kanban/TaskCard";
import { CreateTaskDialog } from "@/components/kanban/CreateTaskDialog";
import { TaskDetailModal } from "@/components/kanban/TaskDetailModal";
import { TaskProgressChip, TaskCountLabel } from "@/components/task-manager/TaskProgressChip";
import { agentHref } from "@/lib/agents/links";
import { useAgents } from "@/lib/hooks/useAgents";
import { useProjects } from "@/lib/hooks/useProjects";
import { useRunFailures } from "@/lib/hooks/useRunFailures";
import { useAllTasks, useTask } from "@/lib/hooks/useTasks";
import { STATUS_DOT } from "@/lib/status-colors";
import { PRIORITY_OPTIONS, STATUS_OPTIONS, priorityLabel, statusLabel } from "@/lib/task-labels";
import type { Task } from "@/lib/types/project";
import { cn } from "@/lib/utils/cn";
import { useLocalState } from "@/lib/utils/local-state";

type Layout = "board" | "list";
type GroupBy = "status" | "agent" | "project";

/** URL keys this view owns. Anything else on the URL is left alone. */
const KEYS = ["q", "agent", "status", "priority", "failed", "group", "task", "run", "new"] as const;

const selectCls =
  "h-9 rounded-md border border-input bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

interface Column {
  key: string;
  label: string;
  dot?: string;
  href?: string;
  tasks: Task[];
}

/**
 * The one task view: board or list, filtered and grouped, with every choice in
 * the URL so a view can be shared. On a project page it is locked to that
 * project. Board-by-status for a single project keeps drag-and-drop.
 */
export function TasksView({ lockedProjectId }: { lockedProjectId?: string }) {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();

  const q = params.get("q") ?? "";
  const projectFilter = lockedProjectId ?? params.get("project") ?? "";
  const agentFilter = params.get("agent") ?? "";
  const statusFilter = params.get("status") ?? "";
  const priorityFilter = params.get("priority") ?? "";
  const failedOnly = params.get("failed") === "1";
  const group: GroupBy = (["agent", "project"] as const).includes(params.get("group") as "agent")
    ? (params.get("group") as GroupBy)
    : "status";
  const taskParam = params.get("task");
  const runParam = params.get("run");

  // Board or List, remembered per browser. An old ?view=tasks link means List.
  const [savedLayout, setSavedLayout] = useLocalState<Layout>("jobshout-tasks-layout", "board");
  const layout: Layout = params.get("view") === "tasks" ? "list" : savedLayout;

  const { data: tasksResp, isLoading } = useAllTasks();
  const { data: agentsResp } = useAgents({ per_page: 100 });
  const { data: projectsResp } = useProjects({ per_page: 100 });
  const { failedTaskIds } = useRunFailures();
  const { data: deepTask } = useTask(taskParam ?? "");

  const agents = useMemo(() => agentsResp?.data ?? [], [agentsResp]);
  const projects = useMemo(() => projectsResp?.data ?? [], [projectsResp]);
  const agentName = useMemo(() => new Map(agents.map((a) => [a.id, a.name])), [agents]);
  const projectName = useMemo(() => new Map(projects.map((p) => [p.id, p.name])), [projects]);

  const allTasks = useMemo(() => tasksResp?.data ?? [], [tasksResp]);
  const tasks = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return allTasks.filter((t) => {
      if (projectFilter && t.project_id !== projectFilter) return false;
      if (agentFilter === "none" ? t.assigned_agent_id : agentFilter && t.assigned_agent_id !== agentFilter) return false;
      if (statusFilter && t.status !== statusFilter) return false;
      if (priorityFilter && t.priority !== priorityFilter) return false;
      if (failedOnly && !failedTaskIds.has(t.id)) return false;
      if (needle && !`${t.title} ${t.description ?? ""}`.toLowerCase().includes(needle)) return false;
      return true;
    });
  }, [allTasks, q, projectFilter, agentFilter, statusFilter, priorityFilter, failedOnly, failedTaskIds]);

  const [selected, setSelected] = useState<Task | null>(null);
  useEffect(() => {
    if (!taskParam) return setSelected(null);
    const t = allTasks.find((x) => x.id === taskParam) ?? deepTask ?? null;
    setSelected((prev) => t ?? (prev?.id === taskParam ? prev : null));
  }, [taskParam, allTasks, deepTask]);

  function setParams(next: Partial<Record<(typeof KEYS)[number] | "project" | "view", string | null>>) {
    const p = new URLSearchParams(params.toString());
    for (const [k, v] of Object.entries(next)) {
      if (v) p.set(k, v);
      else p.delete(k);
    }
    const qs = p.toString();
    router.replace(`${pathname}${qs ? `?${qs}` : ""}`, { scroll: false });
  }

  const filtered = Boolean(q || agentFilter || statusFilter || priorityFilter || failedOnly || (!lockedProjectId && projectFilter));
  const clearFilters = () =>
    setParams({ q: null, agent: null, status: null, priority: null, failed: null, ...(lockedProjectId ? {} : { project: null }) });

  const columns: Column[] = useMemo(() => {
    if (group === "agent") {
      const ids = Array.from(new Set(tasks.map((t) => t.assigned_agent_id ?? "")));
      return ids
        .map((id) => ({
          key: id || "none",
          label: id ? agentName.get(id) ?? "Unknown agent" : "Unassigned",
          href: id ? agentHref(id) : undefined,
          tasks: tasks.filter((t) => (t.assigned_agent_id ?? "") === id),
        }))
        .sort((a, b) => (a.key === "none" ? 1 : b.key === "none" ? -1 : a.label.localeCompare(b.label)));
    }
    if (group === "project") {
      const ids = Array.from(new Set(tasks.map((t) => t.project_id)));
      return ids
        .map((id) => ({
          key: id,
          label: projectName.get(id) ?? "Project",
          href: `/panel/projects?project=${id}`,
          tasks: tasks.filter((t) => t.project_id === id),
        }))
        .sort((a, b) => a.label.localeCompare(b.label));
    }
    return STATUS_OPTIONS.map((s) => ({
      key: s.value,
      label: s.label,
      dot: STATUS_DOT[s.value],
      tasks: tasks.filter((t) => t.status === s.value),
    }));
  }, [tasks, group, agentName, projectName]);

  // Drag-and-drop needs one project's board with nothing hidden from it.
  const dragBoard =
    layout === "board" && group === "status" && Boolean(projectFilter) &&
    !q && !agentFilter && !statusFilter && !priorityFilter && !failedOnly;

  const openTask = (t: Task) => {
    setSelected(t);
    setParams({ task: t.id, run: null });
  };
  const closeTask = () => setParams({ task: null, run: null });

  // New task: straight into the dialog when the project is known, else ask.
  const wantsNew = params.get("new") === "1";
  const [pickedProject, setPickedProject] = useState("");
  const newTaskProject = projectFilter || pickedProject;

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2 border-b border-border px-6 py-3">
        <label className="relative min-w-[12rem] flex-1 sm:max-w-xs">
          <span className="sr-only">Search tasks</span>
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="search"
            value={q}
            onChange={(e) => setParams({ q: e.target.value || null })}
            placeholder="Search tasks"
            className={cn(selectCls, "w-full pl-8")}
          />
        </label>
        {!lockedProjectId && (
          <select aria-label="Project" value={projectFilter} onChange={(e) => setParams({ project: e.target.value || null })} className={selectCls}>
            <option value="">All projects</option>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>{p.name}</option>
            ))}
          </select>
        )}
        <select aria-label="Agent" value={agentFilter} onChange={(e) => setParams({ agent: e.target.value || null })} className={selectCls}>
          <option value="">All agents</option>
          <option value="none">Unassigned</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>{a.name}</option>
          ))}
        </select>
        <select aria-label="Status" value={statusFilter} onChange={(e) => setParams({ status: e.target.value || null })} className={selectCls}>
          <option value="">Any status</option>
          {STATUS_OPTIONS.map((s) => (
            <option key={s.value} value={s.value}>{s.label}</option>
          ))}
        </select>
        <select aria-label="Priority" value={priorityFilter} onChange={(e) => setParams({ priority: e.target.value || null })} className={selectCls}>
          <option value="">Any priority</option>
          {PRIORITY_OPTIONS.map((p) => (
            <option key={p.value} value={p.value}>{p.label}</option>
          ))}
        </select>
        <label className="inline-flex h-9 cursor-pointer items-center gap-1.5 rounded-md border border-input px-2.5 text-sm">
          <input type="checkbox" checked={failedOnly} onChange={(e) => setParams({ failed: e.target.checked ? "1" : null })} />
          <AlertTriangle className="h-3.5 w-3.5 text-destructive" />
          Has failures
        </label>
        {filtered && (
          <button type="button" onClick={clearFilters} className="inline-flex h-9 items-center gap-1 rounded-md px-2 text-sm text-muted-foreground hover:text-foreground">
            <X className="h-3.5 w-3.5" /> Clear
          </button>
        )}

        <div className="ml-auto flex items-center gap-2">
          <select aria-label="Group by" value={group} onChange={(e) => setParams({ group: e.target.value === "status" ? null : e.target.value })} className={selectCls}>
            <option value="status">Group: status</option>
            <option value="agent">Group: agent</option>
            {!lockedProjectId && <option value="project">Group: project</option>}
          </select>
          <div className="flex rounded-md border border-border p-0.5" role="group" aria-label="Layout">
            {([
              { id: "board", label: "Board", icon: LayoutGrid },
              { id: "list", label: "List", icon: List },
            ] as const).map(({ id, label, icon: Icon }) => (
              <button
                key={id}
                type="button"
                aria-pressed={layout === id}
                onClick={() => {
                  setSavedLayout(id);
                  setParams({ view: null });
                }}
                className={cn(
                  "inline-flex items-center gap-1.5 rounded px-2.5 py-1.5 text-sm font-medium",
                  layout === id ? "bg-secondary text-foreground" : "text-muted-foreground hover:text-foreground"
                )}
              >
                <Icon className="h-4 w-4" /> {label}
              </button>
            ))}
          </div>
          <button
            type="button"
            onClick={() => setParams({ new: "1" })}
            className="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-3 text-sm font-semibold text-primary-foreground hover:opacity-90"
          >
            <Plus className="h-4 w-4" /> New task
          </button>
        </div>
      </div>

      {tasksResp && (
        <p className="px-6 pt-2 text-xs text-muted-foreground">
          {filtered ? `${tasks.length} matching · ` : ""}
          <TaskCountLabel loaded={allTasks.length} total={tasksResp.total} />
        </p>
      )}

      {/* Body */}
      <div className={cn("min-h-0 flex-1", dragBoard ? "overflow-hidden" : "overflow-auto p-4 scrollbar-thin")}>
        {isLoading ? (
          <div className="flex h-40 items-center justify-center text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Loading tasks…
          </div>
        ) : dragBoard ? (
          <KanbanBoard projectId={projectFilter} projectName={projectName.get(projectFilter)} onOpenTask={openTask} />
        ) : tasks.length === 0 ? (
          <div className="mx-auto mt-10 max-w-md rounded-lg border border-dashed border-border p-8 text-center">
            <p className="text-sm text-muted-foreground">
              {filtered ? "No tasks match these filters." : "No tasks yet."}
            </p>
            <button
              type="button"
              onClick={() => (filtered ? clearFilters() : setParams({ new: "1" }))}
              className="mt-3 inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-2 text-sm font-semibold text-primary-foreground hover:opacity-90"
            >
              {filtered ? "Clear filters" : (<><Plus className="h-4 w-4" /> Create a task</>)}
            </button>
          </div>
        ) : layout === "board" ? (
          <div className="flex h-full min-w-max gap-3">
            {columns.map((col) => (
              <section key={col.key} aria-label={col.label} className="flex w-72 shrink-0 flex-col rounded-lg border border-border bg-muted/30">
                <header className="flex items-center gap-2 border-b border-border px-3 py-2.5">
                  {col.dot && <span className={cn("h-2 w-2 rounded-full", col.dot)} />}
                  {col.href ? (
                    <Link href={col.href} className="truncate text-sm font-medium hover:underline">{col.label}</Link>
                  ) : (
                    <span className="truncate text-sm font-medium">{col.label}</span>
                  )}
                  <span className="ml-auto font-mono text-[11px] text-muted-foreground">{col.tasks.length}</span>
                </header>
                <ul className="flex-1 space-y-2 overflow-y-auto p-2 scrollbar-thin">
                  {col.tasks.length === 0 ? (
                    <li className="px-2 py-6 text-center text-xs text-muted-foreground">Empty</li>
                  ) : (
                    col.tasks.map((t) => (
                      <li key={t.id} className={cn(failedTaskIds.has(t.id) && "rounded-lg ring-1 ring-destructive/50")}>
                        <TaskCardFace
                          task={t}
                          assigneeName={t.assigned_agent_id ? agentName.get(t.assigned_agent_id) : undefined}
                          onOpenDetail={openTask}
                        />
                      </li>
                    ))
                  )}
                </ul>
              </section>
            ))}
          </div>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-border bg-card">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-border bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th scope="col" className="px-4 py-2.5 font-semibold">Task</th>
                  {!lockedProjectId && <th scope="col" className="px-4 py-2.5 font-semibold">Project</th>}
                  <th scope="col" className="px-4 py-2.5 font-semibold">Agent</th>
                  <th scope="col" className="px-4 py-2.5 font-semibold">Status</th>
                  <th scope="col" className="px-4 py-2.5 font-semibold">Priority</th>
                  <th scope="col" className="px-4 py-2.5 font-semibold">Last run</th>
                </tr>
              </thead>
              {columns.map((col) =>
                col.tasks.length === 0 ? null : (
                  <tbody key={col.key} className="divide-y divide-border">
                    <tr className="bg-muted/20">
                      <th scope="rowgroup" colSpan={6} className="px-4 py-1.5 text-xs font-semibold text-muted-foreground">
                        {col.label} · {col.tasks.length}
                      </th>
                    </tr>
                    {col.tasks.map((t) => (
                      <tr key={t.id} onClick={() => openTask(t)} className="cursor-pointer hover:bg-muted/40">
                        <td className="max-w-[22rem] px-4 py-2.5">
                          <button
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              openTask(t);
                            }}
                            className="truncate text-left font-medium text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          >
                            {t.title}
                          </button>
                        </td>
                        {!lockedProjectId && (
                          <td className="px-4 py-2.5 text-muted-foreground">{projectName.get(t.project_id) ?? "—"}</td>
                        )}
                        <td className="px-4 py-2.5">
                          {t.assigned_agent_id ? (
                            <Link
                              href={agentHref(t.assigned_agent_id)}
                              onClick={(e) => e.stopPropagation()}
                              className="text-foreground hover:underline"
                            >
                              {agentName.get(t.assigned_agent_id) ?? "Agent"}
                            </Link>
                          ) : (
                            <span className="text-muted-foreground">Unassigned</span>
                          )}
                        </td>
                        <td className="px-4 py-2.5">
                          <span className="inline-flex items-center gap-1.5">
                            <span className={cn("h-2 w-2 rounded-full", STATUS_DOT[t.status])} />
                            {statusLabel(t.status)}
                          </span>
                        </td>
                        <td className="px-4 py-2.5 text-muted-foreground">{priorityLabel(t.priority)}</td>
                        <td className="px-4 py-2.5">
                          {failedTaskIds.has(t.id) ? (
                            <span className="inline-flex items-center gap-1 text-xs font-medium text-destructive">
                              <AlertTriangle className="h-3.5 w-3.5" /> Failed
                            </span>
                          ) : (
                            <TaskProgressChip task={t} />
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                )
              )}
            </table>
          </div>
        )}
      </div>

      {wantsNew && !newTaskProject && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" role="dialog" aria-modal="true" aria-label="Choose a project">
          <div className="w-full max-w-sm rounded-lg border border-border bg-card p-5 shadow-card-hover">
            <h2 className="text-base font-semibold">Which project is this task for?</h2>
            {projects.length === 0 ? (
              <p className="mt-2 text-sm text-muted-foreground">
                Tasks live in a project.{" "}
                <Link href="/panel/projects" className="font-medium text-primary hover:underline">Create a project first</Link>.
              </p>
            ) : (
              <select autoFocus aria-label="Project" defaultValue="" onChange={(e) => setPickedProject(e.target.value)} className={cn(selectCls, "mt-3 w-full")}>
                <option value="" disabled>Choose a project…</option>
                {projects.map((p) => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </select>
            )}
            <div className="mt-4 flex justify-end">
              <button type="button" onClick={() => setParams({ new: null })} className="rounded-md px-3 py-1.5 text-sm text-muted-foreground hover:text-foreground">
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}
      {wantsNew && newTaskProject && (
        <CreateTaskDialog
          projectId={newTaskProject}
          onClose={() => {
            setPickedProject("");
            setParams({ new: null });
          }}
          onCreated={(t) => {
            setPickedProject("");
            setParams({ new: null, task: t.id });
          }}
        />
      )}
      {selected && (
        <TaskDetailModal task={selected} onClose={closeTask} onUpdated={(t) => setSelected(t)} initialFocusRunId={runParam} />
      )}
    </div>
  );
}
