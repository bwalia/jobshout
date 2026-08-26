"use client";

import { useEffect, useMemo, useState } from "react";
import { Rocket, X } from "lucide-react";
import { toast } from "sonner";

import { AgentInputFields } from "@/components/task-manager/AgentInputFields";
import {
  defaultValuesForSchema,
  getAgentInputSchema,
  schemaValuesValid,
  taskFieldsFromValues,
} from "@/lib/agents/input-schemas";
import { launchAgentForTask, type LaunchResult } from "@/lib/agents/launch";
import { apiErrorMessage } from "@/lib/api/client";
import { useCreateTask, useUpdateTask } from "@/lib/hooks/useTasks";
import type { Agent } from "@/lib/types/agent";
import type { Priority, TaskStatus } from "@/lib/types/common";
import type { Project, Task } from "@/lib/types/project";

const inputCls =
  "flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

const STATUSES: TaskStatus[] = [
  "backlog",
  "todo",
  "in_progress",
  "review",
  "done",
];
const PRIORITIES: Priority[] = ["low", "medium", "high", "critical"];

interface TaskEditorDialogProps {
  /** Present => edit that task; absent => create a new one. */
  task?: Task;
  /** Required when creating (unless projects list is provided for picker). */
  projectId?: string;
  /** When creating, allow picking a project if projectId is not fixed. */
  projects?: Project[];
  /** When creating a subtask. */
  parentId?: string;
  agents: Agent[];
  /** Pre-select an agent when opening create (e.g. from agent detail). */
  initialAgentId?: string;
  onClose: () => void;
  onSaved?: (task: Task) => void;
  /** Fired after Create & Run succeeds — parent can navigate to the run. */
  onLaunched?: (result: LaunchResult) => void;
}

/**
 * Create or edit a task. Create is agent-first: pick the agent, then fill that
 * agent's inputs, then optionally create & run so every agent is executable
 * from Task Manager.
 */
