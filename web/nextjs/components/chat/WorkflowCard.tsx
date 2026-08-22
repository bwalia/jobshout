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
  const { data: run } = useWorkflowRun(runId);
  const { data: workflow } = useWorkflow(run?.workflow_id);

  if (!run) {
    return (
      <div className="mt-2 flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-xs text-muted-foreground">
        <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading workflow…
      </div>
    );
  }

  const steps = (workflow?.steps ?? [])
    .slice()
    .sort((a, b) => a.position - b.position);
  const outputs = run.outputs ?? {};
  const done = (name: string) => Object.prototype.hasOwnProperty.call(outputs, name);
  const completedCount = steps.filter((s) => done(s.name)).length;
  const running = run.status === "running" || run.status === "pending";
  const failed = run.status === "failed";

  return (
    <div className="mt-2 overflow-hidden rounded-lg border border-border bg-card">
      <div className="flex items-center gap-2 px-3 py-2">
        <WorkflowIcon className="h-4 w-4 text-primary" />
        <span className="text-sm font-medium">
          {workflow?.name ?? "Workflow"}
        </span>
        <span
          className={
            "inline-flex items-center gap-1 text-xs font-medium " +
            (failed
              ? "text-signal-error"
              : running
                ? "text-signal-live"
                : "text-signal-live")
          }
        >
          {failed ? (
            <AlertTriangle className="h-3.5 w-3.5" />
          ) : running ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
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
          {steps.map((s) => {
            const isDone = done(s.name);
            return (
              <li key={s.id} className="flex items-center gap-2 text-xs">
                {isDone ? (
                  <CheckCircle2 className="h-3.5 w-3.5 text-signal-live" />
                ) : running ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />
                ) : (
                  <Circle className="h-3.5 w-3.5 text-muted-foreground" />
                )}
                <span className={isDone ? "" : "text-muted-foreground"}>
                  {s.name}
                </span>
              </li>
            );
          })}
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
