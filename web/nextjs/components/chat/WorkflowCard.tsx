"use client";

import {
  AlertTriangle,
  CheckCircle2,
  Circle,
  Loader2,
  Workflow as WorkflowIcon,
} from "lucide-react";

import { useWorkflow, useWorkflowRun } from "@/lib/hooks/useChatDetails";

/**
 * The Workflow card for a chat turn that ran a workflow. Fetches the run and its
 * workflow definition (for step names/order) and shows live progress — which
 * steps have produced output, which is running, which are pending. Polls while
 * the run is still in flight.
 */
export function WorkflowCard({ runId }: { runId: string }) {
  const { data: run, isError, refetch, isLoading } = useWorkflowRun(runId);
  const { data: workflow } = useWorkflow(run?.workflow_id);

  if (isError) {
    return (
      <div className="mt-2 flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-xs text-muted-foreground">
        Couldn&apos;t load this workflow.
        <button
          type="button"
          onClick={() => void refetch()}
          className="rounded underline transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          Retry
        </button>
      </div>
    );
  }

  if (isLoading || !run) {
    return (
      <div className="mt-2 flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-xs text-muted-foreground">
        <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading workflow…
      </div>
    );
  }

  const steps = (workflow?.steps ?? []).slice().sort((a, b) => a.position - b.position);
  const outputs = run.outputs ?? {};
  const done = (name: string) => Object.prototype.hasOwnProperty.call(outputs, name);
  const completedCount = steps.filter((s) => done(s.name)).length;
  const running = run.status === "running" || run.status === "pending";
  const failed = run.status === "failed";
  const cancelled = run.status === "cancelled" || run.status === "canceled";

  return (
    <div className="mt-2 overflow-hidden rounded-lg border border-border bg-card">
      <div className="flex min-w-0 items-center gap-2 px-3 py-2">
        <WorkflowIcon className="h-4 w-4 shrink-0 text-primary" />
        <span className="min-w-0 truncate text-sm font-medium">
          {workflow?.name ?? "Workflow"}
        </span>
        <span
          className={
            "inline-flex shrink-0 items-center gap-1 text-xs font-medium " +
            (failed
              ? "text-signal-error"
              : running
                ? "text-signal-live"
                : cancelled
                  ? "text-muted-foreground"
                  : "text-signal-live")
          }
        >
          {failed ? (
            <AlertTriangle className="h-3.5 w-3.5" />
          ) : running ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : cancelled ? (
            <Circle className="h-3.5 w-3.5" />
          ) : (
            <CheckCircle2 className="h-3.5 w-3.5" />
          )}
          {run.status}
        </span>
        {steps.length > 0 && (
          <span className="ml-auto font-mono text-[11px] text-muted-foreground">
            {completedCount} / {steps.length}
          </span>
        )}
      </div>

      {steps.length > 0 && (
        <ol className="space-y-1 border-t border-border px-3 py-2">
          {(() => {
            let spun = false;
            return steps.map((s) => {
              const isDone = done(s.name);
              const isCurrent = !isDone && running && !spun;
              if (isCurrent) spun = true;
              return (
                <li key={s.id} className="flex items-center gap-2 text-xs">
                  {isDone ? (
                    <CheckCircle2 className="h-3.5 w-3.5 text-signal-live" />
                  ) : isCurrent ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />
                  ) : (
                    <Circle className="h-3.5 w-3.5 text-muted-foreground" />
                  )}
                  <span
                    className={
                      isDone
                        ? "min-w-0 truncate"
                        : "min-w-0 truncate text-muted-foreground"
                    }
                  >
                    {s.name}
                  </span>
                </li>
              );
            });
          })()}
        </ol>
      )}

      {run.error_message && (
        <div className="border-t border-border px-3 py-2 text-xs text-signal-error">
          {run.error_message}
        </div>
      )}
    </div>
  );
}
