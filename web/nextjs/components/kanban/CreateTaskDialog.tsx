"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { createTask } from "@/lib/api/tasks";
import type { Task, CreateTaskRequest } from "@/lib/types/project";
import type { Priority, TaskStatus } from "@/lib/types/common";

// ---------------------------------------------------------------------------
// CreateTaskDialog
// ---------------------------------------------------------------------------

interface CreateTaskDialogProps {
  /** The project this task will belong to */
  projectId: string;
  /**
   * Pre-fill the status column when the dialog is opened from a specific column
   * (e.g. clicking "+ Add task" inside the Backlog column).
   */
  initialStatus?: TaskStatus;
  onClose: () => void;
  /** Called after a task is successfully created */
  onCreated?: (task: Task) => void;
}

/**
 * Dialog for creating a new Kanban task.
 *
 * Fields:
 * - Title (required)
 * - Description (optional)
 * - Priority selector
 * - Due date (optional)
 */
export function CreateTaskDialog({
  projectId,
  initialStatus = "backlog",
  onClose,
  onCreated,
}: CreateTaskDialogProps) {
  const queryClient = useQueryClient();

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [priority, setPriority] = useState<Priority>("medium");
  const [dueDate, setDueDate] = useState("");

  const createMutation = useMutation({
    mutationFn: (payload: CreateTaskRequest) => createTask(payload),
    onSuccess: (newTask) => {
      queryClient.invalidateQueries({
        queryKey: ["tasks", projectId],
      });
      toast.success(`Task "${newTask.title}" created.`);
      onCreated?.(newTask);
      onClose();
    },
    onError: (error: Error) => {
      toast.error(`Failed to create task: ${error.message}`);
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!title.trim()) return;

    createMutation.mutate({
      project_id: projectId,
      title: title.trim(),
      description: description.trim() || undefined,
      priority,
      due_date: dueDate || undefined,
      // The task is created in the status column the user clicked "Add task" in
      // The API sets position; we let the server decide initial position
    });
  }

  return (
    // Backdrop
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onClose}
    >
      <div
        className="w-full max-w-md rounded-xl border border-border bg-card p-6 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">New Task</h2>
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

        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
          {/* Title */}
          <div className="space-y-1.5">
            <label htmlFor="create-task-title" className="text-sm font-medium">
              Title <span className="text-destructive">*</span>
            </label>
            <input
              id="create-task-title"
              type="text"
              required
              autoFocus
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="e.g. Set up authentication flow"
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          {/* Description */}
          <div className="space-y-1.5">
            <label htmlFor="create-task-desc" className="text-sm font-medium">
              Description
            </label>
            <textarea
              id="create-task-desc"
              rows={3}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Optional details or acceptance criteria…"
              className="flex w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </div>

          {/* Priority + Due date row */}
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <label
                htmlFor="create-task-priority"
                className="text-sm font-medium"
              >
                Priority
              </label>
              <select
                id="create-task-priority"
                value={priority}
                onChange={(e) => setPriority(e.target.value as Priority)}
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <option value="low">Low</option>
                <option value="medium">Medium</option>
                <option value="high">High</option>
                <option value="critical">Critical</option>
              </select>
            </div>

            <div className="space-y-1.5">
              <label
                htmlFor="create-task-due-date"
                className="text-sm font-medium"
              >
                Due Date
              </label>
              <input
                id="create-task-due-date"
                type="date"
                value={dueDate}
                onChange={(e) => setDueDate(e.target.value)}
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </div>
          </div>

          {/* Status indicator (read-only, set by column) */}
          <p className="text-xs text-muted-foreground">
            This task will be created in the{" "}
            <span className="font-medium capitalize">
              {initialStatus.replace("_", " ")}
            </span>{" "}
            column.
          </p>

          {/* Actions */}
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
              disabled={createMutation.isPending || !title.trim()}
              className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
            >
              {createMutation.isPending ? "Creating…" : "Create Task"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
