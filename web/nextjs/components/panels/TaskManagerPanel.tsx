"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import {
  Bot,
  FolderKanban,
  Newspaper,
  Image as ImageIcon,
  Plus,
  ShieldAlert,
  GitPullRequest,
  Mail,
  Rocket,
  BookOpen,
} from "lucide-react";
import Link from "next/link";
import { useAgents } from "@/lib/hooks/useAgents";
import { useProjects } from "@/lib/hooks/useProjects";
import {
  useDeleteTask,
  useProjectTasks,
  useTasks,
  useTransitionTask,
} from "@/lib/hooks/useTasks";
import { STATUS_OPTIONS, statusLabel } from "@/lib/task-labels";
import { TaskEditorDialog } from "@/components/task-manager/TaskEditorDialog";
import { RunTaskDialog } from "@/components/task-manager/RunTaskDialog";
import { TaskRunView } from "@/components/task-manager/TaskRunView";
import { NewProjectDialog } from "@/components/task-manager/NewProjectDialog";
import { CreateAgentDialog } from "@/components/agent/CreateAgentDialog";
import { AgentStatusBadge } from "@/components/agent/AgentStatusBadge";
import { PentestAgentClient } from "@/components/PentestAgentClient";
import { ReviewAgentClient } from "@/components/ReviewAgentClient";
import { MailAgentClient } from "@/components/MailAgentClient";
import ArticlesPage from "@/app/(app)/articles/page";
import ImagesPage from "@/app/(app)/images/page";
import type { LaunchResult } from "@/lib/agents/launch";
import type { Agent } from "@/lib/types/agent";
import type { Project, Task } from "@/lib/types/project";
import type { TaskRun } from "@/lib/types/task-run";
import type { TaskStatus } from "@/lib/types/common";
import { cn } from "@/lib/utils/cn";

type Selection =
  | { kind: "project"; id: string }
  | { kind: "agent"; id: string }
  | { kind: "builtin"; id: "pentest" | "review" | "mail" | "articles" | "images" };

const BUILTINS: {
  id: "pentest" | "review" | "mail" | "articles" | "images";
  label: string;
  icon: React.ElementType;
  match?: string;
}[] = [
  { id: "pentest", label: "Security Tester", icon: ShieldAlert, match: "pentester" },
  { id: "review", label: "PR Reviewer", icon: GitPullRequest, match: "pr_reviewer" },
  { id: "mail", label: "Mail Agent", icon: Mail, match: "mail" },
  { id: "articles", label: "Articles", icon: Newspaper, match: "article_writer" },
  { id: "images", label: "Images", icon: ImageIcon },
];

const STATUS_DOT: Record<TaskStatus, string> = {
  backlog: "bg-muted-foreground",
  todo: "bg-signal-info",
  in_progress: "bg-signal-live",
  review: "bg-signal-warn",
  done: "bg-signal",
};

function parseSelection(
  project: string | null,
  agent: string | null
): Selection | null {
  if (agent === "pentest" || agent === "review" || agent === "mail" || agent === "articles" || agent === "images") {
    return { kind: "builtin", id: agent };
  }
  if (agent) return { kind: "agent", id: agent };
  if (project) return { kind: "project", id: project };
  return null;
}

