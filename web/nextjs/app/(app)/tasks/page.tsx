"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { getTasks } from "@/lib/api/tasks";
import { getDashboardSummary } from "@/lib/api/metrics";
import { taskKeys } from "@/lib/hooks/useTasks";
import { formatDateOnly, isDueOverdue } from "@/lib/dates";
import { THEME_BADGE } from "@/lib/status-colors";
import type { Task } from "@/lib/types/project";
import type { TaskStatus, Priority } from "@/lib/types/common";

const PAGE_SIZE = 20;

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
  todo: THEME_BADGE.info,
  in_progress: THEME_BADGE.warning,
  review: THEME_BADGE.purple,
  done: THEME_BADGE.success,
};

const PRIORITY_BADGE_STYLES: Record<Priority, string> = {
  low: "bg-secondary text-secondary-foreground",
  medium: THEME_BADGE.info,
  high: THEME_BADGE.orange,
  critical: THEME_BADGE.danger,
};

function TableRowSkeleton() {
  return (
    <tr className="animate-pulse border-b border-border">
      <td className="px-4 py-3">
        <div className="h-4 w-56 rounded bg-muted" />
      </td>
      <td className="px-4 py-3">
        <div className="h-5 w-20 rounded-full bg-muted" />
      </td>
      <td className="px-4 py-3">
        <div className="h-5 w-16 rounded-full bg-muted" />
      </td>
      <td className="px-4 py-3">
        <div className="h-4 w-24 rounded bg-muted" />
      </td>
    </tr>
  );
}

export default function TasksPage() {
  const [statusFilter, setStatusFilter] = useState<TaskStatus | "all">("all");
  const [page, setPage] = useState(1);

  const listParams = {
    page,
    per_page: PAGE_SIZE,
    ...(statusFilter === "all" ? {} : { status: statusFilter }),
  };

  const { data, isLoading, isError, error } = useQuery({
    queryKey: taskKeys.list(listParams),
    queryFn: () => getTasks(listParams),
  });

  const { data: summary } = useQuery({
    queryKey: ["metrics", "summary"],
    queryFn: getDashboardSummary,
  });

  const tasks: Task[] = data?.data ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, data?.total_pages ?? 1);
  const byStatus = summary?.tasks_by_status ?? {};
  const allCount = summary?.total_tasks ?? total;

  function selectStatus(next: TaskStatus | "all") {
    setStatusFilter(next);
    setPage(1);
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Tasks</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          All tasks across your projects in one place.
        </p>
      </div>

      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          onClick={() => selectStatus("all")}
          className={[
            "inline-flex items-center rounded-full px-3 py-1 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
            statusFilter === "all"
              ? "bg-primary text-primary-foreground"
              : "border border-border bg-background text-foreground hover:bg-accent",
          ].join(" ")}
        >
          All
          <span className="ml-1.5 text-xs opacity-70">{allCount}</span>
        </button>

        {ALL_STATUSES.map((status) => {
          const count = byStatus[status] ?? 0;
          return (
            <button
              key={status}
              type="button"
              onClick={() => selectStatus(status)}
              className={[
                "inline-flex items-center rounded-full px-3 py-1 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                statusFilter === status
                  ? "bg-primary text-primary-foreground"
                  : "border border-border bg-background text-foreground hover:bg-accent",
              ].join(" ")}
            >
              {STATUS_LABELS[status]}
              <span className="ml-1.5 text-xs opacity-70">{count}</span>
            </button>
          );
        })}
      </div>

      {isError && (
        <div className="rounded-xl border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          Failed to load tasks:{" "}
          {error instanceof Error ? error.message : "An unexpected error occurred."}
        </div>
      )}

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
            {isLoading &&
              Array.from({ length: 6 }).map((_, index) => (
                <TableRowSkeleton key={index} />
              ))}

            {!isLoading &&
              tasks.map((task) => (
                <tr key={task.id} className="transition-colors hover:bg-muted/20">
                  <td className="px-4 py-3">
                    <span className="font-medium">{task.title}</span>
                  </td>
                  <td className="px-4 py-3">
                    <span
                      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS_BADGE_STYLES[task.status]}`}
                    >
                      {STATUS_LABELS[task.status]}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <span
                      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium capitalize ${PRIORITY_BADGE_STYLES[task.priority]}`}
                    >
                      {task.priority}
                    </span>
                  </td>
                  <td
                    className={`px-4 py-3 ${
                      isDueOverdue(task.due_date) && task.status !== "done"
                        ? "font-medium text-red-600 dark:text-red-400"
                        : "text-muted-foreground"
                    }`}
                  >
                    {task.due_date
                      ? formatDateOnly(task.due_date, {
                          day: "numeric",
                          month: "short",
                          year: "numeric",
                        })
                      : "—"}
                  </td>
                </tr>
              ))}

            {!isLoading && !isError && tasks.length === 0 && (
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

      {!isLoading && !isError && (
        <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
          <p>
            Showing {tasks.length} of {total} tasks
          </p>
          {totalPages > 1 && (
            <div className="flex items-center gap-2">
              <button
                type="button"
                disabled={page <= 1}
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                className="rounded-md border border-border px-2 py-1 hover:bg-accent disabled:opacity-40"
              >
                Previous
              </button>
              <span>
                Page {page} of {totalPages}
              </span>
              <button
                type="button"
                disabled={page >= totalPages}
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                className="rounded-md border border-border px-2 py-1 hover:bg-accent disabled:opacity-40"
              >
                Next
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
