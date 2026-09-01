"use client";

import { useState, useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { updateTask, getTaskComments, addTaskComment } from "@/lib/api/tasks";
import { useAgents } from "@/lib/hooks/useAgents";
import { taskKeys } from "@/lib/hooks/useTasks";
import { useAuthStore } from "@/lib/store/auth-store";
import { PRIORITY_OPTIONS, STATUS_OPTIONS } from "@/lib/task-labels";
import type { Task, UpdateTaskRequest, TaskComment } from "@/lib/types/project";
import type { TaskStatus, Priority } from "@/lib/types/common";
import type { LaunchResult } from "@/lib/agents/launch";
import type { TaskRun } from "@/lib/types/task-run";
import { TaskActions } from "@/components/task-manager/TaskActions";
import { TaskHistoryPanel } from "@/components/task-manager/TaskHistoryPanel";
import { RunTaskDialog } from "@/components/task-manager/RunTaskDialog";
import { formatCompletedAt } from "@/components/task-manager/TaskProgressChip";

// ---------------------------------------------------------------------------
// TaskDetailModal
// ---------------------------------------------------------------------------

interface TaskDetailModalProps {
  task: Task;
  /** Called when the modal should close (cancel or after save) */
  onClose: () => void;
  /** Called after a successful save so the parent can update its list */
  onUpdated?: (updatedTask: Task) => void;
  initialFocusRunId?: string | null;
}

// ---------------------------------------------------------------------------
// Relative time helper
// ---------------------------------------------------------------------------

function formatRelativeTime(isoTimestamp: string): string {
  const diffMs = Date.now() - new Date(isoTimestamp).getTime();
  const diffSeconds = Math.floor(diffMs / 1000);

  if (diffSeconds < 60) return "just now";
  if (diffSeconds < 3600) return `${Math.floor(diffSeconds / 60)}m ago`;
  if (diffSeconds < 86_400) return `${Math.floor(diffSeconds / 3600)}h ago`;
  return `${Math.floor(diffSeconds / 86_400)}d ago`;
}

export function TaskDetailModal({
  task,
  onClose,
  onUpdated,
  initialFocusRunId,
}: TaskDetailModalProps) {
  const queryClient = useQueryClient();
  const { data: agentsResp } = useAgents({ per_page: 100 });
  const agents = agentsResp?.data ?? [];
  const currentUserId = useAuthStore((s) => s.user?.id ?? null);

  // Comments only carry ids; resolve what we can rather than rendering every
  // comment anonymously.
  function commentAuthor(comment: TaskComment): string {
    if (comment.agent_id) {
      return agents.find((a) => a.id === comment.agent_id)?.name ?? "Agent";
    }
    if (comment.author_id && comment.author_id === currentUserId) return "You";
    return "Team member";
  }

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

  // Comment input state
  const [commentBody, setCommentBody] = useState("");
  const [runOpen, setRunOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(Boolean(initialFocusRunId));
  const [focusRunId, setFocusRunId] = useState<string | null>(
    initialFocusRunId ?? null
  );

  useEffect(() => {
    if (!initialFocusRunId) return;
    setFocusRunId(initialFocusRunId);
    setHistoryOpen(true);
  }, [task.id, initialFocusRunId]);

  // Keep local state fresh if the task prop changes (e.g. external update)
  useEffect(() => {
    setTitle(task.title);
    setDescription(task.description ?? "");
    setStatus(task.status);
    setPriority(task.priority);
    setAssignedAgentId(task.assigned_agent_id ?? "");
    setDueDate(task.due_date ? task.due_date.slice(0, 10) : "");
  }, [task]);

  // Fetch comments for this task
  const {
    data: comments = [],
    isLoading: commentsLoading,
  } = useQuery<TaskComment[]>({
    queryKey: ["taskComments", task.id],
    queryFn: () => getTaskComments(task.id),
  });

  const updateMutation = useMutation({
    mutationFn: (payload: UpdateTaskRequest) => updateTask(task.id, payload),
    onSuccess: (updatedTask) => {
      // This modal is opened from both the per-project board and the
      // all-projects Task Board (a flat list under a different key), so
      // invalidate every task query rather than guessing which one fed it.
      queryClient.invalidateQueries({ queryKey: taskKeys.all });
      toast.success("Task updated.");
      onUpdated?.(updatedTask);
      onClose();
    },
    onError: (error: Error) => {
      toast.error(`Failed to update task: ${error.message}`);
    },
  });

  const addCommentMutation = useMutation({
    mutationFn: (body: string) => addTaskComment(task.id, body),
    onSuccess: () => {
      // Refresh the comments list after a successful add
      queryClient.invalidateQueries({ queryKey: ["taskComments", task.id] });
      setCommentBody("");
      toast.success("Comment added.");
    },
    onError: (error: Error) => {
      toast.error(`Failed to add comment: ${error.message}`);
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

  function handleAddComment() {
    const trimmed = commentBody.trim();
    if (!trimmed) return;
    addCommentMutation.mutate(trimmed);
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

          {/* Assignee picker */}
          <div className="space-y-1.5">
            <label htmlFor="task-assignee" className="text-sm font-medium">
              Assigned Agent
            </label>
            <select
              id="task-assignee"
              value={assignedAgentId}
              onChange={(e) => setAssignedAgentId(e.target.value)}
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <option value="">Unassigned</option>
              {/* Keep an unknown current assignee selectable rather than
                  silently rendering a blank control. */}
              {assignedAgentId &&
                !agents.some((a) => a.id === assignedAgentId) && (
                  <option value={assignedAgentId}>
                    Unknown agent ({assignedAgentId.slice(0, 8)}…)
                  </option>
                )}
              {agents.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </div>

          {formatCompletedAt(task.completed_at) && (
            <p className="text-sm text-muted-foreground">
              Completed {formatCompletedAt(task.completed_at)}
            </p>
          )}

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

          {/* Comments section */}
          <div className="space-y-3 border-t border-border pt-5">
            <h3 className="text-sm font-semibold">Comments</h3>

            {/* Comment list */}
            {commentsLoading ? (
              <div className="space-y-2">
                {Array.from({ length: 2 }).map((_, i) => (
                  <div
                    key={i}
                    className="h-10 animate-pulse rounded-md bg-muted"
                  />
                ))}
              </div>
            ) : comments.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No comments yet. Be the first to add one.
              </p>
            ) : (
              <ul className="space-y-3">
                {comments.map((comment) => (
                  <li
                    key={comment.id}
                    className="rounded-md border border-border bg-muted/30 px-3 py-2"
                  >
                    <p className="text-sm text-foreground whitespace-pre-wrap">
                      {comment.body}
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {commentAuthor(comment)} ·{" "}
                      {formatRelativeTime(comment.created_at)}
                    </p>
                  </li>
                ))}
              </ul>
            )}

            {/* Add comment input */}
            <div className="space-y-2">
              <textarea
                rows={3}
                value={commentBody}
                onChange={(e) => setCommentBody(e.target.value)}
                placeholder="Write a comment…"
                className="flex w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
              <button
                type="button"
                onClick={handleAddComment}
                disabled={
                  !commentBody.trim() || addCommentMutation.isPending
                }
                className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
              >
                {addCommentMutation.isPending ? "Adding…" : "Add Comment"}
              </button>
            </div>
          </div>
        </div>

        {/* Footer actions */}
        <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border px-6 py-4">
          <TaskActions
            task={task}
            onShowHistory={() => setHistoryOpen(true)}
            onRun={() => setRunOpen(true)}
          />
          <div className="flex gap-2">
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
      {runOpen && (
        <RunTaskDialog
          task={task}
          agents={agents}
          onClose={() => setRunOpen(false)}
          onLaunched={(run: TaskRun) => {
            setFocusRunId(run.id);
            setHistoryOpen(true);
            void queryClient.invalidateQueries({ queryKey: taskKeys.all });
          }}
          onSpecialistLaunched={(result: LaunchResult) => {
            if (result.run_id) setFocusRunId(result.run_id);
            setHistoryOpen(true);
            void queryClient.invalidateQueries({ queryKey: taskKeys.all });
            if (result.task) onUpdated?.(result.task);
          }}
        />
      )}
      {historyOpen && (
        <TaskHistoryPanel
          task={task}
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
