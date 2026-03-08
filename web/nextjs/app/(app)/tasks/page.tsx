"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { getTasks } from "@/lib/api/tasks";
import { taskKeys } from "@/lib/hooks/useTasks";
import type { Task } from "@/lib/types/project";
import type { TaskStatus, Priority } from "@/lib/types/common";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const ALL_STATUSES: TaskStatus[] = [
  "backlog",
  "todo",
  "in_progress",
  "review",
  "done",
];

const STATUS_LABELS: Record<TaskStatus, string> = {
  backlog: "Backlog",
  todo: "To Do",
  in_progress: "In Progress",
  review: "Review",
  done: "Done",
};

const STATUS_BADGE_STYLES: Record<TaskStatus, string> = {
  backlog: "bg-secondary text-secondary-foreground",
  todo: "bg-blue-100 text-blue-700",
  in_progress: "bg-yellow-100 text-yellow-700",
  review: "bg-purple-100 text-purple-700",
  done: "bg-green-100 text-green-700",
};

const PRIORITY_BADGE_STYLES: Record<Priority, string> = {
  low: "bg-secondary text-secondary-foreground",
  medium: "bg-blue-100 text-blue-700",
  high: "bg-orange-100 text-orange-700",
  critical: "bg-red-100 text-red-700",
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatDueDate(isoDate: string | null): string {
  if (!isoDate) return "—";
  return new Date(isoDate).toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    year: "numeric",
  });
}

function isDueOverdue(isoDate: string | null): boolean {
  if (!isoDate) return false;
  return new Date(isoDate) < new Date();
}

// ---------------------------------------------------------------------------
// Loading skeleton
// ---------------------------------------------------------------------------

function TableRowSkeleton() {
  return (
    <tr className="animate-pulse border-b border-border">
      {/* Title */}
      <td className="px-4 py-3">
        <div className="h-4 w-56 rounded bg-muted" />
      </td>
      {/* Status */}
      <td className="px-4 py-3">
        <div className="h-5 w-20 rounded-full bg-muted" />
      </td>
      {/* Priority */}
      <td className="px-4 py-3">
        <div className="h-5 w-16 rounded-full bg-muted" />
      </td>
      {/* Due Date */}
      <td className="px-4 py-3">
        <div className="h-4 w-24 rounded bg-muted" />
      </td>
    </tr>
  );
}

// ---------------------------------------------------------------------------
// Page component
// ---------------------------------------------------------------------------

export default function TasksPage() {
  const [statusFilter, setStatusFilter] = useState<TaskStatus | "all">("all");

  // Fetch all org-wide tasks; no project_id means the backend returns every
  // task the current user has access to across the organisation.
  const { data, isLoading, isError, error } = useQuery({
    queryKey: taskKeys.list({}),
    queryFn: () => getTasks({}),
  });

  const allTasks: Task[] = data?.data ?? [];

  // Client-side status filter applied on top of the full org-wide result set.
  const filteredTasks: Task[] =
    statusFilter === "all"
      ? allTasks
      : allTasks.filter((task) => task.status === statusFilter);

  return (
    <div className="space-y-6">
      {/* Page header */}
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Tasks</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          All tasks across your projects in one place.
        </p>
      </div>

      {/* Status filter bar */}
      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          onClick={() => setStatusFilter("all")}
          className={[
            "inline-flex items-center rounded-full px-3 py-1 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            statusFilter === "all"
              ? "bg-primary text-primary-foreground"
              : "border border-border bg-background text-foreground hover:bg-accent",
          ].join(" ")}
        >
          All
          {/* Show the live total once data has loaded */}
          {!isLoading && (
            <span className="ml-1.5 text-xs opacity-70">{allTasks.length}</span>
          )}
        </button>

        {ALL_STATUSES.map((status) => {
          // Count is derived from the live data; falls back to 0 while loading.
          const count = allTasks.filter((t) => t.status === status).length;
          return (
            <button
              key={status}
              type="button"
              onClick={() => setStatusFilter(status)}
              className={[
                "inline-flex items-center rounded-full px-3 py-1 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                statusFilter === status
                  ? "bg-primary text-primary-foreground"
                  : "border border-border bg-background text-foreground hover:bg-accent",
              ].join(" ")}
            >
              {STATUS_LABELS[status]}
              {!isLoading && (
                <span className="ml-1.5 text-xs opacity-70">{count}</span>
              )}
            </button>
          );
        })}
      </div>

      {/* Error state */}
      {isError && (
        <div className="rounded-xl border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          Failed to load tasks:{" "}
          {error instanceof Error ? error.message : "An unexpected error occurred."}
        </div>
      )}

      {/* Tasks table */}
      <div className="overflow-x-auto rounded-xl border border-border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border bg-muted/30 text-left text-xs font-medium uppercase tracking-wider text-muted-foreground">
              <th className="px-4 py-3">Title</th>
              <th className="px-4 py-3">Status</th>
              <th className="px-4 py-3">Priority</th>
              <th className="px-4 py-3">Due Date</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {/* Loading skeletons – render 6 placeholder rows while fetching */}
            {isLoading &&
              Array.from({ length: 6 }).map((_, index) => (
                <TableRowSkeleton key={index} />
              ))}

            {/* Live task rows */}
            {!isLoading &&
              filteredTasks.map((task) => (
                <tr
                  key={task.id}
                  className="transition-colors hover:bg-muted/20"
                >
                  {/* Title */}
                  <td className="px-4 py-3">
                    <span className="font-medium">{task.title}</span>
                  </td>

                  {/* Status badge */}
                  <td className="px-4 py-3">
                    <span
                      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_BADGE_STYLES[task.status]}`}
                    >
                      {STATUS_LABELS[task.status]}
                    </span>
                  </td>

                  {/* Priority badge */}
                  <td className="px-4 py-3">
                    <span
                      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium capitalize ${PRIORITY_BADGE_STYLES[task.priority]}`}
                    >
                      {task.priority}
                    </span>
                  </td>

                  {/* Due date – highlighted red when overdue and not yet done */}
                  <td
                    className={`px-4 py-3 ${
                      isDueOverdue(task.due_date) && task.status !== "done"
                        ? "font-medium text-red-600"
                        : "text-muted-foreground"
                    }`}
                  >
                    {formatDueDate(task.due_date)}
                  </td>
                </tr>
              ))}

            {/* Empty state – only shown after data has loaded */}
            {!isLoading && !isError && filteredTasks.length === 0 && (
              <tr>
                <td
                  colSpan={4}
                  className="px-4 py-12 text-center text-muted-foreground"
                >
                  No tasks found for the selected status.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Row count – hidden while loading or in error state */}
      {!isLoading && !isError && (
        <p className="text-xs text-muted-foreground">
          Showing {filteredTasks.length} of {allTasks.length} tasks
        </p>
      )}
    </div>
  );
}
