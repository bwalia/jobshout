"use client";

import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import type { Task } from "@/lib/types/project";
import type { Priority } from "@/lib/types/common";

// ---------------------------------------------------------------------------
// Priority colour helpers
// ---------------------------------------------------------------------------

const PRIORITY_DOT_COLOURS: Record<Priority, string> = {
  low: "bg-zinc-400",
  medium: "bg-sky-500",
  high: "bg-orange-500",
  critical: "bg-red-500",
};

const PRIORITY_LABEL_COLOURS: Record<Priority, string> = {
  low: "text-zinc-400",
  medium: "text-sky-500",
  high: "text-orange-500",
  critical: "text-red-500",
};

// ---------------------------------------------------------------------------
// Date formatting
// ---------------------------------------------------------------------------

function formatDueDate(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const isOverdue = date < now;

  const formatted = date.toLocaleDateString("en-GB", {
    day: "2-digit",
    month: "short",
  });

  return isOverdue ? `${formatted} (overdue)` : formatted;
}

function isDueDateOverdue(dateString: string): boolean {
  return new Date(dateString) < new Date();
}

// ---------------------------------------------------------------------------
// TaskCard
// ---------------------------------------------------------------------------

interface TaskCardProps {
  task: Task;
  /** Called when the card body (non-drag area) is clicked to open the detail modal */
  onOpenDetail: (task: Task) => void;
}

/**
 * Draggable task card rendered inside a KanbanColumn.
 *
 * Uses @dnd-kit/sortable's `useSortable` hook so cards can be reordered
 * within a column and moved between columns via drag-and-drop.
 *
 * Displays:
 * - Task title
 * - Priority colour dot + label
 * - Assignee avatar placeholder (initials or generic icon)
 * - Due date (highlighted red when overdue)
 */
export function TaskCard({ task, onOpenDetail }: TaskCardProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: task.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    // Keep space in the column for the ghost card while dragging
    opacity: isDragging ? 0.4 : 1,
  };

  const priorityDot = PRIORITY_DOT_COLOURS[task.priority];
  const priorityLabel = PRIORITY_LABEL_COLOURS[task.priority];
  const overdue = task.due_date ? isDueDateOverdue(task.due_date) : false;

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      className="group relative cursor-grab rounded-lg border border-border bg-card px-4 py-3 shadow-sm transition-shadow hover:shadow-md active:cursor-grabbing"
      onClick={() => onOpenDetail(task)}
    >
      {/* Title */}
      <p className="line-clamp-2 text-sm font-medium leading-snug text-foreground">
        {task.title}
      </p>

      {/* Labels */}
      {task.labels && task.labels.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1">
          {task.labels.map((label) => (
            <span
              key={label.label}
              style={{ backgroundColor: `${label.color}22`, color: label.color }}
              className="rounded-full px-2 py-0.5 text-xs font-medium"
            >
              {label.label}
            </span>
          ))}
        </div>
      )}

      {/* Footer row: priority + assignee + due date */}
      <div className="mt-3 flex items-center justify-between gap-2">
        {/* Priority */}
        <span className={`flex items-center gap-1 text-xs font-medium capitalize ${priorityLabel}`}>
          <span className={`inline-block h-2 w-2 rounded-full ${priorityDot}`} />
          {task.priority}
        </span>

        <div className="flex items-center gap-2">
          {/* Due date */}
          {task.due_date && (
            <span
              className={`flex items-center gap-1 text-xs ${
                overdue ? "text-red-500" : "text-muted-foreground"
              }`}
            >
              <svg
                className="h-3 w-3"
                xmlns="http://www.w3.org/2000/svg"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
              >
                <rect x="3" y="4" width="18" height="18" rx="2" />
                <line x1="16" y1="2" x2="16" y2="6" />
                <line x1="8" y1="2" x2="8" y2="6" />
                <line x1="3" y1="10" x2="21" y2="10" />
              </svg>
              {formatDueDate(task.due_date)}
            </span>
          )}

          {/* Assignee avatar placeholder */}
          {(task.assigned_agent_id || task.assigned_user_id) && (
            <div className="flex h-6 w-6 items-center justify-center rounded-full bg-primary/20 text-xs font-semibold text-primary ring-2 ring-card">
              {/* Show a generic person icon — the real name lookup would require
                  an additional query; keeping this lightweight intentionally */}
              <svg
                className="h-3.5 w-3.5"
                xmlns="http://www.w3.org/2000/svg"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"
                />
                <circle cx="12" cy="7" r="4" />
              </svg>
            </div>
          )}

          {/* Story points */}
          {task.story_points !== null && task.story_points !== undefined && (
            <span className="flex h-5 w-5 items-center justify-center rounded-full bg-muted text-xs font-medium text-muted-foreground">
              {task.story_points}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
