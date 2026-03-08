"use client";

import { useState } from "react";
import { useDroppable } from "@dnd-kit/core";
import { SortableContext, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { TaskCard } from "@/components/kanban/TaskCard";
import { CreateTaskDialog } from "@/components/kanban/CreateTaskDialog";
import { TaskDetailModal } from "@/components/kanban/TaskDetailModal";
import type { Task } from "@/lib/types/project";
import type { TaskStatus } from "@/lib/types/common";

// ---------------------------------------------------------------------------
// Column colour theming
// ---------------------------------------------------------------------------

const COLUMN_ACCENT_COLOURS: Record<TaskStatus, string> = {
  backlog: "border-zinc-500/40 bg-zinc-500/10",
  todo: "border-sky-500/40 bg-sky-500/10",
  in_progress: "border-blue-500/40 bg-blue-500/10",
  review: "border-violet-500/40 bg-violet-500/10",
  done: "border-emerald-500/40 bg-emerald-500/10",
};

const COLUMN_DOT_COLOURS: Record<TaskStatus, string> = {
  backlog: "bg-zinc-400",
  todo: "bg-sky-500",
  in_progress: "bg-blue-500",
  review: "bg-violet-500",
  done: "bg-emerald-500",
};

const COLUMN_LABELS: Record<TaskStatus, string> = {
  backlog: "Backlog",
  todo: "Todo",
  in_progress: "In Progress",
  review: "Review",
  done: "Done",
};

// ---------------------------------------------------------------------------
// KanbanColumn
// ---------------------------------------------------------------------------

interface KanbanColumnProps {
  status: TaskStatus;
  tasks: Task[];
  projectId: string;
}

/**
 * A single Kanban column.
 *
 * Features:
 * - Droppable area powered by @dnd-kit/core's `useDroppable`
 * - Sortable task list via SortableContext
 * - Inline "Add task" button that opens CreateTaskDialog
 * - Task card click opens TaskDetailModal
 */
export function KanbanColumn({ status, tasks, projectId }: KanbanColumnProps) {
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);

  // Make this column a valid drop target for dragged cards
  const { setNodeRef, isOver } = useDroppable({ id: status });

  const taskIds = tasks.map((t) => t.id);
  const accentClass = COLUMN_ACCENT_COLOURS[status];
  const dotClass = COLUMN_DOT_COLOURS[status];
  const label = COLUMN_LABELS[status];

  return (
    <>
      <div className="flex w-72 shrink-0 flex-col rounded-xl border border-border bg-background">
        {/* Column header */}
        <div
          className={`flex items-center justify-between rounded-t-xl border-b px-4 py-3 ${accentClass}`}
        >
          <div className="flex items-center gap-2">
            <span className={`h-2.5 w-2.5 rounded-full ${dotClass}`} />
            <span className="text-sm font-semibold">{label}</span>
          </div>
          {/* Task count badge */}
          <span className="inline-flex h-5 min-w-[1.25rem] items-center justify-center rounded-full bg-muted px-1.5 text-xs font-medium text-muted-foreground">
            {tasks.length}
          </span>
        </div>

        {/* Droppable + sortable task list */}
        <div
          ref={setNodeRef}
          className={`flex min-h-[4rem] flex-1 flex-col gap-2 overflow-y-auto p-3 transition-colors ${
            isOver ? "bg-accent/40" : ""
          }`}
        >
          <SortableContext items={taskIds} strategy={verticalListSortingStrategy}>
            {tasks.map((task) => (
              <TaskCard
                key={task.id}
                task={task}
                onOpenDetail={setSelectedTask}
              />
            ))}
          </SortableContext>

          {tasks.length === 0 && !isOver && (
            <p className="py-4 text-center text-xs text-muted-foreground">
              No tasks
            </p>
          )}
        </div>

        {/* Add task button */}
        <div className="border-t border-border p-3">
          <button
            type="button"
            onClick={() => setShowCreateDialog(true)}
            className="flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            <svg
              className="h-4 w-4"
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            Add task
          </button>
        </div>
      </div>

      {/* Create task dialog — rendered in a portal-like pattern via fixed positioning */}
      {showCreateDialog && (
        <CreateTaskDialog
          projectId={projectId}
          initialStatus={status}
          onClose={() => setShowCreateDialog(false)}
        />
      )}

      {/* Task detail modal */}
      {selectedTask && (
        <TaskDetailModal
          task={selectedTask}
          onClose={() => setSelectedTask(null)}
          onUpdated={(updatedTask) => setSelectedTask(updatedTask)}
        />
      )}
    </>
  );
}
