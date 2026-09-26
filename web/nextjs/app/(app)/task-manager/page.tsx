"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import {
  Bot,
  ListTree,
  Pencil,
  Plus,
  Trash2,
} from "lucide-react";

import { TaskEditorDialog } from "@/components/task-manager/TaskEditorDialog";
import { RunTaskDialog } from "@/components/task-manager/RunTaskDialog";
import { TaskRunView } from "@/components/task-manager/TaskRunView";
import { TaskActions } from "@/components/task-manager/TaskActions";
import { TaskHistoryPanel } from "@/components/task-manager/TaskHistoryPanel";
import {
  TaskProgressChip,
  TaskCountLabel,
  formatCompletedAt,
} from "@/components/task-manager/TaskProgressChip";
import { statusLabel } from "@/lib/task-labels";
import { useAgents } from "@/lib/hooks/useAgents";
import { useProjects } from "@/lib/hooks/useProjects";
import {
  useDeleteTask,
  useProjectTasks,
  useTransitionTask,
} from "@/lib/hooks/useTasks";
import type { Agent } from "@/lib/types/agent";
import type { TaskStatus } from "@/lib/types/common";
import type { Task } from "@/lib/types/project";
import type { TaskRun } from "@/lib/types/task-run";

const STATUSES: TaskStatus[] = [
  "backlog",
  "todo",
  "in_progress",
  "review",
  "done",
];

const STATUS_DOT: Record<TaskStatus, string> = {
  backlog: "bg-muted-foreground",
  todo: "bg-signal-info",
  in_progress: "bg-signal-live",
  review: "bg-signal-warn",
  done: "bg-signal",
};

const PRIORITY_CLS: Record<string, string> = {
  low: "text-muted-foreground",
  medium: "text-signal-info",
  high: "text-signal-warn",
  critical: "text-signal-error",
};

