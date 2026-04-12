"use client";

import { useParams, useRouter } from "next/navigation";
import { ArrowLeft, Play } from "lucide-react";
import { useState } from "react";
import { WorkflowBuilder } from "@/components/workflow/WorkflowBuilder";
import {
  useWorkflow,
  useUpdateWorkflow,
  useExecuteWorkflow,
  useWorkflowRuns,
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

export default function WorkflowDetailPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const { data: workflow, isLoading } = useWorkflow(params.id);
  const updateWorkflow = useUpdateWorkflow();
  const executeWorkflow = useExecuteWorkflow();
  const { data: runsData } = useWorkflowRuns(params.id);
  const [showRuns, setShowRuns] = useState(false);

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
    nodes: (workflow.steps ?? []).map((step, index): GraphNode => ({
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
    edges: (workflow.steps ?? []).flatMap((step) =>
      (step.depends_on ?? []).map((dep): GraphEdge => {
        const depStep = workflow.steps?.find((s) => s.name === dep);
        return {
          id: `${depStep?.id ?? dep}-${step.id}`,
          from: depStep?.id ?? dep,
          to: step.id,
        };
      })
    ),
    entry_point: workflow.steps?.[0]?.id,
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

  const runs = runsData?.data ?? [];

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
          />
        </div>

        {showRuns && (
          <div className="w-72 overflow-y-auto border-l border-border bg-card p-4">
            <h3 className="text-sm font-semibold text-foreground mb-3">
              Execution History
            </h3>
            {runs.length === 0 ? (
              <p className="text-xs text-muted-foreground">No runs yet</p>
            ) : (
              <div className="space-y-2">
                {runs.map((run: WorkflowRun) => (
                  <div
                    key={run.id}
                    className="rounded-md border border-border p-2.5 text-xs"
                  >
                    <div className="flex items-center justify-between">
                      <span className="font-mono text-muted-foreground">
                        {run.id.slice(0, 8)}
                      </span>
                      <RunStatusBadge status={run.status} />
                    </div>
                    <p className="mt-1 text-muted-foreground">
                      {new Date(run.created_at).toLocaleString()}
                    </p>
                    {run.error_message && (
                      <p className="mt-1 text-red-400 truncate">
                        {run.error_message}
                      </p>
                    )}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