export function TaskEditorDialog({
  task,
  projectId: fixedProjectId,
  projects,
  parentId,
  agents,
  initialAgentId,
  onClose,
  onSaved,
  onLaunched,
}: TaskEditorDialogProps) {
  const isEdit = Boolean(task);
  const createTask = useCreateTask();
  const updateTask = useUpdateTask();

  // ── Edit-mode state (board metadata) ──────────────────────────────────
  const [title, setTitle] = useState(task?.title ?? "");
  const [description, setDescription] = useState(task?.description ?? "");
  const [status, setStatus] = useState<TaskStatus>(task?.status ?? "todo");
  const [priority, setPriority] = useState<Priority>(task?.priority ?? "medium");
  const [assignedAgentId, setAssignedAgentId] = useState<string>(
    task?.assigned_agent_id ?? initialAgentId ?? ""
  );
  const [storyPoints, setStoryPoints] = useState<string>(
    task?.story_points != null ? String(task.story_points) : ""
  );
  const [dueDate, setDueDate] = useState<string>(
    task?.due_date ? task.due_date.slice(0, 10) : ""
  );

  // ── Create-mode state (agent-first) ───────────────────────────────────
  const [agentId, setAgentId] = useState(initialAgentId ?? "");
  const [projectId, setProjectId] = useState(fixedProjectId ?? "");
  const selectedAgent = useMemo(
    () => agents.find((a) => a.id === agentId) ?? null,
    [agents, agentId]
  );
  const schema = useMemo(
    () => getAgentInputSchema(selectedAgent),
    [selectedAgent]
  );
  const [values, setValues] = useState<Record<string, string>>(() =>
    defaultValuesForSchema(getAgentInputSchema(null))
  );
  const [launching, setLaunching] = useState(false);

  useEffect(() => {
    if (isEdit) return;
    setValues(defaultValuesForSchema(schema));
  }, [schema, isEdit]);

  const pending = createTask.isPending || updateTask.isPending || launching;
  const createReady =
    Boolean(agentId) &&
    Boolean(projectId || fixedProjectId) &&
    schemaValuesValid(schema, values);

  function setValue(key: string, value: string) {
    setValues((prev) => ({ ...prev, [key]: value }));
  }

  async function createBoardTask(): Promise<Task> {
    const pid = fixedProjectId || projectId;
    if (!pid || !selectedAgent) throw new Error("Project and agent are required");
    const { title: t, description: d } = taskFieldsFromValues(schema, values);
    return createTask.mutateAsync({
      project_id: pid,
      parent_id: parentId,
      title: t,
      description: d,
      priority,
      assigned_agent_id: selectedAgent.id,
    });
  }

  async function handleCreateOnly(e: React.FormEvent) {
    e.preventDefault();
    if (!createReady) return;
    try {
      const created = await createBoardTask();
      onSaved?.(created);
      onClose();
    } catch {
      // toast from hook
    }
  }

  async function handleCreateAndRun() {
    if (!createReady || !selectedAgent) return;
    setLaunching(true);
    try {
      const created = await createBoardTask();
      onSaved?.(created);
      const result = await launchAgentForTask({
        agent: selectedAgent,
        task: created,
        schema,
        values,
      });
      toast.success(
        result.kind === "researcher"
          ? "Research complete"
          : result.kind === "article_writer"
            ? "Article run started"
            : result.kind === "pentester"
              ? "Security scan queued"
              : result.kind === "pr_reviewer"
                ? "PR review queued"
                : "Agent run started"
      );
      onLaunched?.(result);
      onClose();
    } catch (err) {
      toast.error(apiErrorMessage(err, "Failed to launch agent"));
    } finally {
      setLaunching(false);
    }
  }

  async function handleEditSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!title.trim() || !task) return;
    const points = storyPoints.trim() === "" ? null : Number(storyPoints);
    try {
      const updated = await updateTask.mutateAsync({
        id: task.id,
        payload: {
          title: title.trim(),
          description: description.trim() || undefined,
          status,
          priority,
          assigned_agent_id: assignedAgentId || null,
          story_points: points,
          due_date: dueDate || null,
        },
      });
      onSaved?.(updated);
      onClose();
    } catch {
      // toast from hook
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={onClose}
    >
      <div
        className="flex max-h-[90vh] w-full max-w-lg flex-col rounded-xl border border-border bg-card shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-6 py-4">
          <h2 className="font-display text-lg font-semibold">
            {isEdit ? "Edit Task" : parentId ? "New Subtask" : "New Task"}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
            aria-label="Close"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {isEdit ? (
          <form
            onSubmit={handleEditSubmit}
            className="space-y-4 overflow-y-auto px-6 py-5 scrollbar-thin"
          >
            <div className="space-y-1.5">
              <label className="text-sm font-medium">
                Assigned agent
              </label>
              <select
                value={assignedAgentId}
                onChange={(e) => setAssignedAgentId(e.target.value)}
                className={inputCls}
              >
                <option value="">— Unassigned —</option>
                {agents.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name} · {a.role}
                  </option>
                ))}
              </select>
            </div>

            <div className="space-y-1.5">
              <label className="text-sm font-medium">
                Title <span className="text-destructive">*</span>
              </label>
              <input
                type="text"
                required
                autoFocus
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                className={inputCls}
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-sm font-medium">Description</label>
              <textarea
                rows={4}
                value={description ?? ""}
                onChange={(e) => setDescription(e.target.value)}
                className="w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </div>

            <div className="grid grid-cols-2 gap-3">
              <div className="space-y-1.5">
                <label className="text-sm font-medium">Status</label>
                <select
                  value={status}
                  onChange={(e) => setStatus(e.target.value as TaskStatus)}
                  className={inputCls}
                >
                  {STATUSES.map((s) => (
                    <option key={s} value={s}>
                      {s.replace("_", " ")}
                    </option>
                  ))}
                </select>
              </div>
              <div className="space-y-1.5">
                <label className="text-sm font-medium">Priority</label>
                <select
                  value={priority}
                  onChange={(e) => setPriority(e.target.value as Priority)}
                  className={inputCls}
                >
                  {PRIORITIES.map((p) => (
                    <option key={p} value={p}>
                      {p}
                    </option>
                  ))}
                </select>
              </div>
              <div className="space-y-1.5">
                <label className="text-sm font-medium">Story points</label>
                <input
                  type="number"
                  min={0}
                  value={storyPoints}
                  onChange={(e) => setStoryPoints(e.target.value)}
                  className={inputCls}
                />
              </div>
              <div className="space-y-1.5">
                <label className="text-sm font-medium">Due date</label>
                <input
                  type="date"
                  value={dueDate}
                  onChange={(e) => setDueDate(e.target.value)}
                  className={inputCls}
                />
              </div>
            </div>

            <div className="flex justify-end gap-2 pt-1">
              <button
                type="button"
                onClick={onClose}
                className="inline-flex h-9 items-center rounded-md border border-border bg-background px-4 text-sm font-medium hover:bg-accent"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={pending || !title.trim()}
                className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
              >
                {pending ? "Saving…" : "Save changes"}
              </button>
            </div>
          </form>
        ) : (
          <form
            onSubmit={handleCreateOnly}
            className="flex min-h-0 flex-1 flex-col"
          >
            <div className="space-y-4 overflow-y-auto px-6 py-5 scrollbar-thin">
              {/* 1. Agent first */}
              <div className="space-y-1.5">
                <label className="text-sm font-medium">
                  Agent <span className="text-destructive">*</span>
                </label>
                <select
                  value={agentId}
                  onChange={(e) => setAgentId(e.target.value)}
                  autoFocus={!initialAgentId}
                  className={inputCls}
                >
                  <option value="">— Choose an agent —</option>
                  {agents.map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.name}
                      {a.metadata?.builtin ? ` · ${a.metadata.builtin}` : ` · ${a.role}`}
                    </option>
                  ))}
                </select>
                {selectedAgent && (
                  <p className="text-xs text-muted-foreground">{schema.hint}</p>
                )}
              </div>

              {/* 2. Agent-specific inputs */}
              {selectedAgent && (
                <AgentInputFields
                  fields={schema.fields}
                  values={values}
                  onChange={setValue}
                  disabled={pending}
                  autoFocusFirst={Boolean(initialAgentId || agentId)}
                />
              )}

              {/* 3. Project + priority */}
              {selectedAgent && (
                <div className="grid grid-cols-2 gap-3 border-t border-border pt-4">
                  {!fixedProjectId && (
                    <div className="col-span-2 space-y-1.5 sm:col-span-1">
                      <label className="text-sm font-medium">
                        Project <span className="text-destructive">*</span>
                      </label>
                      <select
                        value={projectId}
                        onChange={(e) => setProjectId(e.target.value)}
                        required
                        className={inputCls}
                      >
                        <option value="">— Choose a project —</option>
                        {(projects ?? []).map((p) => (
                          <option key={p.id} value={p.id}>
                            {p.name}
                          </option>
                        ))}
                      </select>
                    </div>
                  )}
                  <div className="space-y-1.5">
                    <label className="text-sm font-medium">Priority</label>
                    <select
                      value={priority}
                      onChange={(e) => setPriority(e.target.value as Priority)}
                      className={inputCls}
                    >
                      {PRIORITIES.map((p) => (
                        <option key={p} value={p}>
                          {p}
                        </option>
                      ))}
                    </select>
                  </div>
                </div>
              )}
            </div>

            <div className="flex flex-wrap justify-end gap-2 border-t border-border px-6 py-4">
              <button
                type="button"
                onClick={onClose}
                className="inline-flex h-9 items-center rounded-md border border-border bg-background px-4 text-sm font-medium hover:bg-accent"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={pending || !createReady}
                className="inline-flex h-9 items-center rounded-md border border-border bg-background px-4 text-sm font-medium hover:bg-accent disabled:pointer-events-none disabled:opacity-50"
              >
                {createTask.isPending && !launching ? "Creating…" : "Create task"}
              </button>
              <button
                type="button"
                onClick={() => void handleCreateAndRun()}
                disabled={pending || !createReady}
                className="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
              >
                <Rocket className="h-4 w-4" />
                {launching ? "Starting…" : "Create & run"}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}
