"use client";

import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  Calendar,
  CheckSquare,
  Circle,
  Minus,
  User,
} from "lucide-react";
import { formatDateOnly, isDueOverdue } from "@/lib/dates";
import { cn } from "@/lib/utils/cn";
import type { Priority } from "@/lib/types/common";
import type { Task } from "@/lib/types/project";

function PriorityIcon({ priority }: { priority: Priority }) {
  const className = "h-3.5 w-3.5";
  switch (priority) {
    case "critical":
      return <AlertTriangle className={cn(className, "text-red-500")} />;
    case "high":
      return <ArrowUp className={cn(className, "text-orange-500")} />;
    case "medium":
      return <Minus className={cn(className, "text-amber-500")} />;
    case "low":
      return <ArrowDown className={cn(className, "text-blue-500")} />;
    default:
      return <Circle className={cn(className, "text-muted-foreground")} />;
  }
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return `${parts[0][0]}${parts[1][0]}`.toUpperCase();
}

function DueBadge({ due, done }: { due: string; done: boolean }) {
  const overdue = !done && isDueOverdue(due);
  const formatted = formatDateOnly(due, { month: "short", day: "numeric" });
  return (
    <div
      className={cn(
        "mt-2 flex items-center gap-1 text-xs",
        overdue && "text-red-600 dark:text-red-400",
        done && "text-muted-foreground line-through",
        !overdue && !done && "text-muted-foreground"
      )}
    >
      <Calendar className="h-3 w-3" />
      <span>{formatted}</span>
      {overdue && <span className="font-medium">(Overdue)</span>}
    </div>
  );
}

export function TaskCardFace({
  task,
  assigneeName,
  isDragging,
  isOverlay,
  onOpenDetail,
}: {
  task: Task;
  assigneeName?: string;
  isDragging?: boolean;
  isOverlay?: boolean;
  onOpenDetail?: (task: Task) => void;
}) {
  const overdue = Boolean(task.due_date && isDueOverdue(task.due_date) && task.status !== "done");
  const labels = task.labels?.slice(0, 3) ?? [];
  const extraLabels = (task.labels?.length ?? 0) - labels.length;

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={() => {
        if (isDragging) return;
        onOpenDetail?.(task);
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          if (!isDragging) onOpenDetail?.(task);
        }
      }}
      className={cn(
        "relative rounded-lg border border-border bg-card p-3 shadow-sm transition-all duration-200",
        "hover:border-primary/30 hover:shadow-md",
        "focus:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        isDragging && "scale-[0.98] border-primary/40 opacity-40 shadow-lg",
        isOverlay && "rotate-2 scale-105 border-primary/50 shadow-2xl ring-2 ring-primary/20",
        overdue && "border-l-4 border-l-red-500"
      )}
    >
      {labels.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1">
          {labels.map((label) => (
            <span
              key={label.label}
              className="inline-block rounded-full px-2 py-0.5 text-[11px] font-medium"
              style={{
                backgroundColor: `${label.color}20`,
                color: label.color,
              }}
            >
              {label.label}
            </span>
          ))}
          {extraLabels > 0 && (
            <span className="inline-block rounded-full bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
              +{extraLabels}
            </span>
          )}
        </div>
      )}

      <h4 className="line-clamp-2 text-sm font-medium text-foreground">
        {task.title}
      </h4>

      <div className="mt-2 flex items-center justify-between">
        <span className="font-mono text-[11px] text-muted-foreground">
          #{task.id.slice(0, 8)}
        </span>
        <PriorityIcon priority={task.priority} />
      </div>

      {task.due_date && (
        <DueBadge due={task.due_date} done={task.status === "done"} />
      )}

      {typeof task.subtask_count === "number" && task.subtask_count > 0 && (
        <div className="mt-2 flex items-center gap-1 text-xs text-muted-foreground">
          <CheckSquare className="h-3 w-3" />
          {task.subtask_count}
        </div>
      )}

      <div className="mt-3 flex items-center justify-between border-t border-border pt-2">
        {assigneeName ? (
          <div
            className="flex h-6 w-6 items-center justify-center rounded-full border-2 border-card bg-primary/15 text-[10px] font-semibold text-primary"
            title={assigneeName}
          >
            {initials(assigneeName)}
          </div>
        ) : (
          <User className="h-3.5 w-3.5 text-muted-foreground/60" />
        )}
        {task.story_points != null && task.story_points > 0 && (
          <span className="rounded bg-muted px-2 py-0.5 text-[11px] font-medium text-muted-foreground">
            {task.story_points} pts
          </span>
        )}
      </div>
    </div>
  );
}

interface TaskCardProps {
  task: Task;
  assigneeName?: string;
  onOpenDetail: (task: Task) => void;
  isOverlay?: boolean;
}

export function TaskCard({
  task,
  assigneeName,
  onOpenDetail,
  isOverlay,
}: TaskCardProps) {
  if (isOverlay) {
    return (
      <TaskCardFace
        task={task}
        assigneeName={assigneeName}
        isOverlay
      />
    );
  }

  return (
    <SortableTaskCard
      task={task}
      assigneeName={assigneeName}
      onOpenDetail={onOpenDetail}
    />
  );
}

function SortableTaskCard({
  task,
  assigneeName,
  onOpenDetail,
}: {
  task: Task;
  assigneeName?: string;
  onOpenDetail: (task: Task) => void;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: task.id });

  return (
    <div
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
      }}
      className={cn("touch-none", isDragging && "z-50")}
      {...attributes}
      {...listeners}
    >
      <TaskCardFace
        task={task}
        assigneeName={assigneeName}
        isDragging={isDragging}
        onOpenDetail={onOpenDetail}
      />
    </div>
  );
}
