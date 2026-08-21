"use client";

import { useEffect, useState } from "react";
import {
  AlertTriangle,
  CheckCircle2,
  ChevronRight,
  Clock,
  Loader2,
} from "lucide-react";

import { useExecution, useTaskRun, useTaskRuns } from "@/lib/hooks/useTaskRuns";
import type { Agent } from "@/lib/types/agent";
import type { Task } from "@/lib/types/project";
import type { TaskRun, TaskRunStatus } from "@/lib/types/task-run";

function statusMeta(status: TaskRunStatus): {
  label: string;
  cls: string;
  icon: React.ReactNode;
} {
  switch (status) {
    case "queued":
      return {
        label: "Queued",
        cls: "text-muted-foreground",
        icon: <Clock className="h-4 w-4" />,
      };
    case "running":
      return {
        label: "Running",
        cls: "text-signal-live",
        icon: <Loader2 className="h-4 w-4 animate-spin" />,
      };
    case "completed":
      return {
        label: "Completed",
        cls: "text-signal-live",
        icon: <CheckCircle2 className="h-4 w-4" />,
      };
    case "failed":
      return {
        label: "Failed",
        cls: "text-signal-error",
        icon: <AlertTriangle className="h-4 w-4" />,
      };
  }
}

function relTime(iso: string): string {
  const then = new Date(iso).getTime();
  const secs = Math.max(0, Math.round((Date.now() - then) / 1000));
  if (secs < 60) return `${secs}s ago`;
  const mins = Math.round(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  return new Date(iso).toLocaleDateString();
}

interface TaskRunViewProps {
  task: Task;
  agents: Agent[];
  /** A run just launched from the parent — select it immediately. */
  focusRunId?: string | null;
}

/**
 * The live run surface for a task: a history list on the left and the selected
 * run's live detail on the right — status, output, the exact config it ran with,
 * and (for debug runs) the full engine tool-call trace.
 */
export function TaskRunView({ task, agents, focusRunId }: TaskRunViewProps) {
  const { data: runsResp, isLoading } = useTaskRuns(task.id);
  const runs = runsResp?.data ?? [];
  const [selectedId, setSelectedId] = useState<string | null>(null);

  // Default the selection to the freshly launched run, else the newest one.
  useEffect(() => {
    if (focusRunId) setSelectedId(focusRunId);
  }, [focusRunId]);
  useEffect(() => {
    if (!selectedId && runs.length > 0) setSelectedId(runs[0].id);
  }, [runs, selectedId]);

  const agentName = (id: string) =>
    agents.find((a) => a.id === id)?.name ?? "Unknown agent";

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading runs…
      </div>
    );
  }

  if (runs.length === 0) {
    return (
      <div className="rounded-lg border border-dashed border-border bg-card/50 p-6 text-center text-sm text-muted-foreground">
        No runs yet. Use <span className="font-medium text-foreground">Run now</span> to
        execute this task with an agent.
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-[240px_1fr]">
      {/* History */}
      <div className="space-y-1.5">
        {runs.map((run) => {
          const m = statusMeta(run.status);
          const active = run.id === selectedId;
          return (
            <button
              key={run.id}
              onClick={() => setSelectedId(run.id)}
              className={
                "flex w-full items-center gap-2 rounded-md border px-3 py-2 text-left text-sm transition-colors " +
                (active
                  ? "border-primary bg-primary/10"
                  : "border-border bg-card hover:bg-accent")
              }
            >
              <span className={m.cls}>{m.icon}</span>
              <span className="min-w-0 flex-1">
                <span className="block truncate">{agentName(run.agent_id)}</span>
                <span className="block truncate font-mono text-xs text-muted-foreground">
                  {relTime(run.created_at)}
                  {run.debug ? " · debug" : ""}
                </span>
              </span>
              <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
            </button>
          );
        })}
      </div>

      {/* Detail */}
      {selectedId ? (
        <RunDetail runId={selectedId} agentName={agentName} />
      ) : (
        <div className="rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground">
          Select a run to see its detail.
        </div>
      )}
    </div>
  );
}

