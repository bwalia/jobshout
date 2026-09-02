"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import {
  ListChecks,
  CheckCircle2,
  Activity,
  Bot,
  ArrowRight,
  MessageSquare,
  Plus,
} from "lucide-react";
import { getDashboardSummary, getTaskCompletion } from "@/lib/api/metrics";
import { getProject } from "@/lib/api/projects";
import { useAgents } from "@/lib/hooks/useAgents";
import { useTasks } from "@/lib/hooks/useTasks";
import { useAuthStore } from "@/lib/store/auth-store";
import { ThroughputChart } from "@/components/dashboard/ThroughputChart";
import { StatusDonut } from "@/components/dashboard/StatusDonut";
import { SecurityTesterCard } from "@/components/dashboard/SecurityTesterCard";
import type { StatusSlice } from "@/components/dashboard/StatusDonut";
import type { Agent } from "@/lib/types/agent";
import type { Task } from "@/lib/types/project";
import { formatDateOnly } from "@/lib/dates";
import { STATUS_DOT, STATUS_DOT_HSL } from "@/lib/status-colors";
import { cn } from "@/lib/utils/cn";

type DateRange = "7d" | "30d" | "90d";
const RANGES: { label: string; value: DateRange; days: number }[] = [
  { label: "7d", value: "7d", days: 7 },
  { label: "30d", value: "30d", days: 30 },
  { label: "90d", value: "90d", days: 90 },
];

const PRIORITY_PILL: Record<string, string> = {
  critical: "bg-red-500/10 text-red-600 dark:text-red-400",
  high: "bg-orange-500/10 text-orange-600 dark:text-orange-400",
  medium: "bg-amber-500/10 text-amber-600 dark:text-amber-400",
  low: "bg-blue-500/10 text-blue-600 dark:text-blue-400",
};

const AVATAR_COLOURS = [
  "bg-violet-600",
  "bg-blue-600",
  "bg-emerald-600",
  "bg-amber-600",
  "bg-rose-600",
  "bg-cyan-600",
];

function greeting(): string {
  const h = new Date().getHours();
  if (h < 12) return "Good morning";
  if (h < 18) return "Good afternoon";
  return "Good evening";
}

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.floor(diff / 60_000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return new Date(iso).toLocaleDateString();
}

function initials(name: string): string {
  return name
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0].toUpperCase())
    .join("");
}

function KpiCard({
  label,
  value,
  hint,
  icon: Icon,
  tint,
  progress,
}: {
  label: string;
  value: string;
  hint: string;
  icon: React.ElementType;
  tint: string;
  progress?: number;
}) {
  return (
    <div className="rounded-xl border border-border bg-card p-5 shadow-card transition-shadow hover:shadow-card-hover">
      <div className="flex items-start justify-between">
        <p className="text-sm font-medium text-muted-foreground">{label}</p>
        <span
          className={cn(
            "flex h-8 w-8 items-center justify-center rounded-lg",
            tint
          )}
        >
          <Icon className="h-4 w-4" />
        </span>
      </div>
      <p className="mt-2 text-3xl font-semibold tracking-tight">{value}</p>
      <p className="mt-1 text-xs text-muted-foreground">{hint}</p>
      {typeof progress === "number" && (
        <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-secondary">
          <div
            className="h-full rounded-full bg-primary transition-all duration-500"
            style={{ width: `${Math.min(100, Math.max(0, progress))}%` }}
          />
        </div>
      )}
    </div>
  );
}

function Card({
  title,
  action,
  children,
  className,
}: {
  title: string;
  action?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <section
      className={cn(
        "flex flex-col rounded-xl border border-border bg-card shadow-card",
        className
      )}
    >
      <header className="flex items-center justify-between gap-3 border-b border-border px-5 py-3.5">
        <h2 className="text-sm font-semibold">{title}</h2>
        {action}
      </header>
      <div className="min-h-0 flex-1 p-5">{children}</div>
    </section>
  );
}

function Skeleton({ className }: { className?: string }) {
  return <div className={cn("animate-pulse rounded-lg bg-secondary", className)} />;
}