export function TaskManagerPanel() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const projectParam = searchParams.get("project");
  const agentParam = searchParams.get("agent");

  const { data: projectsResp } = useProjects({ per_page: 100 });
  const projects = useMemo(() => projectsResp?.data ?? [], [projectsResp]);
  const { data: agentsResp } = useAgents({ per_page: 100 });
  const agents = useMemo(() => agentsResp?.data ?? [], [agentsResp]);
  const { data: allTasksResp } = useTasks({ per_page: 200 });

  const taskCounts = useMemo(() => {
    const m = new Map<string, number>();
    for (const t of allTasksResp?.data ?? []) {
      m.set(t.project_id, (m.get(t.project_id) ?? 0) + 1);
    }
    return m;
  }, [allTasksResp]);

  const [selection, setSelection] = useState<Selection | null>(() =>
    parseSelection(projectParam, agentParam)
  );
  const [createAgentOpen, setCreateAgentOpen] = useState(false);
  const [createProjectOpen, setCreateProjectOpen] = useState(false);

  useEffect(() => {
    const parsed = parseSelection(projectParam, agentParam);
    if (parsed) {
      setSelection(parsed);
      return;
    }
    if (!selection && projects[0]) {
      select({ kind: "project", id: projects[0].id });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectParam, agentParam, projects]);

  function select(next: Selection) {
    setSelection(next);
    const params = new URLSearchParams();
    if (next.kind === "project") params.set("project", next.id);
    else if (next.kind === "builtin") params.set("agent", next.id);
    else params.set("agent", next.id);
    router.replace(`/panel/task-manager?${params.toString()}`, { scroll: false });
  }

  const selectedAgent =
    selection?.kind === "agent"
      ? agents.find((a) => a.id === selection.id)
      : null;

  function handleLaunchResult(result: LaunchResult) {
    if (!result.task) return;
    const params = new URLSearchParams({
      project: result.task.project_id,
      task: result.task.id,
    });
    if (result.run_id) params.set("run", result.run_id);
    setSelection({ kind: "project", id: result.task.project_id });
    router.replace(`/panel/task-manager?${params.toString()}`, { scroll: false });
  }

  return (
    <div className="flex h-[calc(100dvh-3rem)] flex-col lg:h-screen">
      <div className="flex items-center justify-between border-b border-border px-6 py-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Task Manager</h1>
          <p className="text-sm text-muted-foreground">
            Projects, agents, and runs — one control center
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setCreateProjectOpen(true)}
            className="inline-flex h-9 items-center gap-1.5 rounded-md border border-border bg-background px-3 text-sm hover:bg-secondary"
          >
            <Plus className="h-4 w-4" /> New project
          </button>
          <button
            type="button"
            onClick={() => setCreateAgentOpen(true)}
            className="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-3 text-sm font-medium text-primary-foreground hover:opacity-90"
          >
            <Plus className="h-4 w-4" /> New agent
          </button>
        </div>
      </div>

      <div className="flex min-h-0 flex-1">
        {/* Master rail */}
        <aside className="flex w-64 shrink-0 flex-col border-r border-border bg-muted/20">
          <div className="flex-1 overflow-y-auto scrollbar-thin p-3">
            <p className="mb-1.5 px-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
              Projects
            </p>
            <ul className="mb-4 space-y-0.5">
              {projects.length === 0 ? (
                <li className="px-2 py-3 text-xs text-muted-foreground">
                  No projects yet
                </li>
              ) : (
                projects.map((p) => {
                  const active =
                    selection?.kind === "project" && selection.id === p.id;
                  return (
                    <li key={p.id}>
                      <button
                        type="button"
                        onClick={() => select({ kind: "project", id: p.id })}
                        className={cn(
                          "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm",
                          active
                            ? "bg-accent text-accent-foreground"
                            : "text-foreground hover:bg-secondary"
                        )}
                      >
                        <FolderKanban className="h-3.5 w-3.5 shrink-0 opacity-60" />
                        <span className="min-w-0 flex-1 truncate">{p.name}</span>
                        <span className="font-mono text-[10px] text-muted-foreground">
                          {taskCounts.get(p.id) ?? 0}
                        </span>
                      </button>
                    </li>
                  );
                })
              )}
            </ul>

            <p className="mb-1.5 px-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
              Agents
            </p>
            <ul className="space-y-0.5">
              {BUILTINS.map((b) => {
                const Icon = b.icon;
                const active =
                  selection?.kind === "builtin" && selection.id === b.id;
                return (
                  <li key={b.id}>
                    <button
                      type="button"
                      onClick={() => select({ kind: "builtin", id: b.id })}
                      className={cn(
                        "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm",
                        active
                          ? "bg-accent text-accent-foreground"
                          : "text-foreground hover:bg-secondary"
                      )}
                    >
                      <Icon className="h-3.5 w-3.5 shrink-0 opacity-60" />
                      <span className="truncate">{b.label}</span>
                    </button>
                  </li>
                );
              })}
              {agents
                .filter((a) => {
                  const builtin = a.metadata?.builtin;
                  return !BUILTINS.some((b) => b.match && b.match === builtin);
                })
                .map((a) => {
                  const active =
                    selection?.kind === "agent" && selection.id === a.id;
                  return (
                    <li key={a.id}>
                      <button
                        type="button"
                        onClick={() => select({ kind: "agent", id: a.id })}
                        className={cn(
                          "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm",
                          active
                            ? "bg-accent text-accent-foreground"
                            : "text-foreground hover:bg-secondary"
                        )}
                      >
                        <Bot className="h-3.5 w-3.5 shrink-0 opacity-60" />
                        <span className="min-w-0 flex-1 truncate">{a.name}</span>
                      </button>
                    </li>
                  );
                })}
            </ul>
          </div>
        </aside>

        {/* Detail */}
        <div className="min-w-0 flex-1 overflow-y-auto scrollbar-thin p-6">
          {selection?.kind === "project" && (
            <ProjectTasksView
              projectId={selection.id}
              agents={agents}
              projectName={
                projects.find((p) => p.id === selection.id)?.name ?? "Project"
              }
              onLaunched={handleLaunchResult}
            />
          )}
          {selection?.kind === "builtin" && selection.id === "pentest" && (
            <BuiltinFrame title="Security Tester">
              <PentestAgentClient />
            </BuiltinFrame>
          )}
          {selection?.kind === "builtin" && selection.id === "review" && (
            <BuiltinFrame title="PR Reviewer">
              <ReviewAgentClient />
            </BuiltinFrame>
          )}
          {selection?.kind === "builtin" && selection.id === "mail" && (
            <BuiltinFrame title="Mail Agent">
              <MailAgentClient />
            </BuiltinFrame>
          )}
          {selection?.kind === "builtin" && selection.id === "articles" && (
            <ArticlesPage />
          )}
          {selection?.kind === "builtin" && selection.id === "images" && (
            <ImagesPage />
          )}
          {selection?.kind === "agent" && selectedAgent && (
            <AgentDetailView
              agent={selectedAgent}
              agents={agents}
              projects={projects}
              onLaunched={handleLaunchResult}
            />
          )}
          {!selection && (
            <p className="text-sm text-muted-foreground">
              Select a project or agent.
            </p>
          )}
        </div>
      </div>

      <CreateAgentDialog
        open={createAgentOpen}
        onClose={() => setCreateAgentOpen(false)}
      />
      {createProjectOpen && (
        <NewProjectDialog
          onClose={() => setCreateProjectOpen(false)}
          onCreated={(p) => {
            setCreateProjectOpen(false);
            select({ kind: "project", id: p.id });
          }}
        />
      )}
    </div>
  );
}

