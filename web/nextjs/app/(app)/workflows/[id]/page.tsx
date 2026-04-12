"use client";

import { useParams, useRouter } from "next/navigation";
import { ArrowLeft, Play, ChevronDown, ChevronRight, Clock, Bot } from "lucide-react";
import { useState, useMemo } from "react";
import { WorkflowBuilder } from "@/components/workflow/WorkflowBuilder";
import {
  useWorkflow,
  useUpdateWorkflow,
  useExecuteWorkflow,
  useWorkflowRuns,
  useWorkflowRun,
} from "@/lib/hooks/useWorkflows";
import type { GraphDefinition, GraphNode, GraphEdge, WorkflowRun } from "@/lib/types/workflow";

function RunStatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    completed: "bg-green-500/20 text-green-400",
    running: "bg-yellow-500/20 text-yellow-400",
    failed: "bg-red-500/20 text-red-400",
    pending: "bg-blue-500/20 text-blue-400",
  };
  return (
    <span
      className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${colors[status] ?? "bg-muted text-muted-foreground"}`}
    >
      {status}
    </span>
  );
}

function formatDuration(startedAt: string | null, completedAt: string | null): string {
  if (!startedAt) return "";
  const start = new Date(startedAt).getTime();
  const end = completedAt ? new Date(completedAt).getTime() : Date.now();
  const ms = end - start;
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.floor(ms / 60000)}m ${Math.round((ms % 60000) / 1000)}s`;
}

