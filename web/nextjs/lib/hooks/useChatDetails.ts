import { useQuery, type UseQueryResult } from "@tanstack/react-query";

import { getExecution, type AgentExecution } from "@/lib/api/executions";
import { getWorkflow, getWorkflowRun } from "@/lib/api/workflows";
import type { Workflow, WorkflowRun } from "@/lib/types/workflow";

// Detail fetches for the reference cards a chat turn renders. Each is keyed by
// the id the router put in the assistant message's metadata, and only runs when
// that id is present.

const RUNNING = new Set(["pending", "running", "queued", "planning", "executing"]);

/** The execution behind a turn — agent activity, tokens, cost, tool timeline. */
export function useExecution(
  executionId: string | null | undefined
): UseQueryResult<AgentExecution> {
  return useQuery({
    queryKey: ["chat", "execution", executionId ?? ""],
    queryFn: () => getExecution(executionId as string),
    enabled: Boolean(executionId),
    // Keep polling while the run is still in flight.
    refetchInterval: (query) =>
      query.state.data && RUNNING.has(query.state.data.status) ? 2000 : false,
  });
}

/** The workflow run behind a turn — status and per-step progress. */
export function useWorkflowRun(
  runId: string | null | undefined
): UseQueryResult<WorkflowRun> {
  return useQuery({
    queryKey: ["chat", "workflow-run", runId ?? ""],
    queryFn: () => getWorkflowRun(runId as string),
    enabled: Boolean(runId),
    refetchInterval: (query) =>
      query.state.data && RUNNING.has(query.state.data.status) ? 2000 : false,
  });
}

/** The workflow definition, for step names/order when rendering run progress. */
export function useWorkflow(
  workflowId: string | null | undefined
): UseQueryResult<Workflow> {
  return useQuery({
    queryKey: ["chat", "workflow", workflowId ?? ""],
    queryFn: () => getWorkflow(workflowId as string),
    enabled: Boolean(workflowId),
    staleTime: 60_000,
  });
}
