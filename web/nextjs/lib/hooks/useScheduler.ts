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
  type SchedulerListParams,
} from "@/lib/api/scheduler";
import type { CreateScheduledTaskRequest, UpdateScheduledTaskRequest } from "@/lib/types/scheduler";

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
