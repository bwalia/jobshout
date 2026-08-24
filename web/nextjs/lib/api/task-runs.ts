import { apiClient } from "@/lib/api/client";
import type { PaginatedResponse, PaginationParams } from "@/lib/types/common";
import type { CreateTaskRunRequest, TaskRun } from "@/lib/types/task-run";

/** Launch an on-demand agent run of a task. Returns the queued run (202). */
export async function createTaskRun(
  taskId: string,
  payload: CreateTaskRunRequest
): Promise<TaskRun> {
  const { data } = await apiClient.post<TaskRun>(
    `/tasks/${taskId}/run`,
    payload
  );
  return data;
}

/** List the runs of a task, newest first. */
export async function getTaskRuns(
  taskId: string,
  params: PaginationParams = {}
): Promise<PaginatedResponse<TaskRun>> {
  const { data } = await apiClient.get<PaginatedResponse<TaskRun>>(
    `/tasks/${taskId}/runs`,
    { params }
  );
  return data;
}

/** Fetch a single run by its own ID (the poll target). */
export async function getTaskRun(runId: string): Promise<TaskRun> {
  const { data } = await apiClient.get<TaskRun>(`/task-runs/${runId}`);
  return data;
}
