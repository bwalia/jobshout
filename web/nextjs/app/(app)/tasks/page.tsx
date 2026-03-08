"use client";

import { useState } from "react";
import type { Task, TaskStatus, Priority } from "@/lib/types/project";

// Mock task data covering multiple projects and a variety of statuses/priorities
const MOCK_TASKS: Array<
  Task & { project_name: string; assignee_name: string | null }
> = [
  {
    id: "t1",
    project_id: "p1",
    project_name: "Website Redesign",
    parent_id: null,
    title: "Implement new homepage hero section",
    description: null,
    status: "in_progress",
    priority: "high",
    assigned_agent_id: null,
    assigned_user_id: "u1",
    assignee_name: "TypeScript Mentor",
    story_points: 5,
    due_date: new Date(Date.now() + 2 * 86_400_000).toISOString(),
    position: 1,
    created_by: null,
    created_at: new Date(Date.now() - 3 * 86_400_000).toISOString(),
    updated_at: new Date(Date.now() - 86_400_000).toISOString(),
  },
  {
    id: "t2",
    project_id: "p1",
    project_name: "Website Redesign",
    parent_id: null,
    title: "Write end-to-end tests for checkout flow",
    description: null,
    status: "todo",
    priority: "medium",
    assigned_agent_id: "a1",
    assigned_user_id: null,
    assignee_name: "QA Sentinel",
    story_points: 3,
    due_date: new Date(Date.now() + 5 * 86_400_000).toISOString(),
    position: 2,
    created_by: null,
    created_at: new Date(Date.now() - 2 * 86_400_000).toISOString(),
    updated_at: new Date(Date.now() - 2 * 86_400_000).toISOString(),
  },
  {
    id: "t3",
    project_id: "p2",
    project_name: "API v2 Migration",
    parent_id: null,
    title: "Migrate auth endpoints to v2 schema",
    description: null,
    status: "review",
    priority: "critical",
    assigned_agent_id: "a2",
    assigned_user_id: null,
    assignee_name: "CodeReviewer Pro",
    story_points: 8,
    due_date: new Date(Date.now() + 86_400_000).toISOString(),
    position: 1,
    created_by: null,
    created_at: new Date(Date.now() - 7 * 86_400_000).toISOString(),
    updated_at: new Date(Date.now() - 86_400_000).toISOString(),
  },
  {
    id: "t4",
    project_id: "p2",
    project_name: "API v2 Migration",
    parent_id: null,
    title: "Update SDK documentation",
    description: null,
    status: "backlog",
    priority: "low",
    assigned_agent_id: null,
    assigned_user_id: null,
    assignee_name: null,
    story_points: 2,
    due_date: null,
    position: 2,
    created_by: null,
    created_at: new Date(Date.now() - 10 * 86_400_000).toISOString(),
    updated_at: new Date(Date.now() - 10 * 86_400_000).toISOString(),
  },
  {
    id: "t5",
    project_id: "p3",
    project_name: "Infrastructure Hardening",
    parent_id: null,
    title: "Configure Kubernetes resource limits",
    description: null,
    status: "done",
    priority: "high",
    assigned_agent_id: "a3",
    assigned_user_id: null,
    assignee_name: "K8s Guardian",
    story_points: 5,
    due_date: null,
    position: 1,
    created_by: null,
    created_at: new Date(Date.now() - 14 * 86_400_000).toISOString(),
    updated_at: new Date(Date.now() - 2 * 86_400_000).toISOString(),
  },
  {
    id: "t6",
    project_id: "p3",
    project_name: "Infrastructure Hardening",
    parent_id: null,
    title: "Set up CI/CD pipeline optimisation",
    description: null,
    status: "in_progress",
    priority: "medium",
    assigned_agent_id: "a4",
    assigned_user_id: null,
    assignee_name: "CI Optimizer",
    story_points: 3,
    due_date: new Date(Date.now() + 3 * 86_400_000).toISOString(),
    position: 2,
    created_by: null,
    created_at: new Date(Date.now() - 4 * 86_400_000).toISOString(),
    updated_at: new Date(Date.now() - 86_400_000).toISOString(),
  },
];

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

export default function TasksPage() {
  const [statusFilter, setStatusFilter] = useState<TaskStatus | "all">("all");

  const filteredTasks =
    statusFilter === "all"
      ? MOCK_TASKS
      : MOCK_TASKS.filter((t) => t.status === statusFilter);

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
          <span className="ml-1.5 text-xs opacity-70">{MOCK_TASKS.length}</span>
        </button>

        {ALL_STATUSES.map((status) => {
          const count = MOCK_TASKS.filter((t) => t.status === status).length;
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
              <span className="ml-1.5 text-xs opacity-70">{count}</span>
            </button>
          );
        })}
      </div>

      {/* Tasks table */}
      <div className="overflow-x-auto rounded-xl border border-border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border bg-muted/30 text-left text-xs font-medium uppercase tracking-wider text-muted-foreground">
              <th className="px-4 py-3">Title</th>
              <th className="px-4 py-3">Project</th>
              <th className="px-4 py-3">Status</th>
              <th className="px-4 py-3">Priority</th>
              <th className="px-4 py-3">Assignee</th>
              <th className="px-4 py-3">Due Date</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {filteredTasks.map((task) => (
              <tr
                key={task.id}
                className="transition-colors hover:bg-muted/20"
              >
                {/* Title */}
                <td className="px-4 py-3">
                  <span className="font-medium">{task.title}</span>
                </td>

                {/* Project */}
                <td className="px-4 py-3 text-muted-foreground">
                  {task.project_name}
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

                {/* Assignee */}
                <td className="px-4 py-3 text-muted-foreground">
                  {task.assignee_name ?? (
                    <span className="italic">Unassigned</span>
                  )}
                </td>

                {/* Due date - highlighted red when overdue */}
                <td
                  className={`px-4 py-3 ${isDueOverdue(task.due_date) && task.status !== "done" ? "font-medium text-red-600" : "text-muted-foreground"}`}
                >
                  {formatDueDate(task.due_date)}
                </td>
              </tr>
            ))}

            {filteredTasks.length === 0 && (
              <tr>
                <td
                  colSpan={6}
                  className="px-4 py-12 text-center text-muted-foreground"
                >
                  No tasks found for the selected status.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* Row count */}
      <p className="text-xs text-muted-foreground">
        Showing {filteredTasks.length} of {MOCK_TASKS.length} tasks
      </p>
    </div>
  );
}
