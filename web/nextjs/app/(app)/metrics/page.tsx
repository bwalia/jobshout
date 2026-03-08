"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AgentUtilizationChart } from "@/components/dashboard/AgentUtilizationChart";
import type { AgentUtilizationDataPoint } from "@/components/dashboard/AgentUtilizationChart";
import { TaskCompletionChart } from "@/components/dashboard/TaskCompletionChart";
import type { TaskCompletionDataPoint } from "@/components/dashboard/TaskCompletionChart";
import { MetricCard } from "@/components/dashboard/MetricCard";
import { getDashboardSummary, getTaskCompletion } from "@/lib/api/metrics";
import { getAgents } from "@/lib/api/agents";
import type { Agent } from "@/lib/types/agent";

type DateRange = "7d" | "30d" | "90d";

const DATE_RANGE_OPTIONS: { label: string; value: DateRange }[] = [
  { label: "7 days", value: "7d" },
  { label: "30 days", value: "30d" },
  { label: "90 days", value: "90d" },
];

/** Maps the UI date-range token to a numeric day count for the API. */
const DATE_RANGE_DAYS: Record<DateRange, number> = {
  "7d": 7,
  "30d": 30,
  "90d": 90,
};

// ---------------------------------------------------------------------------
// Loading skeleton helpers
// ---------------------------------------------------------------------------

function MetricCardSkeleton() {
  return (
    <div className="rounded-xl border border-border bg-card p-5 shadow-sm animate-pulse">
      <div className="h-4 w-24 rounded bg-muted" />
      <div className="mt-2 h-9 w-20 rounded bg-muted" />
      <div className="mt-2 h-5 w-28 rounded bg-muted" />
    </div>
  );
}

function ChartSkeleton() {
  return (
    <div className="h-[260px] w-full animate-pulse rounded-lg bg-muted" />
  );
}

// ---------------------------------------------------------------------------
// Page component
// ---------------------------------------------------------------------------

export default function MetricsPage() {
  const [dateRange, setDateRange] = useState<DateRange>("7d");
  const days = DATE_RANGE_DAYS[dateRange];

  // Fetch dashboard summary metrics (does not vary by date range on the API,
  // but we show values from the single summary response for all ranges).
  const {
    data: summary,
    isLoading: summaryLoading,
    isError: summaryError,
  } = useQuery({
    queryKey: ["metrics", "summary"],
    queryFn: getDashboardSummary,
  });

  // Fetch task completion trend keyed by the current date range so React
  // Query re-fetches automatically when the user switches ranges.
  const {
    data: taskCompletionRaw,
    isLoading: taskCompletionLoading,
    isError: taskCompletionError,
  } = useQuery({
    queryKey: ["metrics", "task-completion", days],
    queryFn: () => getTaskCompletion(days),
  });

  // Fetch all agents (up to 100) so we can derive per-agent utilization from
  // performance_score. The agents list does not vary by date range.
  const {
    data: agentsData,
    isLoading: agentsLoading,
    isError: agentsError,
  } = useQuery({
    queryKey: ["agents", "list", { per_page: 100 }],
    queryFn: () => getAgents({ per_page: 100 }),
  });

  // Map API shapes to the chart prop shapes.
  const agentUtilizationData: AgentUtilizationDataPoint[] =
    agentsData?.data.map((agent: Agent) => ({
      name: agent.name,
      // performance_score is 0–100 and serves as the utilization proxy when
      // no aggregate per-agent utilization endpoint exists.
      utilization: Math.round(agent.performance_score),
    })) ?? [];

  const taskCompletionData: TaskCompletionDataPoint[] =
    taskCompletionRaw?.map((point) => ({
      // Use a short date label (MM/DD) for readability on the chart axis.
      day: new Date(point.date).toLocaleDateString("en-US", {
        month: "numeric",
        day: "numeric",
      }),
      tasks: point.completed,
    })) ?? [];

  return (
    <div className="space-y-8">
      {/* Page header */}
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Metrics</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Monitor team performance, agent utilisation, and task throughput.
          </p>
        </div>

        {/* Date range selector */}
        <div className="flex rounded-md border border-border bg-background p-1 gap-1">
          {DATE_RANGE_OPTIONS.map(({ label, value }) => (
            <button
              key={value}
              type="button"
              onClick={() => setDateRange(value)}
              className={[
                "rounded px-3 py-1.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                dateRange === value
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
              ].join(" ")}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      {/* Summary metric cards */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {summaryLoading ? (
          <>
            <MetricCardSkeleton />
            <MetricCardSkeleton />
            <MetricCardSkeleton />
            <MetricCardSkeleton />
          </>
        ) : summaryError || !summary ? (
          <p className="col-span-4 text-sm text-muted-foreground">
            Unable to load summary metrics.
          </p>
        ) : (
          <>
            <MetricCard
              title="Tasks Completed"
              value={summary.tasks_completed.toLocaleString()}
              // Delta is not available from this endpoint; display 0 as a
              // neutral placeholder until a time-series summary endpoint exists.
              delta={0}
              description={`Over the last ${dateRange}`}
            />
            <MetricCard
              title="Active Agents"
              value={String(summary.active_agents)}
              delta={0}
              description="Currently deployed agents"
            />
            <MetricCard
              title="Total Tasks"
              value={summary.total_tasks.toLocaleString()}
              delta={0}
              description="All tasks across projects"
            />
            <MetricCard
              title="Tasks In Progress"
              value={summary.tasks_in_progress.toLocaleString()}
              delta={0}
              description="Currently being worked on"
            />
          </>
        )}
      </div>

      {/* Charts grid */}
      <div className="grid gap-6 lg:grid-cols-2">
        {/* Agent Utilisation */}
        <div className="rounded-xl border border-border bg-card p-6">
          <h2 className="mb-4 text-base font-semibold">Agent Utilisation</h2>
          {agentsLoading ? (
            <ChartSkeleton />
          ) : agentsError ? (
            <p className="flex h-[260px] items-center justify-center text-sm text-muted-foreground">
              Unable to load agent utilisation data.
            </p>
          ) : agentUtilizationData.length === 0 ? (
            <p className="flex h-[260px] items-center justify-center text-sm text-muted-foreground">
              No agent data available.
            </p>
          ) : (
            <AgentUtilizationChart data={agentUtilizationData} />
          )}
        </div>

        {/* Task Completion Trend */}
        <div className="rounded-xl border border-border bg-card p-6">
          <h2 className="mb-4 text-base font-semibold">Task Completion Trend</h2>
          {taskCompletionLoading ? (
            <ChartSkeleton />
          ) : taskCompletionError ? (
            <p className="flex h-[260px] items-center justify-center text-sm text-muted-foreground">
              Unable to load task completion data.
            </p>
          ) : taskCompletionData.length === 0 ? (
            <p className="flex h-[260px] items-center justify-center text-sm text-muted-foreground">
              No task completion data for this period.
            </p>
          ) : (
            <TaskCompletionChart data={taskCompletionData} />
          )}
        </div>
      </div>
    </div>
  );
}