export default function TaskManagerPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const projectParam = searchParams.get("project");
  const taskParam = searchParams.get("task");
  const runParam = searchParams.get("run");

  const { data: projectsResp } = useProjects({ per_page: 100 });
  const projects = useMemo(() => projectsResp?.data ?? [], [projectsResp]);

  const { data: agentsResp } = useAgents({ per_page: 100 });
  const agents = useMemo(() => agentsResp?.data ?? [], [agentsResp]);

  const [projectId, setProjectId] = useState<string>(projectParam ?? "");
  useEffect(() => {
    if (projectParam) {
      setProjectId(projectParam);
      return;
    }
    if (!projectId && projects.length > 0) setProjectId(projects[0].id);
  }, [projects, projectId, projectParam]);

  const { data: tasksResp, isLoading: tasksLoading } = useProjectTasks(projectId);
  const tasks = useMemo(() => tasksResp?.data ?? [], [tasksResp]);

  const [statusFilter, setStatusFilter] = useState<TaskStatus | "all">("all");
  const filtered = useMemo(
    () =>
      statusFilter === "all"
        ? tasks
        : tasks.filter((t) => t.status === statusFilter),
    [tasks, statusFilter]
  );

  const [selectedId, setSelectedId] = useState<string | null>(taskParam);
  const selected = tasks.find((t) => t.id === selectedId) ?? null;
  const [editorOpen, setEditorOpen] = useState(false);
  const [editorTask, setEditorTask] = useState<Task | undefined>(undefined);
  const [runOpen, setRunOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [focusRunId, setFocusRunId] = useState<string | null>(runParam);

  useEffect(() => {
    setSelectedId(taskParam);
    setFocusRunId(runParam);
  }, [projectId, taskParam, runParam]);
  useEffect(() => {
    // Keep a valid selection as the list changes.
    if (selectedId && !tasks.some((t) => t.id === selectedId)) {
      setSelectedId(null);
    }
  }, [tasks, selectedId]);

  function writeTaskUrl(nextProject: string, taskId: string | null, runId?: string | null) {
    const params = new URLSearchParams();
    if (nextProject) params.set("project", nextProject);
    if (taskId) params.set("task", taskId);
    if (runId) params.set("run", runId);
    const qs = params.toString();
    router.replace(`/task-manager${qs ? `?${qs}` : ""}`, { scroll: false });
  }

  const transition = useTransitionTask();
  const deleteTask = useDeleteTask();

  function openCreate() {
    setEditorTask(undefined);
    setEditorOpen(true);
  }
  function openEdit(task: Task) {
    setEditorTask(task);
    setEditorOpen(true);
  }

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 font-display text-2xl font-semibold">
            <ListTree className="h-6 w-6 text-primary" />
            Task Manager
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Write a task, assign an agent, and run it — with per-run skills,
            model, inputs and debug overrides.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <select
            value={projectId}
            onChange={(e) => {
              setProjectId(e.target.value);
              setSelectedId(null);
              writeTaskUrl(e.target.value, null);
            }}
            className="h-9 rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {projects.length === 0 && <option value="">No projects</option>}
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <button
            onClick={openCreate}
            disabled={!projectId}
            className="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
          >
            <Plus className="h-4 w-4" /> New Task
          </button>
        </div>
      </div>

      {projects.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border bg-card/50 p-8 text-center text-sm text-muted-foreground">
          No projects yet.{" "}
          <Link href="/projects" className="text-primary hover:underline">
            Create a project
          </Link>{" "}
          to start adding tasks.
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-[320px_1fr]">
          {/* Task list */}
          <div className="space-y-3">
            <div className="flex items-center gap-2">
              <select
                value={statusFilter}
                onChange={(e) =>
                  setStatusFilter(e.target.value as TaskStatus | "all")
                }
                className="h-8 rounded-md border border-input bg-background px-2 text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <option value="all">All statuses</option>
                {STATUSES.map((s) => (
                  <option key={s} value={s}>
                    {s.replace("_", " ")}
                  </option>
                ))}
              </select>
              <span className="text-xs text-muted-foreground">
                <TaskCountLabel loaded={filtered.length} total={tasksResp?.total} />
              </span>
            </div>

            {tasksLoading ? (
              <div className="rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground">
                Loading tasks…
              </div>
            ) : filtered.length === 0 ? (
              <div className="rounded-lg border border-dashed border-border bg-card/50 p-6 text-center text-sm text-muted-foreground">
                No tasks here yet.
              </div>
            ) : (
              <ul className="space-y-1.5">
                {filtered.map((task) => {
                  const active = task.id === selectedId;
                  const agent = agents.find(
                    (a) => a.id === task.assigned_agent_id
                  );
                  return (
                    <li
                      key={task.id}
                      className={
                        "rounded-md border px-3 py-2.5 " +
                        (active
                          ? "border-primary bg-primary/10"
                          : "border-border bg-card")
                      }
                    >
                      <button
                        type="button"
                        onClick={() => {
                          setSelectedId(task.id);
                          setFocusRunId(null);
                          writeTaskUrl(projectId, task.id);
                        }}
                        className="w-full text-left"
                      >
                        <div className="flex items-center gap-2">
                          <span
                            className={
                              "h-2 w-2 shrink-0 rounded-full " +
                              STATUS_DOT[task.status]
                            }
                          />
                          <span className="min-w-0 flex-1 truncate text-sm font-medium">
                            {task.title}
                          </span>
                          <span
                            className={
                              "shrink-0 text-xs font-medium " +
                              (PRIORITY_CLS[task.priority] ?? "")
                            }
                          >
                            {task.priority}
                          </span>
                        </div>
                        <div className="mt-1 flex flex-wrap items-center gap-2 pl-4 text-xs text-muted-foreground">
                          {agent ? (
                            <span className="inline-flex items-center gap-1">
                              <Bot className="h-3 w-3" /> {agent.name}
                            </span>
                          ) : (
                            <span className="italic">unassigned</span>
                          )}
                          <span>
                            {formatCompletedAt(task.completed_at)
                              ? `Completed ${formatCompletedAt(task.completed_at)}`
                              : `Updated ${new Date(task.updated_at).toLocaleString()}`}
                          </span>
                        </div>
                        <div className="mt-1.5 pl-4">
                          <TaskProgressChip task={task} />
                        </div>
                      </button>
                      <div className="mt-2 pl-4">
                        <TaskActions
                          task={task}
                          size="sm"
                          onShowHistory={() => {
                            setSelectedId(task.id);
                            setHistoryOpen(true);
                            writeTaskUrl(projectId, task.id, runParam);
                          }}
                        />
                      </div>
                    </li>
                  );
                })}
              </ul>
            )}
          </div>

          {/* Detail */}
          <div>
            {selected ? (
              <TaskDetail
                task={selected}
                agents={agents}
                focusRunId={focusRunId}
                onEdit={() => openEdit(selected)}
                onRun={() => setRunOpen(true)}
                onShowHistory={() => setHistoryOpen(true)}
                onDelete={async () => {
                  if (
                    confirm(`Delete "${selected.title}"? This cannot be undone.`)
                  ) {
                    await deleteTask.mutateAsync(selected.id);
                    setSelectedId(null);
                  }
                }}
                onTransition={(status) =>
                  transition.mutate({ id: selected.id, payload: { status } })
                }
              />
            ) : (
              <div className="flex h-full min-h-[240px] items-center justify-center rounded-lg border border-dashed border-border bg-card/50 text-sm text-muted-foreground">
                Select a task to edit and run it.
              </div>
            )}
          </div>
        </div>
      )}

      {/* Dialogs */}
      {editorOpen && (
        <TaskEditorDialog
          task={editorTask}
          projectId={projectId}
          agents={agents}
          onClose={() => setEditorOpen(false)}
          onSaved={(t) => setSelectedId(t.id)}
        />
      )}
      {runOpen && selected && (
        <RunTaskDialog
          task={selected}
          agents={agents}
          onClose={() => setRunOpen(false)}
          onLaunched={(run: TaskRun) => {
            setFocusRunId(run.id);
            setHistoryOpen(true);
          }}
        />
      )}
      {historyOpen && selected && (
        <TaskHistoryPanel
          task={selected}
          agents={agents}
          focusRunId={focusRunId}
          onClose={() => setHistoryOpen(false)}
          onReRun={() => {
            setHistoryOpen(false);
            setRunOpen(true);
          }}
        />
      )}
    </div>
  );
}

