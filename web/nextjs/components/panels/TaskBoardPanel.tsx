"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useAgents } from "@/lib/hooks/useAgents";
import { useProjects } from "@/lib/hooks/useProjects";
import { useTasks } from "@/lib/hooks/useTasks";
import { TaskDetailModal } from "@/components/kanban/TaskDetailModal";
import { KanbanBoard } from "@/components/kanban/KanbanBoard";
import AgentBoardPage from "@/app/(app)/agent-board/page";
import type { Task } from "@/lib/types/project";
import type { TaskStatus } from "@/lib/types/common";
import { cn } from "@/lib/utils/cn";
import { STATUS_DOT } from "@/lib/status-colors";

const COLUMNS: { status: TaskStatus; label: string; dot: string }[] = [
  { status: "backlog", label: "Backlog", dot: STATUS_DOT.backlog },
  { status: "todo", label: "To Do", dot: STATUS_DOT.todo },
  { status: "in_progress", label: "In Progress", dot: STATUS_DOT.in_progress },
  { status: "review", label: "Review", dot: STATUS_DOT.review },
  { status: "done", label: "Done", dot: STATUS_DOT.done },
];

type View = "tasks" | "agents";

export function TaskBoardPanel() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const taskParam = searchParams.get("task");
  const projectParam = searchParams.get("project");
  const viewParam = searchParams.get("view");

  const view: View = viewParam === "agents" ? "agents" : "tasks";
  const [projectFilter, setProjectFilter] = useState<string>(projectParam ?? "");

  const { data: tasksResp, isLoading } = useTasks({ per_page: 200 });
  const { data: agentsResp } = useAgents({ per_page: 100 });
  const { data: projectsResp } = useProjects({ per_page: 100 });

  const tasks = useMemo(() => tasksResp?.data ?? [], [tasksResp]);
  const projects = useMemo(() => projectsResp?.data ?? [], [projectsResp]);
  const agentNames = useMemo(() => {
    const m = new Map<string, string>();
    for (const a of agentsResp?.data ?? []) m.set(a.id, a.name);
    return m;
  }, [agentsResp]);
  const projectNames = useMemo(() => {
    const m = new Map<string, string>();
    for (const p of projects) m.set(p.id, p.name);
    return m;
  }, [projects]);

  const [selected, setSelected] = useState<Task | null>(null);

  useEffect(() => {
    setProjectFilter(projectParam ?? "");
  }, [projectParam]);

  useEffect(() => {
    if (!taskParam) return;
    const t = tasks.find((x) => x.id === taskParam);
    if (t) setSelected(t);
  }, [taskParam, tasks]);

  const byStatus = useMemo(() => {
    const map: Record<TaskStatus, Task[]> = {
      backlog: [],
      todo: [],
      in_progress: [],
      review: [],
      done: [],
    };
    for (const t of tasks) {
      if (map[t.status]) map[t.status].push(t);
    }
    return map;
  }, [tasks]);

  function setUrl(params: URLSearchParams) {
    const qs = params.toString();
    router.replace(`/panel/task-board${qs ? `?${qs}` : ""}`, { scroll: false });
  }

  function switchView(next: View) {
    const params = new URLSearchParams();
    if (next === "agents") params.set("view", "agents");
    else if (projectFilter) params.set("project", projectFilter);
    setUrl(params);
  }

  function changeProject(id: string) {
    setProjectFilter(id);
    const params = new URLSearchParams();
    if (id) params.set("project", id);
    setUrl(params);
  }

  function openTask(task: Task) {
    setSelected(task);
    const params = new URLSearchParams(searchParams.toString());
    params.set("task", task.id);
    setUrl(params);
  }

  function closeTask() {
    setSelected(null);
    const params = new URLSearchParams(searchParams.toString());
    params.delete("task");
    setUrl(params);
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-6 py-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Task Board</h1>
          <p className="text-sm text-muted-foreground">
            {view === "agents"
              ? "Live agent activity"
              : projectFilter
                ? `Board for ${projectNames.get(projectFilter) ?? "project"} — drag to move tasks`
                : "All tasks across projects"}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {view === "tasks" && (
            <select
              value={projectFilter}
              onChange={(e) => changeProject(e.target.value)}
              className="h-9 rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label="Filter by project"
            >
              <option value="">All projects</option>
              {projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          )}
          <div className="flex rounded-md border border-border p-0.5">
            {(
              [
                { id: "tasks", label: "Tasks" },
                { id: "agents", label: "Agents" },
              ] as const
            ).map((t) => (
              <button
                key={t.id}
                type="button"
                onClick={() => switchView(t.id)}
                className={cn(
                  "rounded px-3 py-1.5 text-sm font-medium transition-colors",
                  view === t.id
                    ? "bg-secondary text-foreground"
                    : "text-muted-foreground hover:text-foreground"
                )}
              >
                {t.label}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-auto scrollbar-thin p-4">
        {view === "agents" ? (
          <AgentBoardPage hideHeader />
        ) : projectFilter ? (
          // Single project: full drag-and-drop board with inline task creation.
          <div className="h-full min-h-[420px]">
            <KanbanBoard projectId={projectFilter} />
          </div>
        ) : isLoading ? (
          <div className="flex h-40 items-center justify-center text-sm text-muted-foreground">
            Loading tasks…
          </div>
        ) : (
          <div className="flex h-full min-w-max gap-3">
            {COLUMNS.map((col) => {
              const items = byStatus[col.status] ?? [];
              return (
                <div
                  key={col.status}
                  className="flex w-72 shrink-0 flex-col rounded-lg border border-border bg-muted/30"
                >
                  <div className="flex items-center gap-2 border-b border-border px-3 py-2.5">
                    <span className={cn("h-2 w-2 rounded-full", col.dot)} />
                    <span className="text-sm font-medium">{col.label}</span>
                    <span className="ml-auto font-mono text-[11px] text-muted-foreground">
                      {items.length}
                    </span>
                  </div>
                  <ul className="flex-1 space-y-2 overflow-y-auto scrollbar-thin p-2">
                    {items.length === 0 ? (
                      <li className="px-2 py-6 text-center text-xs text-muted-foreground">
                        Empty
                      </li>
                    ) : (
                      items.map((task) => (
                        <li key={task.id}>
                          <button
                            type="button"
                            onClick={() => openTask(task)}
                            className={cn(
                              "w-full rounded-md border border-border bg-card px-3 py-2.5 text-left transition-colors hover:border-primary/40",
                              selected?.id === task.id &&
                                "border-primary/50 bg-accent/40"
                            )}
                          >
                            <p className="truncate text-sm font-medium">
                              {task.title}
                            </p>
                            <p className="mt-1 truncate text-[11px] text-muted-foreground">
                              {task.assigned_agent_id
                                ? agentNames.get(task.assigned_agent_id) ?? "Agent"
                                : "Unassigned"}
                              {" · "}
                              {projectNames.get(task.project_id) ?? "Project"}
                            </p>
                          </button>
                        </li>
                      ))
                    )}
                  </ul>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {selected && (
        <TaskDetailModal
          task={selected}
          onClose={closeTask}
          onUpdated={(t) => setSelected(t)}
        />
      )}
    </div>
  );
}