function RunCard({
  run,
  stepNames,
  isSelected,
  onSelect,
}: {
  run: WorkflowRun;
  stepNames: string[];
  isSelected: boolean;
  onSelect: (id: string | null) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const isActive = run.status === "running" || run.status === "pending";

  // Poll individual run for live updates while active
  const { data: liveRun } = useWorkflowRun(isActive ? run.id : "");
  const currentRun = liveRun ?? run;

  const outputs = currentRun.outputs ?? {};
  const hasOutputs = Object.keys(outputs).length > 0;

  return (
    <div
      className={`rounded-md border text-xs transition-colors ${
        isSelected
          ? "border-primary bg-primary/5"
          : "border-border hover:border-muted-foreground/30"
      }`}
    >
      {/* Header - always visible */}
      <button
        onClick={() => {
          setExpanded(!expanded);
          onSelect(isSelected ? null : currentRun.id);
        }}
        className="flex w-full items-center gap-2 p-2.5 text-left"
      >
        {hasOutputs || currentRun.error_message ? (
          expanded ? (
            <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
          )
        ) : (
          <div className="h-3 w-3 shrink-0" />
        )}

        <div className="flex flex-1 items-center justify-between min-w-0">
          <span className="font-mono text-muted-foreground">
            {currentRun.id.slice(0, 8)}
          </span>
          <RunStatusBadge status={currentRun.status} />
        </div>
      </button>

      {/* Timing row */}
      <div className="flex items-center gap-1.5 px-2.5 pb-2 text-muted-foreground">
        <Clock className="h-3 w-3" />
        <span>{new Date(currentRun.created_at).toLocaleString()}</span>
        {currentRun.started_at && (
          <span className="ml-auto">
            {formatDuration(currentRun.started_at, currentRun.completed_at)}
            {isActive && " …"}
          </span>
        )}
      </div>

      {/* Expanded: step outputs */}
      {expanded && (
        <div className="border-t border-border">
          {currentRun.error_message && (
            <div className="border-b border-border bg-red-500/5 px-3 py-2">
              <p className="font-medium text-red-400 mb-0.5">Error</p>
              <p className="text-red-400/80 whitespace-pre-wrap break-words">
                {currentRun.error_message}
              </p>
            </div>
          )}

          {hasOutputs ? (
            <div className="divide-y divide-border">
              {stepNames.map((stepName) => {
                const output = outputs[stepName];
                if (output === undefined) return null;
                return (
                  <div key={stepName} className="px-3 py-2.5">
                    <div className="flex items-center gap-1.5 mb-1.5">
                      <Bot className="h-3 w-3 text-primary" />
                      <span className="font-medium text-foreground">
                        {stepName}
                      </span>
                    </div>
                    <p className="text-muted-foreground whitespace-pre-wrap break-words leading-relaxed">
                      {output}
                    </p>
                  </div>
                );
              })}
            </div>
          ) : (
            !currentRun.error_message && (
              <div className="px-3 py-2.5 text-muted-foreground italic">
                {isActive ? "Waiting for agent results…" : "No outputs recorded"}
              </div>
            )
          )}
        </div>
      )}
    </div>
  );
}

export default function WorkflowDetailPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const { data: workflow, isLoading } = useWorkflow(params.id);
  const updateWorkflow = useUpdateWorkflow();
  const executeWorkflow = useExecuteWorkflow();
  const { data: runsData } = useWorkflowRuns(params.id);
  const [showRuns, setShowRuns] = useState(false);
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);

  const runs = runsData?.data ?? [];
  const steps = workflow?.steps ?? [];

  const stepNames = useMemo(
    () => steps.map((s) => s.name),
    [steps],
  );

  // Build execution status map for the selected run to color nodes on the canvas
  const { data: selectedRun } = useWorkflowRun(selectedRunId ?? "");
  const executionStatus = useMemo<
    Record<string, "pending" | "running" | "completed" | "failed"> | undefined
  >(() => {
    if (!selectedRun || steps.length === 0) return undefined;
    const outputs = selectedRun.outputs ?? {};
    const map: Record<string, "pending" | "running" | "completed" | "failed"> =
      {};
    for (const step of steps) {
      if (outputs[step.name] !== undefined) {
        map[step.name] = "completed";
      } else if (
        selectedRun.error_message?.includes(step.name)
      ) {
        map[step.name] = "failed";
      } else if (selectedRun.status === "running") {
        map[step.name] =
          Object.keys(outputs).length === 0 ? "running" : "pending";
      } else {
        map[step.name] = "pending";
      }
    }
    return map;
  }, [selectedRun, steps]);

  if (isLoading) {
    return (
      <div className="flex h-[calc(100vh-4rem)] items-center justify-center">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      </div>
    );
  }

  if (!workflow) {
    return (
      <div className="flex h-[calc(100vh-4rem)] flex-col items-center justify-center gap-3">
        <p className="text-sm text-muted-foreground">Workflow not found</p>
        <button
          onClick={() => router.push("/workflows")}
          className="text-sm text-primary hover:underline"
        >
          Back to workflows
        </button>
      </div>
    );
  }

  const initialGraph: GraphDefinition = {
    nodes: steps.map((step, index): GraphNode => ({
      id: step.id,
      type: (step.engine_type === "langgraph" ? "agent" : "llm") as GraphNode["type"],
      name: step.name,
      config: {
        agent_id: step.agent_id,
        input_template: step.input_template,
        engine_type: step.engine_type,
      },
      position: { x: 250, y: index * 150 },
    })),
    edges: steps.flatMap((step) =>
      (step.depends_on ?? []).map((dep): GraphEdge => {
        const depStep = steps.find((s) => s.name === dep);
        return {
          id: `${depStep?.id ?? dep}-${step.id}`,
          from: depStep?.id ?? dep,
          to: step.id,
        };
      })
    ),
    entry_point: steps[0]?.id,
  };

  const handleSave = (graph: GraphDefinition) => {
    updateWorkflow.mutate({
      id: params.id,
      payload: {
        name: workflow.name,
        description: workflow.description ?? undefined,
      },
    });
  };

  return (
    <div className="flex h-[calc(100vh-4rem)] flex-col">
      <div className="flex items-center gap-4 border-b border-border px-4 py-3">
        <button
          onClick={() => router.push("/workflows")}
          className="rounded-md p-1 text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-5 w-5" />
        </button>

        <div className="flex-1">
          <h1 className="text-lg font-semibold text-foreground">
            {workflow.name}
          </h1>
          {workflow.description && (
            <p className="text-xs text-muted-foreground">
              {workflow.description}
            </p>
          )}
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => setShowRuns(!showRuns)}
            className="rounded-md border border-border px-3 py-1.5 text-xs font-medium text-foreground hover:bg-accent transition-colors"
          >
            {showRuns ? "Hide Runs" : `Runs (${runs.length})`}
          </button>
          <button
            onClick={() =>
              executeWorkflow.mutate({ id: params.id, payload: {} })
            }
            disabled={executeWorkflow.isPending}
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 transition-colors disabled:opacity-50"
          >
            <Play className="h-3 w-3" />
            Execute
          </button>
        </div>
      </div>

      <div className="flex flex-1 overflow-hidden">
        <div className="flex-1">
          <WorkflowBuilder
            initialGraph={initialGraph}
            onSave={handleSave}
            executionStatus={executionStatus}
          />
        </div>

        {showRuns && (
          <div className="w-80 overflow-y-auto border-l border-border bg-card p-4">
            <h3 className="text-sm font-semibold text-foreground mb-3">
              Execution History
            </h3>
            {runs.length === 0 ? (
              <p className="text-xs text-muted-foreground">No runs yet</p>
            ) : (
              <div className="space-y-2">
                {runs.map((run: WorkflowRun) => (
                  <RunCard
                    key={run.id}
                    run={run}
                    stepNames={stepNames}
                    isSelected={selectedRunId === run.id}
                    onSelect={setSelectedRunId}
                  />
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
