import type { Priority, TaskStatus } from "@/lib/types/common";

/**
 * The one place task status/priority display names live. Several surfaces
 * used to render raw snake_case ("in progress", "low") next to properly
 * labelled siblings.
 */
export const STATUS_OPTIONS: { value: TaskStatus; label: string }[] = [
  { value: "backlog", label: "Backlog" },
  { value: "todo", label: "Todo" },
  { value: "in_progress", label: "In Progress" },
  { value: "review", label: "Review" },
  { value: "done", label: "Done" },
];

export const PRIORITY_OPTIONS: { value: Priority; label: string }[] = [
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
  { value: "critical", label: "Critical" },
];

export function statusLabel(status: string): string {
  return (
    STATUS_OPTIONS.find((s) => s.value === status)?.label ??
    status.replace(/_/g, " ")
  );
}

export function priorityLabel(priority: string): string {
  return PRIORITY_OPTIONS.find((p) => p.value === priority)?.label ?? priority;
}
