"use client";

import { History, Rocket } from "lucide-react";
import type { Task } from "@/lib/types/project";
import { cn } from "@/lib/utils/cn";

export function canRunTask(task: Task): boolean {
  return task.status !== "done";
}

export function TaskActions({
  task,
  onRun,
  onShowHistory,
  size = "md",
  className,
}: {
  task: Task;
  onRun?: () => void;
  onShowHistory: () => void;
  size?: "sm" | "md";
  className?: string;
}) {
  const compact = size === "sm";
  const secondary = compact
    ? "inline-flex items-center gap-1 rounded-md border border-border px-2 py-1 text-xs hover:bg-secondary"
    : "inline-flex h-9 items-center gap-1.5 rounded-md border border-border px-3 text-sm hover:bg-secondary";
  const primary = compact
    ? "inline-flex items-center gap-1 rounded-md bg-primary px-2 py-1 text-xs font-medium text-primary-foreground"
    : "inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-3 text-sm font-medium text-primary-foreground hover:opacity-90";

  return (
    <div className={cn("flex shrink-0 items-center gap-2", className)}>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          onShowHistory();
        }}
        className={secondary}
      >
        <History className={compact ? "h-3 w-3" : "h-4 w-4"} />
        Show History
      </button>
      {canRunTask(task) && onRun ? (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            onRun();
          }}
          className={primary}
        >
          <Rocket className={compact ? "h-3 w-3" : "h-4 w-4"} />
          Run
        </button>
      ) : null}
    </div>
  );
}