function BuiltinFrame({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
      {children}
    </div>
  );
}

function ProjectTasksView({
  projectId,
  agents,
  projectName,
  onLaunched,
}: {
  projectId: string;
  agents: Agent[];
  projectName: string;
  onLaunched: (result: LaunchResult) => void;
}) {
  const searchParams = useSearchParams();
  const taskParam = searchParams.get("task");
  const runParam = searchParams.get("run");

  const { data: tasksResp, isLoading } = useProjectTasks(projectId);
  const tasks = useMemo(() => tasksResp?.data ?? [], [tasksResp]);
  const [selectedId, setSelectedId] = useState<string | null>(taskParam);
  const selected = tasks.find((t) => t.id === selectedId) ?? null;
  const [editorOpen, setEditorOpen] = useState(false);
  const [editorTask, setEditorTask] = useState<Task | undefined>();
  const [runOpen, setRunOpen] = useState(false);
  const [focusRunId, setFocusRunId] = useState<string | null>(runParam);
  const transition = useTransitionTask();
  const deleteTask = useDeleteTask();

  useEffect(() => {
    setSelectedId(taskParam);
    setFocusRunId(runParam);
  }, [projectId, taskParam, runParam]);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold tracking-tight">{projectName}</h2>
          <p className="text-sm text-muted-foreground">
            {tasks.length} task{tasks.length === 1 ? "" : "s"}
          </p>
        </div>
        <button
          type="button"
          onClick={() => {
            setEditorTask(undefined);
            setEditorOpen(true);
          }}
          className="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:opacity-90"
        >
          <Plus className="h-4 w-4" /> New task
        </button>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[300px_1fr]">
        <div className="space-y-1.5">
          {isLoading ? (
            <p className="text-sm text-muted-foreground">Loading…</p>
          ) : tasks.length === 0 ? (
            <p className="rounded-lg border border-dashed border-border px-4 py-8 text-center text-sm text-muted-foreground">
              No tasks yet. Create one to get started.
            </p>
          ) : (
            tasks.map((task) => {
              const agent = agents.find((a) => a.id === task.assigned_agent_id);
              const active = task.id === selectedId;
              return (
                <button
                  key={task.id}
                  type="button"
                  onClick={() => {
                    setSelectedId(task.id);
                    setFocusRunId(null);
                  }}
                  className={cn(
                    "w-full rounded-md border px-3 py-2.5 text-left transition-colors",
                    active
                      ? "border-primary bg-primary/10"
                      : "border-border bg-card hover:bg-secondary/60"
                  )}
                >
                  <div className="flex items-center gap-2">
                    <span
                      className={cn(
                        "h-2 w-2 shrink-0 rounded-full",
                        STATUS_DOT[task.status]
                      )}
                    />
                    <span className="min-w-0 flex-1 truncate text-sm font-medium">
                      {task.title}
                    </span>
                  </div>
                  <p className="mt-1 pl-4 text-xs text-muted-foreground">
                    {agent?.name ?? "Unassigned"}
                    {" · "}
                    {task.status.replace(/_/g, " ")}
                    {" · "}
                    {new Date(task.updated_at).toLocaleDateString()}
                  </p>
                </button>
              );
            })
          )}
        </div>

        <div>
          {selected ? (
            <div className="space-y-4 rounded-lg border border-border bg-card p-5">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <h3 className="text-lg font-semibold">{selected.title}</h3>
                  {selected.description && (
                    <p className="mt-1 whitespace-pre-wrap text-sm text-muted-foreground">
                      {selected.description}
                    </p>
                  )}
                </div>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => {
                      setEditorTask(selected);
                      setEditorOpen(true);
                    }}
                    className="h-9 rounded-md border border-border px-3 text-sm hover:bg-secondary"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    onClick={() => setRunOpen(true)}
                    className="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-3 text-sm font-medium text-primary-foreground"
                  >
                    <Rocket className="h-4 w-4" /> Run
                  </button>
                </div>
              </div>
              <div className="flex flex-wrap gap-3 text-sm">
                <label className="flex items-center gap-1.5">
                  <span className="text-muted-foreground">Status</span>
                  <select
                    value={selected.status}
                    onChange={(e) =>
                      transition.mutate({
                        id: selected.id,
                        payload: { status: e.target.value as TaskStatus },
                      })
                    }
                    className="h-8 rounded-md border border-input bg-background px-2 text-sm"
                  >
                    {STATUS_OPTIONS.map((s) => (
                      <option key={s.value} value={s.value}>
                        {s.label}
                      </option>
                    ))}
                  </select>
                </label>
                <button
                  type="button"
                  className="text-xs text-destructive hover:underline"
                  onClick={() => {
                    if (confirm(`Delete "${selected.title}"?`)) {
                      void deleteTask.mutateAsync(selected.id).then(() =>
                        setSelectedId(null)
                      );
                    }
                  }}
                >
                  Delete
                </button>
              </div>
              <div className="border-t border-border pt-4">
                <h4 className="mb-2 text-sm font-medium">Runs</h4>
                <TaskRunView
                  task={selected}
                  agents={agents}
                  focusRunId={focusRunId}
                />
              </div>
            </div>
          ) : (
            <div className="flex min-h-[200px] items-center justify-center rounded-lg border border-dashed border-border text-sm text-muted-foreground">
              Select a task
            </div>
          )}
        </div>
      </div>

      {editorOpen && (
        <TaskEditorDialog
          task={editorTask}
          projectId={projectId}
          agents={agents}
          onClose={() => setEditorOpen(false)}
          onSaved={(t) => setSelectedId(t.id)}
          onLaunched={(result) => {
            setSelectedId(result.task.id);
            if (result.run_id) setFocusRunId(result.run_id);
            onLaunched(result);
          }}
        />
      )}
      {runOpen && selected && (
        <RunTaskDialog
          task={selected}
          agents={agents}
          onClose={() => setRunOpen(false)}
          onLaunched={(run: TaskRun) => setFocusRunId(run.id)}
          onSpecialistLaunched={onLaunched}
        />
      )}
    </div>
  );
}

