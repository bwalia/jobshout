import { apiClient } from "@/lib/api/client";
import type { PaginatedResponse, PaginationParams } from "@/lib/types/common";
import type {
  ScheduledTask,
  CreateScheduledTaskRequest,
  UpdateScheduledTaskRequest,
  ScheduledTaskRun,
} from "@/lib/types/scheduler";

export interface SchedulerListParams extends PaginationParams {}

export async function getScheduledTasks(
  params: SchedulerListParams = {}
): Promise<PaginatedResponse<ScheduledTask>> {
  const { data } = await apiClient.get<PaginatedResponse<ScheduledTask>>(
    "/scheduled-tasks",
    { params }
  );
  return data;
}

export async function getScheduledTask(id: string): Promise<ScheduledTask> {
  const { data } = await apiClient.get<ScheduledTask>(`/scheduled-tasks/${id}`);
  return data;
}

export async function createScheduledTask(
  payload: CreateScheduledTaskRequest
): Promise<ScheduledTask> {
  const { data } = await apiClient.post<ScheduledTask>("/scheduled-tasks", payload);
  return data;
}

export async function updateScheduledTask(
  id: string,
  payload: UpdateScheduledTaskRequest
): Promise<ScheduledTask> {
  const { data } = await apiClient.put<ScheduledTask>(`/scheduled-tasks/${id}`, payload);
  return data;
}

export async function deleteScheduledTask(id: string): Promise<void> {
  await apiClient.delete(`/scheduled-tasks/${id}`);
}

export async function getScheduledTaskRuns(
  taskId: string,
  params: PaginationParams = {}
): Promise<PaginatedResponse<ScheduledTaskRun>> {
  const { data } = await apiClient.get<PaginatedResponse<ScheduledTaskRun>>(
    `/scheduled-tasks/${taskId}/runs`,
    { params }
  );
  return data;
}
