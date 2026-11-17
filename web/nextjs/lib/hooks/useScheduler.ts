import {
  useQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { toast } from "sonner";
import {
  getScheduledTasks,
  getScheduledTask,
  createScheduledTask,
  updateScheduledTask,
  deleteScheduledTask,
  getScheduledTaskRuns,
  runScheduledTaskOnce,
  type SchedulerListParams,
} from "@/lib/api/scheduler";
import type {
  CreateScheduledTaskRequest,
  ScheduledTaskRun,
  UpdateScheduledTaskRequest,
} from "@/lib/types/scheduler";
import { apiErrorMessage } from "@/lib/api/client";

export const schedulerKeys = {
  all: ["scheduled-tasks"] as const,
  lists: () => [...schedulerKeys.all, "list"] as const,
  list: (p: SchedulerListParams) => [...schedulerKeys.lists(), p] as const,
  detail: (id: string) => [...schedulerKeys.all, "detail", id] as const,
  runs: (id: string) => [...schedulerKeys.all, "runs", id] as const,
};

export function useScheduledTasks(params: SchedulerListParams = {}) {
  return useQuery({
    queryKey: schedulerKeys.list(params),
    queryFn: () => getScheduledTasks(params),
  });
}

export function useScheduledTask(id: string) {
  return useQuery({
    queryKey: schedulerKeys.detail(id),
    queryFn: () => getScheduledTask(id),
    enabled: Boolean(id),
  });
}

export function useScheduledTaskRuns(taskId: string) {
  return useQuery({
    queryKey: schedulerKeys.runs(taskId),
    queryFn: () => getScheduledTaskRuns(taskId),
    enabled: Boolean(taskId),
  });
}

export function useCreateScheduledTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createScheduledTask,
    onSuccess: (t) => {
      qc.invalidateQueries({ queryKey: schedulerKeys.lists() });
      toast.success(`Task "${t.name}" scheduled.`);
    },
    onError: (e: Error) => toast.error(`Failed: ${e.message}`),
  });
}

export function useUpdateScheduledTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, payload }: { id: string; payload: UpdateScheduledTaskRequest }) =>
      updateScheduledTask(id, payload),
    onSuccess: (t) => {
      qc.invalidateQueries({ queryKey: schedulerKeys.lists() });
      qc.invalidateQueries({ queryKey: schedulerKeys.detail(t.id) });
      toast.success("Task updated.");
    },
    onError: (e: Error) => toast.error(`Failed: ${e.message}`),
  });
}

export function useDeleteScheduledTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: deleteScheduledTask,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: schedulerKeys.lists() });
      toast.success("Task deleted.");
    },
    onError: (e: Error) => toast.error(`Failed: ${e.message}`),
  });
}

export type RunOnceContext = {
  id: string;
  workflow_id?: string | null;
};

/**
 * Manual one-off fire for testing. Leaves the schedule's next run time alone.
 * Toast links to the resulting article / workflow / runs hub when present.
 */
export function useRunScheduledTaskOnce() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id }: RunOnceContext) => runScheduledTaskOnce(id),
    onSuccess: (run: ScheduledTaskRun, vars) => {
      qc.invalidateQueries({ queryKey: schedulerKeys.lists() });
      qc.invalidateQueries({ queryKey: schedulerKeys.runs(run.scheduled_task_id) });
      if (run.status === "failed") {
        toast.error(run.error_message || "Run failed.");
        return;
      }
      const href = resultHrefForRun(run, vars.workflow_id);
      if (href) {
        toast.success("Run started.", {
          action: {
            label: "Open result",
            onClick: () => {
              window.location.href = href;
            },
          },
        });
      } else {
        toast.success("Run completed. Check history on this schedule.");
      }
    },
    onError: (e: Error) => toast.error(apiErrorMessage(e, "Failed to run schedule")),
  });
}

/** Deep link for whatever the dispatcher produced on this schedule run. */
export function resultHrefForRun(
  run: ScheduledTaskRun,
  workflowId?: string | null
): string | null {
  if (run.workflow_run_id && workflowId) {
    return `/workflows/${workflowId}?run=${run.workflow_run_id}`;
  }
  if (run.execution_id) {
    return "/panel/runs";
  }
  if (!run.output) return null;
  try {
    const parsed = JSON.parse(run.output) as Record<string, unknown>;
    if (typeof parsed.blog_run_id === "string") return `/articles/${parsed.blog_run_id}`;
  } catch {
    // Output is free text for some task types.
  }
  return null;
}
