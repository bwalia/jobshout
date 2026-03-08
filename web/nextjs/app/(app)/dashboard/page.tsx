"use client";

import Link from "next/link";
import { StatsCards } from "@/components/dashboard/StatsCards";
import { AgentCard } from "@/components/agent/AgentCard";
import { useAgents } from "@/lib/hooks/useAgents";
import { useTasks } from "@/lib/hooks/useTasks";
import { useProjects } from "@/lib/hooks/useProjects";
import type { Agent } from "@/lib/types/agent";
import type { Task } from "@/lib/types/project";

// Utility: format a status string for display
function formatStatus(status: string): string {
  return status.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

// Priority badge colours
const PRIORITY_CLASSES: Record<string, string> = {
  critical: "bg-red-500/20 text-red-400",
  high: "bg-orange-500/20 text-orange-400",
  medium: "bg-yellow-500/20 text-yellow-400",
  low: "bg-blue-500/20 text-blue-400",
};

function TaskRow({ task }: { task: Task }) {
  const priorityClass =
    PRIORITY_CLASSES[task.priority] ?? "bg-muted text-muted-foreground";

  return (
    <li className="flex items-center justify-between gap-4 rounded-md px-4 py-3 hover:bg-accent/50 transition-colors">
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-foreground">
          {task.title}
        </p>
        <p className="text-xs text-muted-foreground">
          {formatStatus(task.status)}
        </p>
      </div>
      <span
        className={`flex-shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${priorityClass}`}
      >
        {task.priority}
      </span>
    </li>
  );
}

export default function DashboardPage() {
  // Fetch agents with status "active" for the active agents panel
  const {
    data: agentsData,
    isLoading: agentsLoading,
    isError: agentsError,
  } = useAgents({ status: "active", per_page: 6 });

  // Fetch recent tasks (latest 8) for the recent tasks panel
  const {
    data: tasksData,
    isLoading: tasksLoading,
    isError: tasksError,
  } = useTasks({ per_page: 8 });

  // Fetch project count for stats
  const { data: projectsData } = useProjects({ per_page: 1 });

  // All agents (for total count stat)
  const { data: allAgentsData } = useAgents({ per_page: 1 });

  // Tasks assigned today (we use today's tasks using in_progress filter as a proxy)
  const { data: activeTasksData } = useTasks({
    status: "in_progress",
    per_page: 1,
  });

  // Derive average performance score from the visible active agents
  const avgPerformance =
    agentsData && agentsData.data.length > 0
      ? Math.round(
          agentsData.data.reduce(
            (acc: number, agent: Agent) => acc + agent.performance_score,
            0
          ) / agentsData.data.length
        )
      : null;

  const stats = {
    totalAgents: allAgentsData?.total ?? 0,
    activeProjects: projectsData?.total ?? 0,
    tasksToday: activeTasksData?.total ?? 0,
    avgPerformance: avgPerformance,
  };

  const activeAgents: Agent[] = agentsData?.data ?? [];
  const recentTasks: Task[] = tasksData?.data ?? [];

  return (
    <div className="space-y-8">
      {/* Page heading */}
      <div>
        <h1 className="text-2xl font-bold tracking-tight text-foreground">
          Dashboard
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Overview of your AI team activity.
        </p>
      </div>

      {/* Stats row */}
      <StatsCards stats={stats} />

      {/* Active agents grid */}
      <section>
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-foreground">
            Active Agents
          </h2>
          <Link
            href="/agents"
            className="text-sm text-primary hover:underline underline-offset-4"
          >
            View all
          </Link>
        </div>

        {agentsLoading && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <div
                key={i}
                className="h-32 animate-pulse rounded-lg bg-muted"
              />
            ))}
          </div>
        )}

        {agentsError && (
          <p className="text-sm text-muted-foreground">
            Unable to load agents.
          </p>
        )}

        {!agentsLoading && !agentsError && activeAgents.length === 0 && (
          <div className="rounded-lg border border-dashed border-border px-6 py-10 text-center">
            <p className="text-sm text-muted-foreground">
              No active agents right now.{" "}
              <Link href="/agents" className="text-primary hover:underline">
                Create one
              </Link>
            </p>
          </div>
        )}

        {!agentsLoading && !agentsError && activeAgents.length > 0 && (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {activeAgents.map((agent) => (
              <AgentCard key={agent.id} agent={agent} />
            ))}
          </div>
        )}
      </section>

      {/* Recent tasks */}
      <section>
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-foreground">
            Recent Tasks
          </h2>
          <Link
            href="/projects"
            className="text-sm text-primary hover:underline underline-offset-4"
          >
            View projects
          </Link>
        </div>

        <div className="rounded-lg border border-border bg-card">
          {tasksLoading && (
            <div className="space-y-px p-2">
              {Array.from({ length: 5 }).map((_, i) => (
                <div
                  key={i}
                  className="h-14 animate-pulse rounded-md bg-muted"
                />
              ))}
            </div>
          )}

          {tasksError && (
            <p className="px-4 py-6 text-sm text-muted-foreground">
              Unable to load tasks.
            </p>
          )}

          {!tasksLoading && !tasksError && recentTasks.length === 0 && (
            <p className="px-4 py-6 text-sm text-muted-foreground">
              No tasks yet. Start a project to create tasks.
            </p>
          )}

          {!tasksLoading && !tasksError && recentTasks.length > 0 && (
            <ul className="divide-y divide-border">
              {recentTasks.map((task) => (
                <TaskRow key={task.id} task={task} />
              ))}
            </ul>
          )}
        </div>
      </section>
    </div>
  );
}
