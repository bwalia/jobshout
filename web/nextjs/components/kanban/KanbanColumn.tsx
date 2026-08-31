"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { useDroppable } from "@dnd-kit/core";
import { SortableContext, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { TaskCard } from "@/components/kanban/TaskCard";
import { CreateTaskDialog } from "@/components/kanban/CreateTaskDialog";
import { TaskDetailModal } from "@/components/kanban/TaskDetailModal";
import { cn } from "@/lib/utils/cn";
import type { Task } from "@/lib/types/project";
import type { TaskStatus } from "@/lib/types/common";

const COLUMN_DOT: Record<TaskStatus, string> = {
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

interface KanbanColumnProps {
  status: TaskStatus;
  tasks: Task[];
  projectId: string;
  assigneeNames?: Map<string, string>;
  isDragging?: boolean;
}

export function KanbanColumn({
  status,
  tasks,
  projectId,
  assigneeNames,
  isDragging,
}: KanbanColumnProps) {
  const [showCreateDialog, setShowCreateDialog] = useState(false);
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const { setNodeRef, isOver } = useDroppable({ id: status });
  const taskIds = tasks.map((t) => t.id);
  const label = COLUMN_LABELS[status];

  function openCreate() {
    setShowCreateDialog(true);
  }

  return (
    <>
      <div
        className={cn(
          "flex max-h-full w-[85vw] max-w-[22rem] shrink-0 snap-center flex-col rounded-lg bg-muted transition-all duration-200",
          "md:w-72 md:min-w-72 md:max-w-none",
          isOver && "bg-primary/10 shadow-lg ring-2 ring-primary",
          isDragging && !isOver && "ring-1 ring-primary/20"
        )}
      >
        <div className="flex items-center justify-between rounded-t-lg border-b border-border/70 bg-muted/80 px-3 py-2">
          <div className="flex min-w-0 items-center gap-2">
            <span className={cn("h-3 w-3 shrink-0 rounded-full", COLUMN_DOT[status])} />
            <h3 className="truncate text-sm font-semibold text-foreground">{label}</h3>
            <span
              className={cn(
                "shrink-0 rounded-full px-2 py-0.5 text-xs font-medium",
                "bg-background/80 text-muted-foreground"
              )}
            >
              {tasks.length}
            </span>
            {status === "done" && (
              <span className="rounded bg-emerald-100 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-500/20 dark:text-emerald-400">
                Done
              </span>
            )}
          </div>
          <button
            type="button"
            onClick={openCreate}
            className="rounded p-1 text-muted-foreground hover:bg-background/80 hover:text-foreground"
            title="Add task"
            aria-label="Add task"
          >
            <Plus className="h-4 w-4" />
          </button>
        </div>

        <div
          ref={setNodeRef}
          className={cn(
            "min-h-[120px] flex-1 space-y-2 overflow-y-auto p-2 transition-colors",
            isOver && "bg-primary/5"
          )}
        >
          <SortableContext items={taskIds} strategy={verticalListSortingStrategy}>
            {tasks.map((task) => (
              <TaskCard
                key={task.id}
                task={task}
                assigneeName={
                  task.assigned_agent_id
                    ? assigneeNames?.get(task.assigned_agent_id)
                    : undefined
                }
                onOpenDetail={setSelectedTask}
              />
            ))}
          </SortableContext>

          {tasks.length === 0 && (
            <div
              className={cn(
                "rounded-lg border-2 border-dashed py-8 text-center text-sm transition-all",
                isOver
                  ? "scale-[1.02] border-primary bg-primary/10 font-medium text-primary"
                  : isDragging
                    ? "border-primary/30 bg-primary/5 text-primary/70"
                    : "border-border text-muted-foreground"
              )}
            >
              {isOver
                ? "Drop task here"
                : isDragging
                  ? "Drag here to move"
                  : "No tasks yet"}
            </div>
          )}

          {tasks.length > 0 && isOver && (
            <div className="my-1 h-1 animate-pulse rounded-full bg-primary" />
          )}
        </div>

        <button
          type="button"
          onClick={openCreate}
          disabled={isDragging}
          className={cn(
            "flex items-center gap-2 rounded-b-lg px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-background/60 hover:text-foreground",
            isDragging && "pointer-events-none opacity-50"
          )}
        >
          <Plus className="h-4 w-4" />
          Add a task
        </button>
      </div>

      {showCreateDialog && (
        <CreateTaskDialog
          projectId={projectId}
          initialStatus={status}
          onClose={() => setShowCreateDialog(false)}
        />
      )}

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
