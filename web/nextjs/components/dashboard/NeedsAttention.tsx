"use client";

import Link from "next/link";
import { AlertTriangle, Cpu, ExternalLink, Loader2, RotateCcw } from "lucide-react";
import { agentHref } from "@/lib/agents/links";
import { useAgents } from "@/lib/hooks/useAgents";
import { useRetryBlogRun } from "@/lib/hooks/useBlog";
import { useRunFailures, type RunFailure } from "@/lib/hooks/useRunFailures";
import { useCreateTaskRun } from "@/lib/hooks/useTaskRuns";

/** How many failures the card lists before "View all". */
const SHOWN = 5;

function relative(at: string): string {
  const mins = Math.round((Date.now() - Date.parse(at)) / 60000);
  if (Number.isNaN(mins)) return "";
  if (mins < 60) return `${Math.max(mins, 1)}m ago`;
  const h = Math.round(mins / 60);
  return h < 24 ? `${h}h ago` : `${Math.round(h / 24)}d ago`;
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
  const openHref = f.taskId
    ? `/panel/tasks?task=${f.taskId}`
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
          {f.error} <span className="text-xs text-muted-foreground">· {relative(f.at)}</span>
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
            <ExternalLink className="h-3.5 w-3.5" /> {f.taskId ? "Open task" : "Open run"}
          </Link>
        )}
      </div>
    </li>
  );
}

/**
 * Failed runs from the last two weeks, at the top of the dashboard, so a run
 * that died on a quota error is seen here rather than found by accident. The
 * card is absent when nothing has failed.
 */
export function NeedsAttention() {
  const { failures, isLoading } = useRunFailures();
  const { data: agentsResp } = useAgents({ per_page: 100 });
  if (isLoading || failures.length === 0) return null;
  const names = new Map((agentsResp?.data ?? []).map((a) => [a.id, a.name]));

  return (
    <section
      aria-labelledby="needs-attention"
      className="rounded-xl border border-destructive/40 bg-destructive/5 p-5"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="needs-attention" className="flex items-center gap-2 text-base font-semibold">
          <AlertTriangle className="h-4 w-4 text-destructive" />
          Needs attention
          <span className="rounded-full bg-destructive px-2 py-0.5 text-xs font-bold text-destructive-foreground">
            {failures.length}
          </span>
        </h2>
        <Link href="/panel/tasks?failed=1" className="text-xs font-medium text-primary hover:underline">
          View all failed tasks →
        </Link>
      </div>
      <ul className="mt-1 divide-y divide-destructive/20">
        {failures.slice(0, SHOWN).map((f) => (
          <FailureRow key={f.key} f={f} agentName={f.agentId ? names.get(f.agentId) : undefined} />
        ))}
      </ul>
      {failures.length > SHOWN && (
        <p className="pt-2 text-xs text-muted-foreground">
          And {failures.length - SHOWN} more.
        </p>
      )}
    </section>
  );
}
