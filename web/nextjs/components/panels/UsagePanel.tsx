"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import {
  AlertTriangle,
  Cpu,
  ExternalLink,
  Loader2,
  RotateCcw,
} from "lucide-react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { MetricCard } from "@/components/dashboard/MetricCard";
import { agentHref } from "@/lib/agents/links";
import { useAgents } from "@/lib/hooks/useAgents";
import { useRunFailures, type RunFailure } from "@/lib/hooks/useRunFailures";
import { useRetryBlogRun } from "@/lib/hooks/useBlog";
import { useCreateTaskRun } from "@/lib/hooks/useTaskRuns";
import {
  useBudgetAlerts,
  useBudgets,
  useTopSpendAgents,
  useUsageAnomalies,
  useUsageSeries,
  useUsageSummary,
  type UsagePeriod,
} from "@/lib/hooks/useUsage";
import { runsHubHref, taskResultHref } from "@/lib/run-links";
import { cn } from "@/lib/utils/cn";

const PERIODS: { id: UsagePeriod; label: string }[] = [
  { id: "7d", label: "7 days" },
  { id: "30d", label: "30 days" },
  { id: "month", label: "This month" },
];

function money(n: number): string {
  if (n >= 100) return `$${n.toFixed(0)}`;
  if (n >= 1) return `$${n.toFixed(2)}`;
  return `$${n.toFixed(4)}`;
}

function relative(at: string): string {
  const mins = Math.round((Date.now() - Date.parse(at)) / 60000);
  if (Number.isNaN(mins)) return "";
  if (mins < 60) return `${Math.max(mins, 1)}m ago`;
  const h = Math.round(mins / 60);
  return h < 24 ? `${h}h ago` : `${Math.round(h / 24)}d ago`;
}

