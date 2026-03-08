"use client";

import { useState, useEffect } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { updateTask } from "@/lib/api/tasks";
import type { Task, UpdateTaskRequest } from "@/lib/types/project";
import type { TaskStatus, Priority } from "@/lib/types/common";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STATUS_OPTIONS: { value: TaskStatus; label: string }[] = [
  { value: "backlog", label: "Backlog" },
  { value: "todo", label: "Todo" },
  { value: "in_progress", label: "In Progress" },
  { value: "review", label: "Review" },
  { value: "done", label: "Done" },
];

const PRIORITY_OPTIONS: { value: Priority; label: string }[] = [
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
  { value: "critical", label: "Critical" },
];

// ---------------------------------------------------------------------------
// TaskDetailModal
// ---------------------------------------------------------------------------

interface TaskDetailModalProps {
  task: Task;
  /** Called when the modal should close (cancel or after save) */
  onClose: () => void;
  /** Called after a successful save so the parent can update its list */
  onUpdated?: (updatedTask: Task) => void;
}

/**
 * Slide-over style modal for viewing and editing a task's details.
 *
 * Editable fields:
 * - Title
 * - Description
 * - Status
 * - Priority
 * - Assignee (agent ID text field — a real implementation would use a picker)
 * - Due date
 */
export function TaskDetailModal({ task, onClose, onUpdated }: TaskDetailModalProps) {
  const queryClient = useQueryClient();

  // Local form state initialised from the task prop
  const [title, setTitle] = useState(task.title);
  const [description, setDescription] = useState(task.description ?? "");
  const [status, setStatus] = useState<TaskStatus>(task.status);
  const [priority, setPriority] = useState<Priority>(task.priority);
  const [assignedAgentId, setAssignedAgentId] = useState(
    task.assigned_agent_id ?? ""
  );
  const [dueDate, setDueDate] = useState(
    task.due_date ? task.due_date.slice(0, 10) : ""
  );

  // Keep local state fresh if the task prop changes (e.g. external update)
  useEffect(() => {
    setTitle(task.title);
    setDescription(task.description ?? "");
    setStatus(task.status);
    setPriority(task.priority);
    setAssignedAgentId(task.assigned_agent_id ?? "");
    setDueDate(task.due_date ? task.due_date.slice(0, 10) : "");
  }, [task]);

  const updateMutation = useMutation({
    mutationFn: (payload: UpdateTaskRequest) => updateTask(task.id, payload),
    onSuccess: (updatedTask) => {
      queryClient.invalidateQueries({
        queryKey: ["tasks", task.project_id],
      });
      toast.success("Task updated.");
      onUpdated?.(updatedTask);
      onClose();
    },
    onError: (error: Error) => {
      toast.error(`Failed to update task: ${error.message}`);
    },
  });

  function handleSave() {
    if (!title.trim()) return;

    updateMutation.mutate({
      title: title.trim(),
      description: description.trim() || undefined,
      status,
      priority,
      assigned_agent_id: assignedAgentId.trim() || null,
      due_date: dueDate || null,
    });
  }

  return (
    // Backdrop
    <div
      className="fixed inset-0 z-50 flex items-start justify-end bg-black/40"
      onClick={onClose}
    >
      {/* Slide-over panel */}
      <div
        className="flex h-full w-full max-w-md flex-col overflow-y-auto border-l border-border bg-card shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-border px-6 py-4">
          <h2 className="text-base font-semibold">Task Detail</h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
            aria-label="Close"
          >
            <svg
              className="h-5 w-5"
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <line x1="18" y1="6" x2="6" y2="18" />
              <line x1="6" y1="6" x2="18" y2="18" />
            </svg>
          </button>
        </div>

        {/* Form body */}
        <div className="flex-1 space-y-5 px-6 py-5">
          {/* Title */}
          <div className="space-y-1.5">
            <label htmlFor="task-title" className="text-sm font-medium">
              Title <span className="text-destructive">*</span>
            </label>
            <input
              id="task-title"
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          {/* Description */}
          <div className="space-y-1.5">
            <label htmlFor="task-desc" className="text-sm font-medium">
              Description
            </label>
            <textarea
              id="task-desc"
              rows={5}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Add a description…"
              className="flex w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          {/* Status + Priority row */}
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <label htmlFor="task-status" className="text-sm font-medium">
                Status
              </label>
              <select
                id="task-status"
                value={status}
                onChange={(e) => setStatus(e.target.value as TaskStatus)}
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                {STATUS_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>

            <div className="space-y-1.5">
              <label htmlFor="task-priority" className="text-sm font-medium">
                Priority
              </label>
              <select
                id="task-priority"
                value={priority}
                onChange={(e) => setPriority(e.target.value as Priority)}
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                {PRIORITY_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>
          </div>

          {/* Assignee picker (simplified — shows agent ID input) */}
          <div className="space-y-1.5">
            <label htmlFor="task-assignee" className="text-sm font-medium">
              Assigned Agent ID
            </label>
            <input
              id="task-assignee"
              type="text"
              value={assignedAgentId}
              onChange={(e) => setAssignedAgentId(e.target.value)}
              placeholder="Paste agent ID or leave blank"
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          {/* Due date */}
          <div className="space-y-1.5">
            <label htmlFor="task-due-date" className="text-sm font-medium">
              Due Date
            </label>
            <input
              id="task-due-date"
              type="date"
              value={dueDate}
              onChange={(e) => setDueDate(e.target.value)}
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>
        </div>

        {/* Footer actions */}
        <div className="flex justify-end gap-2 border-t border-border px-6 py-4">
          <button
            type="button"
            onClick={onClose}
            className="inline-flex h-9 items-center rounded-md border border-border bg-background px-4 text-sm font-medium hover:bg-accent"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={handleSave}
            disabled={updateMutation.isPending || !title.trim()}
            className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
          >
            {updateMutation.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}