function TaskDetail({
  task,
  agents,
  focusRunId,
  onEdit,
  onRun,
  onShowHistory,
  onDelete,
  onTransition,
}: {
  task: Task;
  agents: Agent[];
  focusRunId: string | null;
  onEdit: () => void;
  onRun: () => void;
  onShowHistory: () => void;
  onDelete: () => void;
  onTransition: (status: TaskStatus) => void;
}) {
  const agent = agents.find((a) => a.id === task.assigned_agent_id);

  return (
    <div className="space-y-5 rounded-lg border border-border bg-card p-5">
      {/* Title + actions */}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="font-display text-xl font-semibold">{task.title}</h2>
          {task.description && (
            <p className="mt-1 whitespace-pre-wrap text-sm text-muted-foreground">
              {task.description}
            </p>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <button
            onClick={onEdit}
            className="inline-flex h-9 items-center gap-1.5 rounded-md border border-border bg-background px-3 text-sm hover:bg-accent"
          >
            <Pencil className="h-4 w-4" /> Edit
          </button>
          <button
            onClick={onDelete}
            className="inline-flex h-9 w-9 items-center justify-center rounded-md border border-border text-muted-foreground hover:bg-accent hover:text-signal-error"
            aria-label="Delete task"
          >
            <Trash2 className="h-4 w-4" />
          </button>
          <TaskActions
            task={task}
            onShowHistory={onShowHistory}
            onRun={onRun}
          />
        </div>
      </div>

      {/* Meta row */}
      <div className="flex flex-wrap items-center gap-4 text-sm">
        <label className="flex items-center gap-1.5">
          <span className="text-muted-foreground">Status</span>
          <select
            value={task.status}
            onChange={(e) => onTransition(e.target.value as TaskStatus)}
            className="h-8 rounded-md border border-input bg-background px-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {statusLabel(s)}
              </option>
            ))}
          </select>
        </label>
        <span className="text-muted-foreground">
          Priority{" "}
          <span
            className={"font-medium " + (PRIORITY_CLS[task.priority] ?? "")}
          >
            {task.priority}
          </span>
        </span>
        <span className="inline-flex items-center gap-1.5 text-muted-foreground">
          <Bot className="h-4 w-4" />
          {agent ? (
            <span className="font-medium text-foreground">{agent.name}</span>
          ) : (
            <span className="italic">unassigned</span>
          )}
        </span>
        {task.due_date && (
          <span className="text-muted-foreground">
            Due {new Date(task.due_date).toLocaleDateString()}
          </span>
        )}
      </div>

      {/* Runs */}
      <div className="space-y-2 border-t border-border pt-4">
        <h3 className="text-sm font-medium">Runs</h3>
        <TaskRunView task={task} agents={agents} focusRunId={focusRunId} />
      </div>
    </div>
  );
}
