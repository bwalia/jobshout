"use client";

import { useEffect, useMemo, useState } from "react";
import { Loader2, Rocket, X } from "lucide-react";
import { toast } from "sonner";

import { AgentInputFields } from "@/components/task-manager/AgentInputFields";
import {
  TASK_TITLE_MIN_LENGTH,
  defaultValuesForSchema,
  getAgentInputSchema,
  schemaValuesValid,
  taskFieldsFromValues,
  validateSchemaValues,
  validateTaskTitle,
} from "@/lib/agents/input-schemas";
import { launchAgentForTask, type LaunchResult } from "@/lib/agents/launch";
import { fetchMailFormValues, mailFormIsBlank } from "@/lib/agents/mail-playbook";
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
 * Shell for create-or-edit. Create and edit are separate forms so submit
 * handlers cannot cross — edit never creates, create never updates.
 */
export function TaskEditorDialog(props: TaskEditorDialogProps) {
  const { onClose, task } = props;
  const isEdit = Boolean(task);

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
            {isEdit
              ? "Edit Task"
              : props.parentId
                ? "New Subtask"
                : "New Task"}
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

        {task ? (
          <EditTaskForm
            task={task}
            agents={props.agents}
            onClose={onClose}
            onSaved={props.onSaved}
          />
        ) : (
          <CreateTaskForm
            projectId={props.projectId}
            projects={props.projects}
            parentId={props.parentId}
            agents={props.agents}
            initialAgentId={props.initialAgentId}
            onClose={onClose}
            onSaved={props.onSaved}
            onLaunched={props.onLaunched}
          />
        )}
      </div>
    </div>
  );
}

/** Board-metadata editor — PATCH /tasks/{id} only. */
function EditTaskForm({
  task,
  agents,
  onClose,
  onSaved,
}: {
  task: Task;
  agents: Agent[];
  onClose: () => void;
  onSaved?: (task: Task) => void;
}) {
  const updateTask = useUpdateTask();
  const [title, setTitle] = useState(task.title);
  const [description, setDescription] = useState(task.description ?? "");
  const [status, setStatus] = useState<TaskStatus>(task.status);
  const [priority, setPriority] = useState<Priority>(task.priority);
  const [assignedAgentId, setAssignedAgentId] = useState(
    task.assigned_agent_id ?? ""
  );
  const [storyPoints, setStoryPoints] = useState(
    task.story_points != null ? String(task.story_points) : ""
  );
  const [dueDate, setDueDate] = useState(
    task.due_date ? task.due_date.slice(0, 10) : ""
  );
  const [titleError, setTitleError] = useState<string | null>(null);

  const titleOk = !validateTaskTitle(title);
  const pending = updateTask.isPending;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const err = validateTaskTitle(title);
    setTitleError(err);
    if (err) return;

    const points = storyPoints.trim() === "" ? null : Number(storyPoints);
    if (storyPoints.trim() !== "" && !Number.isFinite(points)) {
      toast.error("Story points must be a number");
      return;
    }

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
    <form
      onSubmit={handleSubmit}
      noValidate
      className="space-y-4 overflow-y-auto px-6 py-5 scrollbar-thin"
    >
      <div className="space-y-1.5">
        <label className="text-sm font-medium" htmlFor="edit-task-agent">
          Assigned agent
        </label>
        <select
          id="edit-task-agent"
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
        <label className="text-sm font-medium" htmlFor="edit-task-title">
          Title <span className="text-destructive">*</span>
        </label>
        <input
          id="edit-task-title"
          type="text"
          required
          minLength={TASK_TITLE_MIN_LENGTH}
          autoFocus
          value={title}
          onChange={(e) => {
            setTitle(e.target.value);
            if (titleError) setTitleError(validateTaskTitle(e.target.value));
          }}
          onBlur={() => setTitleError(validateTaskTitle(title))}
          aria-invalid={Boolean(titleError)}
          className={
            inputCls + (titleError ? " border-destructive" : "")
          }
        />
        {titleError ? (
          <p className="text-xs text-destructive" role="alert">
            {titleError}
          </p>
        ) : (
          <p className="text-xs text-muted-foreground">
            At least {TASK_TITLE_MIN_LENGTH} characters
          </p>
        )}
      </div>

      <div className="space-y-1.5">
        <label className="text-sm font-medium" htmlFor="edit-task-desc">
          Description
        </label>
        <textarea
          id="edit-task-desc"
          rows={4}
          value={description}
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
          disabled={pending || !titleOk}
          className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
        >
          {pending ? "Saving…" : "Save changes"}
        </button>
      </div>
    </form>
  );
}

