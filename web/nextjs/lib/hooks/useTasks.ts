import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryResult,
  type UseMutationResult,
} from "@tanstack/react-query";
import { toast } from "sonner";
import {
  getTasks,
  getProjectTasks,
  getTask,
  createTask,
  updateTask,
  deleteTask,
  transitionTask,
  reorderTask,
  type TaskListParams,
  type TransitionTaskRequest,
  type ReorderTaskRequest,
} from "@/lib/api/tasks";
import type { Task, CreateTaskRequest, UpdateTaskRequest } from "@/lib/types/project";
import type { PaginatedResponse } from "@/lib/types/common";

// ---------------------------------------------------------------------------
// Query key factory
// ---------------------------------------------------------------------------
export const taskKeys = {
  all: ["tasks"] as const,
  lists: () => [...taskKeys.all, "list"] as const,
  list: (params: TaskListParams) => [...taskKeys.lists(), params] as const,
  projectLists: (projectId: string) =>
    [...taskKeys.all, "project", projectId] as const,
  details: () => [...taskKeys.all, "detail"] as const,
  detail: (id: string) => [...taskKeys.details(), id] as const,
};

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

/**
 * Returns a paginated list of tasks with optional filters.
 */
export function useTasks(
  params: TaskListParams = {}
): UseQueryResult<PaginatedResponse<Task>> {
  return useQuery({
    queryKey: taskKeys.list(params),
    queryFn: () => getTasks(params),
  });
}

/**
 * Returns all tasks belonging to a specific project.
 */
export function useProjectTasks(
  projectId: string,
  params: Omit<TaskListParams, "project_id"> = {}
): UseQueryResult<PaginatedResponse<Task>> {
  return useQuery({
    queryKey: taskKeys.projectLists(projectId),
    queryFn: () => getProjectTasks(projectId, params),
    enabled: Boolean(projectId),
  });
}

/**
 * Returns a single task by ID.
 */
export function useTask(id: string): UseQueryResult<Task> {
  return useQuery({
    queryKey: taskKeys.detail(id),
    queryFn: () => getTask(id),
    enabled: Boolean(id),
  });
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

/**
 * Creates a new task and invalidates the relevant project task list.
 */
export function useCreateTask(): UseMutationResult<
  Task,
  Error,
  CreateTaskRequest
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: createTask,
    onSuccess: (newTask) => {
      queryClient.invalidateQueries({
        queryKey: taskKeys.projectLists(newTask.project_id),
      });
      queryClient.invalidateQueries({ queryKey: taskKeys.lists() });
      toast.success(`Task "${newTask.title}" created.`);
    },
    onError: (error: Error) => {
      toast.error(`Failed to create task: ${error.message}`);
    },
  });
}

/**
 * Updates an existing task and refreshes its cache entry.
 */
export function useUpdateTask(): UseMutationResult<
  Task,
  Error,
  { id: string; payload: UpdateTaskRequest }
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, payload }) => updateTask(id, payload),
    onSuccess: (updatedTask) => {
      queryClient.invalidateQueries({
        queryKey: taskKeys.detail(updatedTask.id),
      });
      queryClient.invalidateQueries({
        queryKey: taskKeys.projectLists(updatedTask.project_id),
      });
      toast.success(`Task "${updatedTask.title}" updated.`);
    },
    onError: (error: Error) => {
      toast.error(`Failed to update task: ${error.message}`);
    },
  });
}

/**
 * Deletes a task and removes it from the cache.
 */
export function useDeleteTask(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: deleteTask,
    onSuccess: (_data, deletedId) => {
      queryClient.removeQueries({ queryKey: taskKeys.detail(deletedId) });
      // Invalidate all task lists – we don't know which project owned it here
      queryClient.invalidateQueries({ queryKey: taskKeys.all });
      toast.success("Task deleted.");
    },
    onError: (error: Error) => {
      toast.error(`Failed to delete task: ${error.message}`);
    },
  });
}

/**
 * Transitions a task to a new status (e.g. "todo" → "in_progress").
 * Keeps the project board in sync by invalidating the project task list.
 */
export function useTransitionTask(): UseMutationResult<
  Task,
  Error,
  { id: string; payload: TransitionTaskRequest }
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, payload }) => transitionTask(id, payload),
    onSuccess: (updatedTask) => {
      queryClient.invalidateQueries({
        queryKey: taskKeys.detail(updatedTask.id),
      });
      queryClient.invalidateQueries({
        queryKey: taskKeys.projectLists(updatedTask.project_id),
      });
    },
    onError: (error: Error) => {
      toast.error(`Failed to transition task: ${error.message}`);
    },
  });
}

/**
 * Reorders a task within its column, or moves it to another column at a
 * specified position.  Optimistic updates are intentionally omitted here;
 * the caller (kanban board) should manage its own local order state.
 */
export function useReorderTask(): UseMutationResult<
  Task,
  Error,
  { id: string; payload: ReorderTaskRequest }
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, payload }) => reorderTask(id, payload),
    onSuccess: (updatedTask) => {
      queryClient.invalidateQueries({
        queryKey: taskKeys.projectLists(updatedTask.project_id),
      });
    },
    onError: (error: Error) => {
      toast.error(`Failed to reorder task: ${error.message}`);
    },
  });
}