export function DashboardPanel() {
  const user = useAuthStore((s) => s.user);
  const [range, setRange] = useState<DateRange>("7d");
  const days = RANGES.find((r) => r.value === range)?.days ?? 7;

  const summaryQuery = useQuery({
    queryKey: ["metrics", "summary"],
    queryFn: getDashboardSummary,
  });
  const completionQuery = useQuery({
    queryKey: ["metrics", "task-completion", days],
    queryFn: () => getTaskCompletion(days),
  });
  const { data: agentsResp, isLoading: agentsLoading } = useAgents({
    per_page: 100,
  });
  const { data: tasksResp, isLoading: tasksLoading } = useTasks({
    per_page: 20,
  });

  const summary = summaryQuery.data;
  const tasks = useMemo(() => tasksResp?.data ?? [], [tasksResp]);
  const agents = useMemo(() => agentsResp?.data ?? [], [agentsResp]);

  const recentTasks: Task[] = useMemo(
    () =>
      [...tasks]
        .sort(
          (a, b) =>
            new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
        )
        .slice(0, 7),
    [tasks]
  );

  const visibleProjectIds = useMemo(
    () => Array.from(new Set(recentTasks.map((t) => t.project_id))),
    [recentTasks]
  );

  const { data: namedProjects } = useQuery({
    queryKey: ["projects", "named", visibleProjectIds],
    queryFn: async () => {
      const pairs = await Promise.all(
        visibleProjectIds.map(async (id) => {
          try {
            const p = await getProject(id);
            return [id, p.name] as const;
          } catch {
            return [id, "Unknown project"] as const;
          }
        })
      );
      return Object.fromEntries(pairs) as Record<string, string>;
    },
    enabled: visibleProjectIds.length > 0,
  });

  const projectNames = namedProjects ?? {};

  const throughput = useMemo(
    () =>
      (completionQuery.data ?? []).map((p) => ({
        day: formatDateOnly(p.date, { month: "short", day: "numeric" }),
        tasks: p.completed,
      })),
    [completionQuery.data]
  );

  const statusSlices: StatusSlice[] = useMemo(() => {
    const counts = summary?.tasks_by_status ?? {};
    return [
      { name: "Backlog", value: counts.backlog ?? 0, color: STATUS_DOT_HSL.backlog },
      { name: "To Do", value: counts.todo ?? 0, color: STATUS_DOT_HSL.todo },
      { name: "In Progress", value: counts.in_progress ?? 0, color: STATUS_DOT_HSL.in_progress },
      { name: "Review", value: counts.review ?? 0, color: STATUS_DOT_HSL.review },
      { name: "Done", value: counts.done ?? 0, color: STATUS_DOT_HSL.done },
    ];
  }, [summary?.tasks_by_status]);

  const topAgents: Agent[] = useMemo(
    () =>
      [...agents]
        .sort((a, b) => b.performance_score - a.performance_score)
        .slice(0, 6),
    [agents]
  );

  const completionRate =
    summary && summary.total_tasks > 0
      ? Math.round((summary.tasks_completed / summary.total_tasks) * 100)
      : 0;
  const firstName = user?.full_name?.split(" ")[0];

  return (
    <div className="mx-auto max-w-7xl space-y-6 p-6">
      {/* Header */}
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {greeting()}
            {firstName ? `, ${firstName}` : ""}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {new Date().toLocaleDateString(undefined, {
              weekday: "long",
              month: "long",
              day: "numeric",
            })}
            {" — here\u2019s what your agents are up to."}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Link
            href="/panel/task-manager"
            className="inline-flex h-9 items-center gap-1.5 rounded-md border border-border bg-background px-3 text-sm font-medium hover:bg-secondary"
          >
            <Plus className="h-4 w-4" /> New agent
          </Link>
          <Link
            href="/chat"
            className="inline-flex h-9 items-center gap-1.5 rounded-md bg-primary px-3.5 text-sm font-medium text-primary-foreground hover:opacity-90"
          >
            <MessageSquare className="h-4 w-4" /> New chat
          </Link>
        </div>
      </div>

      {/* KPI strip */}
      {summaryQuery.isLoading ? (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-[124px]" />
          ))}
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <KpiCard
            label="Total tasks"
            value={(summary?.total_tasks ?? 0).toLocaleString()}
            hint={`across ${summary?.total_projects ?? 0} projects`}
            icon={ListChecks}
            tint="bg-blue-500/10 text-blue-600 dark:text-blue-400"
          />
          <KpiCard
            label="Completed"
            value={(summary?.tasks_completed ?? 0).toLocaleString()}
            hint={`${completionRate}% completion rate`}
            icon={CheckCircle2}
            tint="bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
            progress={completionRate}
          />
          <KpiCard
            label="In progress"
            value={(summary?.tasks_in_progress ?? 0).toLocaleString()}
            hint="being worked on right now"
            icon={Activity}
            tint="bg-amber-500/10 text-amber-600 dark:text-amber-400"
          />
          <KpiCard
            label="Active agents"
            value={String(summary?.active_agents ?? 0)}
            hint={`of ${summary?.total_agents ?? 0} agents total`}
            icon={Bot}
            tint="bg-violet-500/10 text-violet-600 dark:text-violet-400"
          />
        </div>
      )}

      {/* Charts row */}
      <div className="grid gap-4 lg:grid-cols-3">
        <Card
          title="Task throughput"
          className="lg:col-span-2"
          action={
            <div className="flex rounded-md border border-border p-0.5">
              {RANGES.map((r) => (
                <button
                  key={r.value}
                  type="button"
                  onClick={() => setRange(r.value)}
                  className={cn(
                    "rounded px-2.5 py-1 text-xs font-medium transition-colors",
                    range === r.value
                      ? "bg-secondary text-foreground"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  {r.label}
                </button>
              ))}
            </div>
          }
        >
          {completionQuery.isLoading ? (
            <Skeleton className="h-[280px]" />
          ) : completionQuery.isError ? (
            <p className="flex h-[280px] items-center justify-center text-sm text-muted-foreground">
              Unable to load throughput data.
            </p>
          ) : throughput.length === 0 ? (
            <p className="flex h-[280px] items-center justify-center text-sm text-muted-foreground">
              No completed tasks in this period.
            </p>
          ) : (
            <ThroughputChart data={throughput} />
          )}
        </Card>

        <Card title="Tasks by status">
          {summaryQuery.isLoading ? (
            <Skeleton className="h-[280px]" />
          ) : (
            <StatusDonut
              slices={statusSlices}
              total={summary?.total_tasks ?? 0}
            />
          )}
        </Card>
      </div>

      {/* Lists row */}
      <div className="grid gap-4 lg:grid-cols-3">
        <Card
          title="Recent activity"
          className="lg:col-span-2"
          action={
            <Link
              href="/panel/task-board"
              className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
            >
              Task Board <ArrowRight className="h-3 w-3" />
            </Link>
          }
        >
          {tasksLoading ? (
            <div className="space-y-2">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-12" />
              ))}
            </div>
          ) : recentTasks.length === 0 ? (
            <div className="flex h-40 flex-col items-center justify-center gap-2 text-center">
              <p className="text-sm text-muted-foreground">No tasks yet.</p>
              <Link
                href="/panel/projects"
                className="text-sm font-medium text-primary hover:underline"
              >
                Create a project to get started
              </Link>
            </div>
          ) : (
            <ul className="-mx-2 divide-y divide-border/60">
              {recentTasks.map((task) => (
                <li key={task.id}>
                  <Link
                    href={`/panel/task-board?task=${task.id}`}
                    className="flex items-center gap-3 rounded-lg px-2 py-2.5 transition-colors hover:bg-secondary/50"
                  >
                    <span
                      className={cn(
                        "h-2 w-2 shrink-0 rounded-full",
                        STATUS_DOT[task.status as keyof typeof STATUS_DOT] ??
                          "bg-muted-foreground"
                      )}
                    />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{task.title}</p>
                      <p className="truncate text-xs text-muted-foreground">
                        {projectNames[task.project_id] ?? "Unknown project"}
                      </p>
                    </div>
                    <span
                      className={cn(
                        "shrink-0 rounded-md px-2 py-0.5 text-[11px] font-medium capitalize",
                        PRIORITY_PILL[task.priority] ??
                          "bg-secondary text-muted-foreground"
                      )}
                    >
                      {task.priority}
                    </span>
                    <span className="w-16 shrink-0 text-right font-mono text-[11px] text-muted-foreground">
                      {relativeTime(task.updated_at)}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card
          title="Agent performance"
          action={
            <Link
              href="/panel/task-manager"
              className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
            >
              Task Manager <ArrowRight className="h-3 w-3" />
            </Link>
          }
        >
          {agentsLoading ? (
            <div className="space-y-3">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-10" />
              ))}
            </div>
          ) : topAgents.length === 0 ? (
            <div className="flex h-40 flex-col items-center justify-center gap-2 text-center">
              <p className="text-sm text-muted-foreground">No agents yet.</p>
              <Link
                href="/panel/task-manager"
                className="text-sm font-medium text-primary hover:underline"
              >
                Create your first agent
              </Link>
            </div>
          ) : (
            <ul className="space-y-4">
              {topAgents.map((agent, i) => (
                <li key={agent.id}>
                  <Link
                    href={`/panel/task-manager?agent=${agent.id}`}
                    className="group flex items-center gap-3"
                  >
                    <span
                      className={cn(
                        "flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold text-white",
                        AVATAR_COLOURS[i % AVATAR_COLOURS.length]
                      )}
                    >
                      {initials(agent.name)}
                    </span>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-baseline justify-between gap-2">
                        <p className="truncate text-sm font-medium group-hover:text-primary">
                          {agent.name}
                        </p>
                        <span className="shrink-0 font-mono text-xs text-muted-foreground">
                          {Math.round(agent.performance_score)}%
                        </span>
                      </div>
                      <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-secondary">
                        <div
                          className="h-full rounded-full bg-primary transition-all duration-500"
                          style={{
                            width: `${Math.min(100, Math.max(2, agent.performance_score))}%`,
                          }}
                        />
                      </div>
                    </div>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      {/* Security posture */}
      <div className="grid gap-4 lg:grid-cols-3">
        <SecurityTesterCard />
      </div>
    </div>
  );
}