/**
 * Agent-first create — POST /tasks, optionally launch the agent.
 * Never updates an existing task.
 */
function CreateTaskForm({
  projectId: fixedProjectId,
  projects,
  parentId,
  agents,
  initialAgentId,
  onClose,
  onSaved,
  onLaunched,
}: {
  projectId?: string;
  projects?: Project[];
  parentId?: string;
  agents: Agent[];
  initialAgentId?: string;
  onClose: () => void;
  onSaved?: (task: Task) => void;
  onLaunched?: (result: LaunchResult) => void;
}) {
  const createTask = useCreateTask();
  const [agentId, setAgentId] = useState(initialAgentId ?? "");
  const [projectId, setProjectId] = useState(fixedProjectId ?? "");
  const [priority, setPriority] = useState<Priority>("medium");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [launching, setLaunching] = useState(false);
  const [touchedSubmit, setTouchedSubmit] = useState(false);

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
  /** idle until Mail is selected; ready only after GET mailbox (or GET failed). */
  const [mailboxLoad, setMailboxLoad] = useState<"idle" | "loading" | "ready">(
    "idle"
  );

  useEffect(() => {
    setValues(defaultValuesForSchema(schema));
    setFieldErrors({});
    setFormError(null);
    setTouchedSubmit(false);
    if (schema.kind !== "mail") {
      setMailboxLoad("idle");
      return;
    }
    setMailboxLoad("loading");
    let cancelled = false;
    void fetchMailFormValues()
      .then((saved) => {
        if (cancelled || !saved) return;
        setValues((prev) => (mailFormIsBlank(prev) ? { ...prev, ...saved } : prev));
      })
      .finally(() => {
        if (!cancelled) setMailboxLoad("ready");
      });
    return () => {
      cancelled = true;
    };
  }, [schema]);

  const pending = createTask.isPending || launching;
  const resolvedProjectId = fixedProjectId || projectId;
  const mailReady = schema.kind !== "mail" || mailboxLoad === "ready";
  const schemaOk =
    Boolean(selectedAgent) && schemaValuesValid(schema, values);
  const createReady = schemaOk && Boolean(resolvedProjectId) && mailReady;

  function setValue(key: string, value: string) {
    const next = { ...values, [key]: value };
    setValues(next);
    // Re-validate after a failed submit (or when this field already has an
    // error) so filling a blank required field clears the message immediately.
    if (touchedSubmit || fieldErrors[key]) {
      const errs = validateSchemaValues(schema, next);
      setFieldErrors(errs);
      if (Object.keys(errs).length === 0) setFormError(null);
    }
  }

  /** Re-validate schema + project; returns false and surfaces errors if invalid. */
  function assertCreateReady(): boolean {
    if (!selectedAgent) {
      setFormError("Choose an agent first");
      return false;
    }
    if (!resolvedProjectId) {
      setFormError("Choose a project");
      return false;
    }
    if (!mailReady) {
      setFormError("Loading saved mailbox settings…");
      return false;
    }
    setTouchedSubmit(true);
    const errs = validateSchemaValues(schema, values);
    setFieldErrors(errs);
    if (Object.keys(errs).length > 0) {
      setFormError("Fix the highlighted fields before continuing");
      return false;
    }
    setFormError(null);
    return true;
  }

  async function createBoardTask(): Promise<Task> {
    if (!selectedAgent || !resolvedProjectId) {
      throw new Error("Project and agent are required");
    }
    const { title: t, description: d } = taskFieldsFromValues(schema, values);
    const titleErr = validateTaskTitle(t);
    if (titleErr) throw new Error(titleErr);

    return createTask.mutateAsync({
      project_id: resolvedProjectId,
      parent_id: parentId,
      title: t,
      description: d,
      priority,
      assigned_agent_id: selectedAgent.id,
    });
  }

  async function handleCreateOnly(e: React.FormEvent) {
    e.preventDefault();
    if (pending) return;
    if (!assertCreateReady()) return;
    try {
      const created = await createBoardTask();
      onSaved?.(created);
      onClose();
    } catch (err) {
      setFormError(apiErrorMessage(err, "Failed to create task"));
      toast.error(apiErrorMessage(err, "Failed to create task"));
    }
  }

  async function handleCreateAndRun() {
    if (pending) return;
    if (!assertCreateReady() || !selectedAgent) return;
    setLaunching(true);
    setFormError(null);
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
                : result.kind === "mail"
                  ? "Playbook saved and mailbox sync queued"
                  : "Agent run started"
      );
      onLaunched?.(result);
      onClose();
    } catch (err) {
      const msg = apiErrorMessage(err, "Failed to launch agent");
      setFormError(msg);
      toast.error(msg);
    } finally {
      setLaunching(false);
    }
  }

  return (
    <form
      onSubmit={handleCreateOnly}
      noValidate
      className="flex min-h-0 flex-1 flex-col"
    >
      <div className="space-y-4 overflow-y-auto px-6 py-5 scrollbar-thin">
        <div className="space-y-1.5">
          <label className="text-sm font-medium" htmlFor="create-task-agent">
            Agent <span className="text-destructive">*</span>
          </label>
          <select
            id="create-task-agent"
            value={agentId}
            onChange={(e) => {
              setAgentId(e.target.value);
              setFormError(null);
            }}
            autoFocus={!initialAgentId}
            required
            className={inputCls}
          >
            <option value="">— Choose an agent —</option>
            {agents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
                {a.metadata?.builtin
                  ? ` · ${a.metadata.builtin}`
                  : ` · ${a.role}`}
              </option>
            ))}
          </select>
          {selectedAgent && (
            <p className="text-xs text-muted-foreground">{schema.hint}</p>
          )}
        </div>

        {selectedAgent && schema.kind === "mail" && mailboxLoad === "loading" && (
          <p className="text-xs text-muted-foreground">
            Loading saved mailbox settings…
          </p>
        )}

        {selectedAgent && (
          <AgentInputFields
            fields={schema.fields}
            values={values}
            onChange={setValue}
            errors={fieldErrors}
            disabled={pending || !mailReady}
            autoFocusFirst={Boolean(initialAgentId || agentId)}
          />
        )}

        {selectedAgent && (
          <div className="grid grid-cols-2 gap-3 border-t border-border pt-4">
            {!fixedProjectId && (
              <div className="col-span-2 space-y-1.5 sm:col-span-1">
                <label
                  className="text-sm font-medium"
                  htmlFor="create-task-project"
                >
                  Project <span className="text-destructive">*</span>
                </label>
                <select
                  id="create-task-project"
                  value={projectId}
                  onChange={(e) => {
                    setProjectId(e.target.value);
                    setFormError(null);
                  }}
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

        {formError && (
          <p className="text-sm text-destructive" role="alert">
            {formError}
          </p>
        )}
      </div>

      <div className="flex flex-wrap justify-end gap-2 border-t border-border px-6 py-4">
        <button
          type="button"
          onClick={onClose}
          disabled={pending}
          className="inline-flex h-9 items-center rounded-md border border-border bg-background px-4 text-sm font-medium hover:bg-accent disabled:opacity-50"
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
          {launching ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Rocket className="h-4 w-4" />
          )}
          {launching ? "Starting…" : "Create & run"}
        </button>
      </div>
    </form>
  );
}
