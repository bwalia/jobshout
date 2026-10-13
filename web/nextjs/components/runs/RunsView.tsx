"use client";

import { useMemo } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  AlertTriangle,
  Bug,
  CheckCircle2,
  Clock,
  Loader2,
  RotateCcw,
  Search,
  X,
} from "lucide-react";
import { agentHref } from "@/lib/agents/links";
import { useAgents } from "@/lib/hooks/useAgents";
import { useAllTasks } from "@/lib/hooks/useTasks";
import { useCreateTaskRun, useOrgTaskRuns } from "@/lib/hooks/useTaskRuns";
import { runsHubHref, taskResultHref } from "@/lib/run-links";
import { cn } from "@/lib/utils/cn";
import type { TaskRun } from "@/lib/types/task-run";

const STATUS_OPTS = [
  { value: "", label: "Any status" },
  { value: "queued", label: "Queued" },
  { value: "running", label: "Running" },
  { value: "completed", label: "Completed" },
  { value: "failed", label: "Failed" },
  { value: "awaiting_approval", label: "Awaiting approval" },
] as const;

const selectCls =
  "h-9 rounded-md border border-input bg-background px-2.5 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

function relative(at?: string | null): string {
  if (!at) return "—";
  const mins = Math.round((Date.now() - Date.parse(at)) / 60000);
  if (Number.isNaN(mins)) return "—";
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const h = Math.round(mins / 60);
  return h < 24 ? `${h}h ago` : `${Math.round(h / 24)}d ago`;
}

function StatusChip({ status }: { status: string }) {
  const meta =
    status === "completed"
      ? { icon: CheckCircle2, cls: "text-status-done", label: "Completed" }
      : status === "failed"
        ? { icon: AlertTriangle, cls: "text-destructive", label: "Failed" }
        : status === "running" || status === "queued"
          ? { icon: Loader2, cls: "text-status-progress", label: status === "queued" ? "Queued" : "Running" }
          : { icon: Clock, cls: "text-muted-foreground", label: status.replace(/_/g, " ") };
  const Icon = meta.icon;
  return (
    <span className={cn("inline-flex items-center gap-1 text-xs font-medium capitalize", meta.cls)}>
      <Icon className={cn("h-3.5 w-3.5", status === "running" && "animate-spin")} />
      {meta.label}
    </span>
  );
}

function shortError(msg: string | null): string {
  if (!msg) return "";
  const one = msg.replace(/\s+/g, " ").trim();
  return one.length > 80 ? `${one.slice(0, 77)}…` : one;
}

/**
 * Org-wide list of task runs: results, failures, cost, and one-click debug.
 */
