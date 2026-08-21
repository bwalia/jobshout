"use client";

import { useState } from "react";
import { X } from "lucide-react";

import { useCreateTask, useUpdateTask } from "@/lib/hooks/useTasks";
import type { Agent } from "@/lib/types/agent";
import type { Priority, TaskStatus } from "@/lib/types/common";
import type { Task } from "@/lib/types/project";

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
  /** Required when creating. */
  projectId?: string;
  /** When creating a subtask. */
  parentId?: string;
  agents: Agent[];
  onClose: () => void;
  onSaved?: (task: Task) => void;
}

/**
 * Create or fully edit a task — every field the model carries, not just title
 * and status. This is the editor the old Task Manager lacked.
 */
export function TaskEditorDialog({
  task,
  projectId,
  parentId,
  agents,
  onClose,
  onSaved,
}: TaskEditorDialogProps) {
  const isEdit = Boolean(task);
  const createTask = useCreateTask();
  const updateTask = useUpdateTask();

  const [title, setTitle] = useState(task?.title ?? "");
  const [description, setDescription] = useState(task?.description ?? "");
  const [status, setStatus] = useState<TaskStatus>(task?.status ?? "todo");
  const [priority, setPriority] = useState<Priority>(task?.priority ?? "medium");
  const [assignedAgentId, setAssignedAgentId] = useState<string>(
    task?.assigned_agent_id ?? ""
  );
  const [storyPoints, setStoryPoints] = useState<string>(
    task?.story_points != null ? String(task.story_points) : ""
  );
  const [dueDate, setDueDate] = useState<string>(
    task?.due_date ? task.due_date.slice(0, 10) : ""
  );

  const pending = createTask.isPending || updateTask.isPending;

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!title.trim()) return;

    const points = storyPoints.trim() === "" ? null : Number(storyPoints);

    try {
      if (isEdit && task) {
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
      } else if (projectId) {
        const created = await createTask.mutateAsync({
          project_id: projectId,
          parent_id: parentId,
          title: title.trim(),
          description: description.trim() || undefined,
          priority,
          assigned_agent_id: assignedAgentId || undefined,
          story_points: points ?? undefined,
          due_date: dueDate || undefined,
        });
        onSaved?.(created);
      }
      onClose();
    } catch {
      // toast is surfaced by the mutation hooks
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={onClose}
    >
      <div
        className="w-full max-w-lg rounded-xl border border-border bg-card p-6 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between">
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

        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
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
              placeholder="e.g. Draft the launch announcement"
              className={inputCls}
            />
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-medium">Description</label>
            <textarea
              rows={4}
              value={description ?? ""}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="What should be done? This becomes the agent's prompt when you run the task."
              className="w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          <div className="grid grid-cols-2 gap-3">
            {isEdit && (
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

            <div className="space-y-1.5">
              <label className="text-sm font-medium">Assigned agent</label>
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
              <label className="text-sm font-medium">Story points</label>
              <input
                type="number"
                min={0}
                value={storyPoints}
                onChange={(e) => setStoryPoints(e.target.value)}
                placeholder="—"
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
              {pending ? "Saving…" : isEdit ? "Save changes" : "Create task"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