function RunDetail({
  runId,
  agentName,
}: {
  runId: string;
  agentName: (id: string) => string;
}) {
  const { data: run } = useTaskRun(runId);
  if (!run) {
    return (
      <div className="flex items-center gap-2 rounded-lg border border-border bg-card p-4 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading run…
      </div>
    );
  }

  const m = statusMeta(run.status);

  return (
    <div className="space-y-4 rounded-lg border border-border bg-card p-4">
      {/* Status header */}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className={"flex items-center gap-2 text-sm font-medium " + m.cls}>
          {m.icon}
          {m.label}
          <span className="font-normal text-muted-foreground">
            · {agentName(run.agent_id)}
          </span>
        </div>
        <div className="flex flex-wrap gap-3 font-mono text-xs text-muted-foreground">
          <span>{run.total_tokens} tok</span>
          <span>${run.cost_usd.toFixed(4)}</span>
          <span>{run.latency_ms} ms</span>
          <span>{run.iterations} iter</span>
        </div>
      </div>

      {/* Config chips */}
      <div className="flex flex-wrap gap-1.5">
        <ConfigChip label="engine" value={run.engine ?? "default"} />
        <ConfigChip
          label="model"
          value={run.model_name ?? run.model_provider ?? "agent default"}
        />
        {run.skill_slugs.length > 0 && (
          <ConfigChip label="skills" value={run.skill_slugs.join(", ")} />
        )}
        {Object.keys(run.inputs ?? {}).length > 0 && (
          <ConfigChip
            label="inputs"
            value={`${Object.keys(run.inputs).length} key(s)`}
          />
        )}
        {run.debug && <ConfigChip label="debug" value="on" highlight />}
      </div>

      {/* Error */}
      {run.error_message && (
        <div className="rounded-md border border-signal-error/40 bg-signal-error/10 p-3 text-sm text-signal-error">
          {run.error_message}
        </div>
      )}

      {/* Output */}
      {run.output ? (
        <div className="space-y-1.5">
          <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
            Output
          </div>
          <pre className="max-h-80 overflow-auto whitespace-pre-wrap rounded-md border border-border bg-background p-3 text-sm scrollbar-thin">
            {run.output}
          </pre>
        </div>
      ) : (
        run.status === "running" && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" /> The agent is working…
          </div>
        )
      )}

      {/* Debug trace: tool-call timeline from the linked execution */}
      {run.debug && run.execution_id && (
        <DebugTrace executionId={run.execution_id} />
      )}
    </div>
  );
}

function ConfigChip({
  label,
  value,
  highlight,
}: {
  label: string;
  value: string;
  highlight?: boolean;
}) {
  return (
    <span
      className={
        "inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs " +
        (highlight
          ? "border-primary bg-primary/15 text-primary"
          : "border-border bg-background text-muted-foreground")
      }
    >
      <span className="font-mono opacity-70">{label}</span>
      <span className="font-medium text-foreground">{value}</span>
    </span>
  );
}

function DebugTrace({ executionId }: { executionId: string }) {
  const { data: exec, isLoading } = useExecution(executionId);
  const [openIdx, setOpenIdx] = useState<number | null>(null);

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading engine trace…
      </div>
    );
  }
  const calls = exec?.tool_calls ?? [];

  return (
    <div className="space-y-1.5">
      <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        Engine trace · {calls.length} tool call{calls.length === 1 ? "" : "s"}
        {exec ? ` · ${exec.engine_type}` : ""}
      </div>
      {calls.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No tool calls — the agent answered directly.
        </p>
      ) : (
        <ol className="space-y-1.5">
          {calls.map((c, i) => {
            const open = openIdx === i;
            return (
              <li
                key={c.id}
                className="rounded-md border border-border bg-background"
              >
                <button
                  onClick={() => setOpenIdx(open ? null : i)}
                  className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm"
                >
                  <span className="font-mono text-xs text-muted-foreground">
                    {i + 1}
                  </span>
                  <span className="font-mono font-medium">{c.tool_name}</span>
                  {c.error_message && (
                    <AlertTriangle className="h-3.5 w-3.5 text-signal-error" />
                  )}
                  <span className="ml-auto font-mono text-xs text-muted-foreground">
                    {c.duration_ms} ms
                  </span>
                  <ChevronRight
                    className={
                      "h-4 w-4 text-muted-foreground transition-transform " +
                      (open ? "rotate-90" : "")
                    }
                  />
                </button>
                {open && (
                  <div className="space-y-2 border-t border-border px-3 py-2 text-xs">
                    <div>
                      <div className="mb-1 font-medium text-muted-foreground">Input</div>
                      <pre className="overflow-auto whitespace-pre-wrap rounded bg-muted/40 p-2 scrollbar-thin">
                        {JSON.stringify(c.input, null, 2)}
                      </pre>
                    </div>
                    {(c.output || c.error_message) && (
                      <div>
                        <div className="mb-1 font-medium text-muted-foreground">
                          {c.error_message ? "Error" : "Output"}
                        </div>
                        <pre
                          className={
                            "max-h-48 overflow-auto whitespace-pre-wrap rounded p-2 scrollbar-thin " +
                            (c.error_message
                              ? "bg-signal-error/10 text-signal-error"
                              : "bg-muted/40")
                          }
                        >
                          {c.error_message || c.output}
                        </pre>
                      </div>
                    )}
                  </div>
                )}
              </li>
            );
          })}
        </ol>
      )}
    </div>
  );
}