export function RunsView() {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const q = params.get("q") ?? "";
  const agentFilter = params.get("agent") ?? "";
  const statusFilter = params.get("failed") === "1" ? "failed" : params.get("status") ?? "";
  const failedOnly = params.get("failed") === "1";

  const setParams = (patch: Record<string, string | null>) => {
    const next = new URLSearchParams(params.toString());
    for (const [k, v] of Object.entries(patch)) {
      if (v === null || v === "") next.delete(k);
      else next.set(k, v);
    }
    const qs = next.toString();
    router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
  };

  const listParams = useMemo(
    () => ({
      per_page: 50,
      status: statusFilter || undefined,
      agent_id: agentFilter || undefined,
    }),
    [statusFilter, agentFilter]
  );

  const { data: runsResp, isLoading } = useOrgTaskRuns(listParams);
  const { data: tasksResp } = useAllTasks();
  const { data: agentsResp } = useAgents({ per_page: 100 });
  const createRun = useCreateTaskRun();

  const taskTitle = useMemo(
    () => new Map((tasksResp?.data ?? []).map((t) => [t.id, t.title])),
    [tasksResp]
  );
  const agents = useMemo(() => agentsResp?.data ?? [], [agentsResp]);
  const agentName = useMemo(() => new Map(agents.map((a) => [a.id, a.name])), [agents]);

  const runs = useMemo(() => {
    const needle = q.trim().toLowerCase();
    const list = runsResp?.data ?? [];
    if (!needle) return list;
    return list.filter((r) => {
      const title = taskTitle.get(r.task_id) ?? "";
      const agent = agentName.get(r.agent_id) ?? "";
      const err = r.error_message ?? "";
      const model = `${r.model_provider ?? ""} ${r.model_name ?? ""}`;
      return `${title} ${agent} ${err} ${model} ${r.status}`.toLowerCase().includes(needle);
    });
  }, [runsResp, q, taskTitle, agentName]);

  const filtered = Boolean(q || agentFilter || statusFilter || failedOnly);

  const clearFilters = () =>
    setParams({ q: null, agent: null, status: null, failed: null });

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center gap-2 border-b border-border px-6 py-3">
        <div className="relative min-w-[12rem] flex-1">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="search"
            value={q}
            onChange={(e) => setParams({ q: e.target.value || null })}
            placeholder="Search task, agent, error…"
            className="h-9 w-full rounded-md border border-input bg-background pl-9 pr-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
        <select
          aria-label="Agent"
          value={agentFilter}
          onChange={(e) => setParams({ agent: e.target.value || null })}
          className={selectCls}
        >
          <option value="">Any agent</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
        <select
          aria-label="Status"
          value={failedOnly ? "failed" : statusFilter}
          onChange={(e) => {
            const v = e.target.value;
            if (v === "failed") setParams({ status: null, failed: "1" });
            else setParams({ status: v || null, failed: null });
          }}
          className={selectCls}
        >
          {STATUS_OPTS.map((o) => (
            <option key={o.value || "any"} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
        {filtered && (
          <button
            type="button"
            onClick={clearFilters}
            className="inline-flex h-9 items-center gap-1 rounded-md px-2 text-sm text-muted-foreground hover:text-foreground"
          >
            <X className="h-3.5 w-3.5" /> Clear
          </button>
        )}
        <Link
          href="/panel/usage"
          className="ml-auto text-xs font-medium text-primary hover:underline"
        >
          Usage &amp; costs →
        </Link>
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-4 scrollbar-thin">
        {isLoading ? (
          <div className="flex h-40 items-center justify-center text-sm text-muted-foreground">
            <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Loading runs…
          </div>
        ) : runs.length === 0 ? (
          <div className="mx-auto mt-10 max-w-md rounded-lg border border-dashed border-border p-8 text-center">
            <p className="text-sm text-muted-foreground">
              {filtered ? "No runs match these filters." : "No task runs yet."}
            </p>
            <div className="mt-3 flex justify-center gap-2">
              {filtered ? (
                <button
                  type="button"
                  onClick={clearFilters}
                  className="inline-flex items-center rounded-md border border-border px-3 py-2 text-sm font-medium hover:bg-muted"
                >
                  Clear filters
                </button>
              ) : (
                <Link
                  href="/panel/tasks?new=1"
                  className="inline-flex items-center rounded-md bg-primary px-3 py-2 text-sm font-semibold text-primary-foreground hover:opacity-90"
                >
                  Create a task
                </Link>
              )}
            </div>
          </div>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-border bg-card">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-border bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th scope="col" className="px-4 py-2.5 font-semibold">
                    Status
                  </th>
                  <th scope="col" className="px-4 py-2.5 font-semibold">
                    Task
                  </th>
                  <th scope="col" className="px-4 py-2.5 font-semibold">
                    Agent
                  </th>
                  <th scope="col" className="px-4 py-2.5 font-semibold">
                    Model
                  </th>
                  <th scope="col" className="px-4 py-2.5 font-semibold">
                    When
                  </th>
                  <th scope="col" className="px-4 py-2.5 font-semibold">
                    Tokens
                  </th>
                  <th scope="col" className="px-4 py-2.5 font-semibold">
                    Cost
                  </th>
                  <th scope="col" className="px-4 py-2.5 font-semibold">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {runs.map((run) => (
                  <RunRow
                    key={run.id}
                    run={run}
                    title={taskTitle.get(run.task_id) ?? "Task"}
                    agentLabel={agentName.get(run.agent_id) ?? "Agent"}
                    busy={createRun.isPending}
                    onOpen={(href) => router.push(href)}
                    onRetry={() =>
                      createRun.mutate({ taskId: run.task_id, payload: {} })
                    }
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
        {runsResp && (
          <p className="mt-3 text-xs text-muted-foreground">
            Showing {runs.length}
            {filtered ? " matching" : ""} of {runsResp.total} run
            {runsResp.total === 1 ? "" : "s"}
            {failedOnly ? "" : (
              <>
                {" "}
                ·{" "}
                <Link href={runsHubHref({ failed: true })} className="text-primary hover:underline">
                  Failures only
                </Link>
              </>
            )}
          </p>
        )}
      </div>
    </div>
  );
}

function RunRow({
  run,
  title,
  agentLabel,
  busy,
  onOpen,
  onRetry,
}: {
  run: TaskRun;
  title: string;
  agentLabel: string;
  busy: boolean;
  onOpen: (href: string) => void;
  onRetry: () => void;
}) {
  const resultHref = taskResultHref(run.task_id, run.id);
  const model =
    [run.model_provider, run.model_name].filter(Boolean).join(" · ") || "—";
  const when = relative(run.completed_at ?? run.started_at ?? run.created_at);
  const err = run.status === "failed" ? shortError(run.error_message) : "";

  return (
    <tr
      className="cursor-pointer hover:bg-muted/40"
      onClick={() => onOpen(resultHref)}
    >
      <td className="px-4 py-2.5">
        <StatusChip status={run.status} />
        {err && (
          <p className="mt-0.5 max-w-[14rem] truncate text-xs text-destructive" title={run.error_message ?? undefined}>
            {err}
          </p>
        )}
      </td>
      <td className="max-w-[16rem] px-4 py-2.5">
        <Link
          href={resultHref}
          onClick={(e) => e.stopPropagation()}
          className="truncate font-medium hover:underline"
        >
          {title}
        </Link>
      </td>
      <td className="px-4 py-2.5">
        <Link
          href={agentHref(run.agent_id)}
          onClick={(e) => e.stopPropagation()}
          className="hover:underline"
        >
          {agentLabel}
        </Link>
      </td>
      <td className="max-w-[12rem] truncate px-4 py-2.5 text-muted-foreground">
        {model}
      </td>
      <td className="whitespace-nowrap px-4 py-2.5 text-muted-foreground">{when}</td>
      <td className="tabular px-4 py-2.5 text-muted-foreground">
        {run.total_tokens ? run.total_tokens.toLocaleString() : "—"}
      </td>
      <td className="tabular px-4 py-2.5 text-muted-foreground">
        {run.cost_usd ? `$${run.cost_usd.toFixed(4)}` : "—"}
      </td>
      <td className="px-4 py-2.5" onClick={(e) => e.stopPropagation()}>
        <div className="flex flex-wrap gap-1.5">
          <Link
            href={resultHref}
            className="inline-flex h-8 items-center rounded-md border border-border px-2.5 text-xs font-medium hover:bg-muted"
          >
            Open result
          </Link>
          <Link
            href={resultHref}
            className="inline-flex h-8 items-center gap-1 rounded-md border border-border px-2.5 text-xs font-medium hover:bg-muted"
            title="Open the run with its tool trace"
          >
            <Bug className="h-3.5 w-3.5" /> Debug
          </Link>
          {run.status === "failed" && (
            <button
              type="button"
              disabled={busy}
              onClick={onRetry}
              className="inline-flex h-8 items-center gap-1 rounded-md bg-primary px-2.5 text-xs font-semibold text-primary-foreground hover:opacity-90 disabled:opacity-50"
            >
              {busy ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <RotateCcw className="h-3.5 w-3.5" />
              )}
              Retry
            </button>
          )}
        </div>
      </td>
    </tr>
  );
}
