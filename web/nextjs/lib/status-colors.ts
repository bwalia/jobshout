import type { TaskStatus } from "@/lib/types/common";

/** Shared status-dot classes so Dashboard, Task Board, and Agent Board agree. */
export const STATUS_DOT: Record<TaskStatus, string> = {
  backlog: "bg-status-idle",
  todo: "bg-status-todo",
  in_progress: "bg-status-progress",
  review: "bg-status-review",
  done: "bg-status-done",
};

export const STATUS_DOT_HSL: Record<TaskStatus, string> = {
  backlog: "hsl(var(--status-idle))",
  todo: "hsl(var(--status-todo))",
  in_progress: "hsl(var(--status-progress))",
  review: "hsl(var(--status-review))",
  done: "hsl(var(--status-done))",
};

/** Light + dark badge pairs for status pills that used to be one-theme-only. */
export const THEME_BADGE = {
  success:
    "bg-emerald-100 text-emerald-800 dark:bg-emerald-500/20 dark:text-emerald-400",
  warning:
    "bg-amber-100 text-amber-800 dark:bg-amber-500/20 dark:text-amber-400",
  muted: "bg-zinc-100 text-zinc-700 dark:bg-zinc-500/20 dark:text-zinc-400",
  info: "bg-blue-100 text-blue-800 dark:bg-blue-500/20 dark:text-blue-400",
  danger: "bg-red-100 text-red-800 dark:bg-red-500/20 dark:text-red-400",
  purple:
    "bg-violet-100 text-violet-800 dark:bg-violet-500/20 dark:text-violet-400",
  orange:
    "bg-orange-100 text-orange-800 dark:bg-orange-500/20 dark:text-orange-400",
} as const;