function AgentDetailView({
  agent,
  agents,
  projects,
  onLaunched,
}: {
  agent: Agent;
  agents: Agent[];
  projects: Project[];
  onLaunched: (result: LaunchResult) => void;
}) {
  const { data: tasksResp } = useTasks({
    assigned_agent_id: agent.id,
    per_page: 50,
  });
  const tasks = tasksResp?.data ?? [];
  const [runTask, setRunTask] = useState<Task | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="flex items-center gap-3">
            <h2 className="text-lg font-semibold tracking-tight">{agent.name}</h2>
            <AgentStatusBadge status={agent.status} />
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {agent.role}
            {agent.description ? ` — ${agent.description}` : ""}
          </p>
          {(agent.model_provider || agent.model_name) && (
            <p className="mt-1 font-mono text-xs text-muted-foreground">
              {[agent.model_provider, agent.model_name].filter(Boolean).join(" / ")}
            </p>
          )}
        </div>
        <div className="flex gap-2">
          <Link
            href={`/agents/${agent.id}`}
            className="inline-flex h-9 items-center gap-1.5 rounded-md border border-border px-3 text-sm hover:bg-secondary"
          >
            <BookOpen className="h-4 w-4" /> Full profile
          </Link>
          <button
            type="button"
            onClick={() => setCreateOpen(true)}
            className="inline-flex h-9 items-center gap-1.5 rounded-md border border-border px-3 text-sm hover:bg-secondary"
          >
            <Plus className="h-4 w-4" /> New task
          </button>
          <button
            type="button"
            onClick={() => {
              if (tasks[0]) {
                setRunTask(tasks[0]);
                return;
              }
              setCreateOpen(true);
            }}
            title={
              tasks[0] ? `Runs "${tasks[0].title}"` : "Create and run a task"
            }
            className="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:opacity-90"
          >
            <Rocket className="h-4 w-4" />{" "}
            {tasks[0] ? "Run task" : "Run agent"}
          </button>
        </div>
      </div>

      <section>
        <h3 className="mb-2 text-sm font-medium">Assigned tasks</h3>
        {tasks.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            No tasks yet. Use Run agent to fill this agent&apos;s inputs and
            execute it.
          </p>
        ) : (
          <ul className="divide-y divide-border rounded-lg border border-border">
            {tasks.map((t) => (
              <li
                key={t.id}
                className="flex items-center justify-between gap-3 px-4 py-3"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{t.title}</p>
                  <p className="text-xs text-muted-foreground">
                    {statusLabel(t.status)} · updated{" "}
                    {new Date(t.updated_at).toLocaleString()}
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => setRunTask(t)}
                  className="shrink-0 rounded-md border border-border px-2.5 py-1 text-xs hover:bg-secondary"
                >
                  Run
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      {tasks[0] && (
        <section>
          {/* This shows one task's runs, not the agent's whole history — say
              which task, so the heading doesn't overpromise. */}
          <h3 className="mb-2 truncate text-sm font-medium">
            Runs for &ldquo;{tasks[0].title}&rdquo;
          </h3>
          <TaskRunView task={tasks[0]} agents={agents} focusRunId={null} />
        </section>
      )}

      {createOpen && (
        <TaskEditorDialog
          agents={agents}
          projects={projects}
          initialAgentId={agent.id}
          onClose={() => setCreateOpen(false)}
          onSaved={(t) => {
            setCreateOpen(false);
            setRunTask(t);
          }}
          onLaunched={(result) => {
            setCreateOpen(false);
            setRunTask(null);
            onLaunched(result);
          }}
        />
      )}

      {runTask && (
        <RunTaskDialog
          task={runTask}
          agents={agents}
          onClose={() => setRunTask(null)}
          onLaunched={() => setRunTask(null)}
          onSpecialistLaunched={(result) => {
            setRunTask(null);
            onLaunched(result);
          }}
        />
      )}
    </div>
  );
}
