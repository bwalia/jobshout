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

/** Status pills, from the theme tokens so both themes follow the palette. */
export const THEME_BADGE = {
  success: "bg-status-done/10 text-status-done",
  warning: "bg-signal-warn/10 text-signal-warn",
  muted: "bg-muted text-muted-foreground",
  info: "bg-status-progress/10 text-status-progress",
  danger: "bg-destructive/10 text-destructive",
  purple: "bg-status-review/10 text-status-review",
  orange: "bg-gold/15 text-[#6E5418] dark:text-gold",
} as const;

/** One priority palette, readable (AA) in both themes. Medium is blue so it
 *  never reads as High. */
export const PRIORITY_BADGE: Record<string, string> = {
  low: THEME_BADGE.muted,
  medium: THEME_BADGE.info,
  high: THEME_BADGE.orange,
  critical: THEME_BADGE.danger,
};
