import { apiClient } from "@/lib/api/client";
import type {
  Task,
  CreateTaskRequest,
  UpdateTaskRequest,
  TaskComment,
} from "@/lib/types/project";
import type {
  PaginatedResponse,
  PaginationParams,
  TaskStatus,
  Priority,
} from "@/lib/types/common";

export interface TaskListParams extends PaginationParams {
  project_id?: string;
  status?: TaskStatus;
  priority?: Priority;
  assigned_agent_id?: string;
  assigned_user_id?: string;
  search?: string;
}

export interface TransitionTaskRequest {
  status: TaskStatus;
  comment?: string;
}

export interface ReorderTaskRequest {
  /** Target position index within the current status column */
  position: number;
  /** Optionally move to a different status column at the same time */
  status?: TaskStatus;
}

export const TASK_PAGE_SIZE = 200;
const TASK_FETCH_PAGE_CAP = 10;

export async function fetchAllTaskPages(
  load: (page: number, perPage: number) => Promise<PaginatedResponse<Task>>
): Promise<PaginatedResponse<Task>> {
  const first = await load(1, TASK_PAGE_SIZE);
  const all = [...(first.data ?? [])];
  const total = first.total ?? all.length;
  const pages = Math.min(
    first.total_pages || Math.ceil(total / TASK_PAGE_SIZE) || 1,
    TASK_FETCH_PAGE_CAP
  );
  for (let page = 2; page <= pages; page++) {
    const next = await load(page, TASK_PAGE_SIZE);
    all.push(...(next.data ?? []));
  }
  return { ...first, data: all };
}

/**
 * Fetch a paginated list of tasks, optionally filtered by project or status.
 */
export async function getTasks(
  params: TaskListParams = {}
): Promise<PaginatedResponse<Task>> {
  const { data } = await apiClient.get<PaginatedResponse<Task>>("/tasks", {
    params,
  });
  return data;
}

/**
 * Fetch all tasks that belong to a specific project.
 */
export async function getProjectTasks(
  projectId: string,
  params: Omit<TaskListParams, "project_id"> = {}
): Promise<PaginatedResponse<Task>> {
  const { data } = await apiClient.get<PaginatedResponse<Task>>(
    `/projects/${projectId}/tasks`,
    { params }
  );
  return data;
}

/**
 * Fetch a single task by its ID.
 */
export async function getTask(id: string): Promise<Task> {
  const { data } = await apiClient.get<Task>(`/tasks/${id}`);
  return data;
}

/**
 * Create a new task inside a project.
 */
export async function createTask(payload: CreateTaskRequest): Promise<Task> {
  const { data } = await apiClient.post<Task>("/tasks", payload);
  return data;
}

/**
 * Update an existing task by its ID.
 */
export async function updateTask(
  id: string,
  payload: UpdateTaskRequest
): Promise<Task> {
  const { data } = await apiClient.put<Task>(`/tasks/${id}`, payload);
  return data;
}

/**
 * Delete a task by its ID.
 */
export async function deleteTask(id: string): Promise<void> {
  await apiClient.delete(`/tasks/${id}`);
}

/**
 * Transition a task to a new status, optionally adding a comment to the
 * task's activity log.
 */
export async function transitionTask(
  id: string,
  payload: TransitionTaskRequest
): Promise<Task> {
  const { data } = await apiClient.patch<Task>(
    `/tasks/${id}/transition`,
    payload
  );
  return data;
}

/**
 * Reorder a task within its column (or move it to another column at a new
 * position) without going through the full transition endpoint.
 */
export async function reorderTask(
  id: string,
  payload: ReorderTaskRequest
): Promise<Task> {
  const { data } = await apiClient.put<Task>(`/tasks/${id}/position`, payload);
  return data;
}

/**
 * Fetch the comment thread for a task.
 */
export async function getTaskComments(taskId: string): Promise<TaskComment[]> {
  const { data } = await apiClient.get<TaskComment[]>(
    `/tasks/${taskId}/comments`
  );
  return data;
}

/**
 * Add a comment to a task.
 */
export async function addTaskComment(
  taskId: string,
  body: string
): Promise<TaskComment> {
  const { data } = await apiClient.post<TaskComment>(
    `/tasks/${taskId}/comments`,
    { body }
  );
  return data;
}

export interface TaskHistoryEvent {
  id: string;
  kind: "status" | "run" | "specialist";
  old_status?: string | null;
  new_status?: string | null;
  run_id?: string | null;
  status?: string | null;
  agent_id?: string | null;
  launch_kind?: string | null;
  created_at: string;
  completed_at?: string | null;
  changed_at?: string | null;
}

export interface TaskHistory {
  completed_at: string | null;
  events: TaskHistoryEvent[];
}

/** Status timeline + runs for the Show History panel. */
export async function getTaskHistory(taskId: string): Promise<TaskHistory> {
  const { data } = await apiClient.get<TaskHistory>(
    `/tasks/${taskId}/history`
  );
  return data;
}
