"use client";

import { useState } from "react";
import { AgentUtilizationChart } from "@/components/dashboard/AgentUtilizationChart";
import { TaskCompletionChart } from "@/components/dashboard/TaskCompletionChart";
import { MetricCard } from "@/components/dashboard/MetricCard";

type DateRange = "7d" | "30d" | "90d";

const DATE_RANGE_OPTIONS: { label: string; value: DateRange }[] = [
  { label: "7 days", value: "7d" },
  { label: "30 days", value: "30d" },
  { label: "90 days", value: "90d" },
];

// Summary metric data keyed by date range
const SUMMARY_METRICS: Record<
  DateRange,
  { tasksCompleted: number; tasksDelta: number; activeAgents: number; agentsDelta: number; avgUtilization: number; utilizationDelta: number; avgCycleTime: number; cycleTimeDelta: number }
> = {
  "7d": {
    tasksCompleted: 142,
    tasksDelta: 18.3,
    activeAgents: 7,
    agentsDelta: 16.7,
    avgUtilization: 74,
    utilizationDelta: 5.2,
    avgCycleTime: 3.4,
    cycleTimeDelta: -12.1,
  },
  "30d": {
    tasksCompleted: 589,
    tasksDelta: 22.5,
    activeAgents: 8,
    agentsDelta: 14.3,
    avgUtilization: 71,
    utilizationDelta: 3.8,
    avgCycleTime: 3.9,
    cycleTimeDelta: -8.4,
  },
  "90d": {
    tasksCompleted: 1872,
    tasksDelta: 31.0,
    activeAgents: 9,
    agentsDelta: 28.6,
    avgUtilization: 68,
    utilizationDelta: 2.1,
    avgCycleTime: 4.2,
    cycleTimeDelta: -5.9,
  },
};

export default function MetricsPage() {
  const [dateRange, setDateRange] = useState<DateRange>("7d");
  const metrics = SUMMARY_METRICS[dateRange];

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
        <MetricCard
          title="Tasks Completed"
          value={metrics.tasksCompleted.toLocaleString()}
          delta={metrics.tasksDelta}
          description={`Over the last ${dateRange}`}
        />
        <MetricCard
          title="Active Agents"
          value={String(metrics.activeAgents)}
          delta={metrics.agentsDelta}
          description="Currently deployed agents"
        />
        <MetricCard
          title="Avg Utilisation"
          value={`${metrics.avgUtilization}%`}
          delta={metrics.utilizationDelta}
          description="Across all active agents"
        />
        <MetricCard
          title="Avg Cycle Time"
          value={`${metrics.avgCycleTime}d`}
          delta={metrics.cycleTimeDelta}
          description="Days from creation to done"
        />
      </div>

      {/* Charts grid */}
      <div className="grid gap-6 lg:grid-cols-2">
        <div className="rounded-xl border border-border bg-card p-6">
          <h2 className="mb-4 text-base font-semibold">Agent Utilisation</h2>
          <AgentUtilizationChart dateRange={dateRange} />
        </div>

        <div className="rounded-xl border border-border bg-card p-6">
          <h2 className="mb-4 text-base font-semibold">Task Completion Trend</h2>
          <TaskCompletionChart dateRange={dateRange} />
        </div>
      </div>
    </div>
  );
}
