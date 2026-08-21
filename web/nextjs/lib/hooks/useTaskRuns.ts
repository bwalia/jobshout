import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryResult,
  type UseMutationResult,
} from "@tanstack/react-query";
import { toast } from "sonner";

import { apiErrorMessage } from "@/lib/api/client";
import { getExecution, type AgentExecution } from "@/lib/api/executions";
import {
  createTaskRun,
  getTaskRun,
  getTaskRuns,
} from "@/lib/api/task-runs";
import type { PaginatedResponse } from "@/lib/types/common";
import type {
  CreateTaskRunRequest,
  TaskRun,
} from "@/lib/types/task-run";
import { isTaskRunActive } from "@/lib/types/task-run";

export const taskRunKeys = {
  all: ["task-runs"] as const,
  byTask: (taskId: string) => [...taskRunKeys.all, "task", taskId] as const,
  detail: (runId: string) => [...taskRunKeys.all, "detail", runId] as const,
  execution: (execId: string) =>
    [...taskRunKeys.all, "execution", execId] as const,
};

/**
 * The runs of a task, newest first. Polls every 2s while any run is still
 * queued or running, then stops — the same convention as blog/workflow runs.
 */
export function useTaskRuns(
  taskId: string
): UseQueryResult<PaginatedResponse<TaskRun>> {
  return useQuery({
    queryKey: taskRunKeys.byTask(taskId),
    queryFn: () => getTaskRuns(taskId, { per_page: 20 }),
    enabled: Boolean(taskId),
    refetchInterval: (query) => {
      const runs = query.state.data?.data;
      const active = runs?.some((r) => isTaskRunActive(r.status));
      return active ? 2000 : false;
    },
  });
}

/** A single run, polled every 1.5s until it reaches a terminal state. */
export function useTaskRun(
  runId: string | null
): UseQueryResult<TaskRun> {
  return useQuery({
    queryKey: taskRunKeys.detail(runId ?? ""),
    queryFn: () => getTaskRun(runId as string),
    enabled: Boolean(runId),
    refetchInterval: (query) => {
      const run = query.state.data;
      return run && isTaskRunActive(run.status) ? 1500 : false;
    },
  });
}

/**
 * The execution behind a run — the tool-call timeline and token/cost detail the
 * debug view renders. Only fetched once a run has an execution_id.
 */
export function useExecution(
  executionId: string | null
): UseQueryResult<AgentExecution> {
  return useQuery({
    queryKey: taskRunKeys.execution(executionId ?? ""),
    queryFn: () => getExecution(executionId as string),
    enabled: Boolean(executionId),
  });
}

/** Launch a run, then refresh that task's run list so it appears immediately. */
export function useCreateTaskRun(): UseMutationResult<
  TaskRun,
  Error,
  { taskId: string; payload: CreateTaskRunRequest }
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ taskId, payload }) => createTaskRun(taskId, payload),
    onSuccess: (run) => {
      queryClient.invalidateQueries({
        queryKey: taskRunKeys.byTask(run.task_id),
      });
      toast.success("Run started.");
    },
    onError: (error: Error) => {
      toast.error(apiErrorMessage(error, "Failed to start run"));
    },
  });
}