function dayLabel(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function FailureRow({ f, agentName }: { f: RunFailure; agentName?: string }) {
  const retryArticle = useRetryBlogRun();
  const createRun = useCreateTaskRun();
  const busy = retryArticle.isPending || createRun.isPending;
  const retry =
    f.kind === "article"
      ? () => retryArticle.mutate(f.runId)
      : f.kind === "task" && f.taskId
        ? () => createRun.mutate({ taskId: f.taskId as string, payload: {} })
        : undefined;
  const openHref =
    f.taskId && f.runId
      ? taskResultHref(f.taskId, f.runId)
      : f.taskId
        ? taskResultHref(f.taskId)
        : f.kind === "article"
          ? `/articles/${f.runId}`
          : f.agentId
            ? agentHref(f.agentId, "workspace")
            : undefined;

  return (
    <li className="flex flex-col gap-2 py-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">
          {f.agentId ? (
            <Link href={agentHref(f.agentId)} className="hover:underline">
              {agentName ?? "Agent"}
            </Link>
          ) : (
            "Unassigned"
          )}
          <span className="text-muted-foreground"> · {f.title}</span>
        </p>
        <p className="mt-0.5 text-sm text-destructive">
          {f.error}{" "}
          <span className="text-xs text-muted-foreground">· {relative(f.at)}</span>
        </p>
      </div>
      <div className="flex shrink-0 flex-wrap gap-1.5">
        {retry && (
          <button
            type="button"
            onClick={retry}
            disabled={busy}
            className="inline-flex h-8 items-center gap-1 rounded-md bg-primary px-2.5 text-xs font-semibold text-primary-foreground hover:opacity-90 disabled:opacity-50"
          >
            {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RotateCcw className="h-3.5 w-3.5" />}
            Retry
          </button>
        )}
        {f.agentId && (
          <Link
            href={agentHref(f.agentId, "models")}
            className="inline-flex h-8 items-center gap-1 rounded-md border border-border px-2.5 text-xs font-medium hover:bg-muted"
          >
            <Cpu className="h-3.5 w-3.5" /> Change model
          </Link>
        )}
        {openHref && (
          <Link
            href={openHref}
            className="inline-flex h-8 items-center gap-1 rounded-md border border-border px-2.5 text-xs font-medium hover:bg-muted"
          >
            <ExternalLink className="h-3.5 w-3.5" /> Open run
          </Link>
        )}
      </div>
    </li>
  );
}

/**
 * Costs, tokens, top agents, budgets, and failures — the observability home.
 */
export function UsagePanel() {
  const [period, setPeriod] = useState<UsagePeriod>("30d");
  const summary = useUsageSummary(period);
  const series = useUsageSeries(period);
  const topAgents = useTopSpendAgents(period);
  const anomalies = useUsageAnomalies(period);
  const budgets = useBudgets();
  const alerts = useBudgetAlerts();
  const { failures, isLoading: failuresLoading } = useRunFailures();
  const { data: agentsResp } = useAgents({ per_page: 100 });
  const names = useMemo(
    () => new Map((agentsResp?.data ?? []).map((a) => [a.id, a.name])),
    [agentsResp]
  );

  const chartData = useMemo(() => {
    const byDay = new Map<string, { day: string; cost: number; tokens: number; errors: number }>();
    for (const row of series.data ?? []) {
      const day = dayLabel(row.period_start);
      const cur = byDay.get(day) ?? { day, cost: 0, tokens: 0, errors: 0 };
      cur.cost += row.cost_usd;
      cur.tokens += row.total_tokens;
      cur.errors += row.error_count;
      byDay.set(day, cur);
    }
    return Array.from(byDay.values());
  }, [series.data]);

  const s = summary.data;
  const loading = summary.isLoading || series.isLoading;

  return (
    <div className="space-y-6 p-6">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-2xl">Usage</h1>
          <p className="text-sm text-muted-foreground">
            Spend, tokens, and failures across agents and models — open a failed run to debug or
            retest.
          </p>
        </div>
        <div className="flex rounded-md border border-border p-0.5" role="group" aria-label="Period">
          {PERIODS.map((p) => (
            <button
              key={p.id}
              type="button"
              aria-pressed={period === p.id}
              onClick={() => setPeriod(p.id)}
              className={cn(
                "rounded px-3 py-1.5 text-sm font-medium",
                period === p.id
                  ? "bg-secondary text-foreground"
                  : "text-muted-foreground hover:text-foreground"
              )}
            >
              {p.label}
            </button>
          ))}
        </div>
      </div>

      {loading && !s ? (
        <div className="flex h-40 items-center justify-center text-sm text-muted-foreground">
          <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Loading usage…
        </div>
      ) : (
        <>
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <MetricCard title="Total spend" value={money(s?.total_cost_usd ?? 0)} description={PERIODS.find((p) => p.id === period)?.label} />
            <MetricCard
              title="Executions"
              value={(s?.total_execs ?? 0).toLocaleString()}
              description={`${(s?.total_errors ?? 0).toLocaleString()} errors`}
            />
            <MetricCard
              title="Tokens"
              value={((s?.total_input_tokens ?? 0) + (s?.total_output_tokens ?? 0)).toLocaleString()}
              description={`${(s?.total_input_tokens ?? 0).toLocaleString()} in · ${(s?.total_output_tokens ?? 0).toLocaleString()} out`}
            />
            <MetricCard
              title="Avg latency"
              value={s?.avg_latency_ms ? `${(s.avg_latency_ms / 1000).toFixed(1)}s` : "—"}
              description="Per execution"
            />
          </div>

          <section aria-labelledby="usage-chart" className="rounded-xl border border-border bg-card p-5">
            <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
              <h2 id="usage-chart" className="text-base font-semibold">
                Cost &amp; tokens over time
              </h2>
              <Link href={runsHubHref()} className="text-xs font-medium text-primary hover:underline">
                Browse runs →
              </Link>
            </div>
            {chartData.length === 0 ? (
              <p className="py-12 text-center text-sm text-muted-foreground">
                No usage rollups in this period yet.
              </p>
            ) : (
              <ResponsiveContainer width="100%" height={280}>
                <AreaChart data={chartData} margin={{ top: 8, right: 8, left: -8, bottom: 0 }}>
                  <defs>
                    <linearGradient id="usage-cost-fill" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="hsl(var(--primary))" stopOpacity={0.28} />
                      <stop offset="100%" stopColor="hsl(var(--primary))" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="hsl(var(--border))" vertical={false} />
                  <XAxis dataKey="day" tick={{ fontSize: 11, fill: "hsl(var(--muted-foreground))" }} axisLine={false} tickLine={false} minTickGap={24} />
                  <YAxis tick={{ fontSize: 11, fill: "hsl(var(--muted-foreground))" }} axisLine={false} tickLine={false} tickFormatter={(v) => `$${v}`} />
                  <Tooltip
                    content={({ active, payload, label }) => {
                      if (!active || !payload?.length) return null;
                      const row = payload[0]?.payload as { cost: number; tokens: number; errors: number };
                      return (
                        <div className="rounded-lg border border-border bg-card px-3 py-2 text-sm shadow-card-hover">
                          <p className="font-medium">{label}</p>
                          <p className="text-muted-foreground">{money(row.cost)} · {row.tokens.toLocaleString()} tok · {row.errors} err</p>
                        </div>
                      );
                    }}
                  />
                  <Area type="monotone" dataKey="cost" stroke="hsl(var(--primary))" strokeWidth={2} fill="url(#usage-cost-fill)" />
                </AreaChart>
              </ResponsiveContainer>
            )}
          </section>

          <div className="grid gap-4 lg:grid-cols-2">
            <section aria-labelledby="by-provider" className="rounded-xl border border-border bg-card p-5">
              <h2 id="by-provider" className="mb-3 text-base font-semibold">
                By provider
              </h2>
              {(s?.by_provider ?? []).length === 0 ? (
                <p className="text-sm text-muted-foreground">No provider breakdown yet.</p>
              ) : (
                <ul className="divide-y divide-border">
                  {(s?.by_provider ?? []).map((p) => (
                    <li key={p.provider} className="flex items-center justify-between gap-3 py-2.5 text-sm">
                      <span className="font-medium capitalize">{p.provider || "unknown"}</span>
                      <span className="tabular text-muted-foreground">
                        {money(p.cost_usd)} · {p.exec_count.toLocaleString()} runs
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </section>

            <section aria-labelledby="top-agents" className="rounded-xl border border-border bg-card p-5">
              <h2 id="top-agents" className="mb-3 text-base font-semibold">
                Top agents by spend
              </h2>
              {topAgents.isLoading ? (
                <p className="text-sm text-muted-foreground">Loading…</p>
              ) : (topAgents.data ?? []).length === 0 ? (
                <p className="text-sm text-muted-foreground">No agent spend in this period.</p>
              ) : (
                <ul className="divide-y divide-border">
                  {(topAgents.data ?? []).map((a) => (
                    <li key={a.agent_id} className="flex items-center justify-between gap-3 py-2.5 text-sm">
                      <Link href={agentHref(a.agent_id)} className="truncate font-medium hover:underline">
                        {a.agent_name || names.get(a.agent_id) || "Agent"}
                      </Link>
                      <span className="tabular shrink-0 text-muted-foreground">
                        {money(a.total_cost_usd)} · {a.total_runs} runs
                        {a.error_count > 0 ? ` · ${a.error_count} err` : ""}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </section>
          </div>

          {(anomalies.data?.length ?? 0) > 0 && (
            <section aria-labelledby="anomalies" className="rounded-xl border border-border bg-card p-5">
              <h2 id="anomalies" className="mb-3 text-base font-semibold">
                Anomalies
              </h2>
              <ul className="divide-y divide-border">
                {(anomalies.data ?? []).map((a, i) => (
                  <li key={`${a.agent_id}-${a.metric}-${i}`} className="flex flex-wrap items-center justify-between gap-2 py-2.5 text-sm">
                    <span>
                      <Link href={agentHref(a.agent_id)} className="font-medium hover:underline">
                        {a.agent_name || names.get(a.agent_id) || "Agent"}
                      </Link>
                      <span className="text-muted-foreground">
                        {" "}
                        · {a.metric.replace(/_/g, " ")} is {a.severity}
                      </span>
                    </span>
                    <span className="tabular text-xs text-muted-foreground">
                      z={a.z_score.toFixed(1)} · value {a.value.toFixed(2)}
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          )}

          {((budgets.data?.length ?? 0) > 0 || (alerts.data?.length ?? 0) > 0) && (
            <section aria-labelledby="budgets" className="rounded-xl border border-border bg-card p-5">
              <h2 id="budgets" className="mb-3 text-base font-semibold">
                Budgets &amp; alerts
              </h2>
              {(budgets.data ?? []).length > 0 && (
                <ul className="mb-3 divide-y divide-border">
                  {(budgets.data ?? []).map((b) => (
                    <li key={b.id} className="flex justify-between gap-3 py-2 text-sm">
                      <span className="capitalize">{b.period} budget{b.enabled ? "" : " (off)"}</span>
                      <span className="tabular text-muted-foreground">
                        soft {b.soft_limit_usd != null ? money(b.soft_limit_usd) : "—"} · hard{" "}
                        {b.hard_limit_usd != null ? money(b.hard_limit_usd) : "—"}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
              {(alerts.data ?? []).length > 0 ? (
                <ul className="divide-y divide-destructive/20">
                  {(alerts.data ?? []).slice(0, 5).map((a) => (
                    <li key={a.id} className="py-2 text-sm text-destructive">
                      {a.alert_type.replace(/_/g, " ")}: spent {money(a.spend_usd)} of {money(a.limit_usd)} ·{" "}
                      {relative(a.triggered_at)}
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-sm text-muted-foreground">No budget alerts recently.</p>
              )}
            </section>
          )}

          <section
            aria-labelledby="usage-failures"
            className="rounded-xl border border-destructive/40 bg-destructive/5 p-5"
          >
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h2 id="usage-failures" className="flex items-center gap-2 text-base font-semibold">
                <AlertTriangle className="h-4 w-4 text-destructive" />
                Failures &amp; debug
                {!failuresLoading && failures.length > 0 && (
                  <span className="rounded-full bg-destructive px-2 py-0.5 text-xs font-bold text-destructive-foreground">
                    {failures.length}
                  </span>
                )}
              </h2>
              <Link href={runsHubHref({ failed: true })} className="text-xs font-medium text-primary hover:underline">
                All failed runs →
              </Link>
            </div>
            {failuresLoading ? (
              <p className="mt-3 text-sm text-muted-foreground">Loading failures…</p>
            ) : failures.length === 0 ? (
              <p className="mt-3 text-sm text-muted-foreground">No failed runs in the last two weeks.</p>
            ) : (
              <ul className="mt-1 divide-y divide-destructive/20">
                {failures.slice(0, 8).map((f) => (
                  <FailureRow
                    key={f.key}
                    f={f}
                    agentName={f.agentId ? names.get(f.agentId) : undefined}
                  />
                ))}
              </ul>
            )}
          </section>
        </>
      )}
    </div>
  );
}
