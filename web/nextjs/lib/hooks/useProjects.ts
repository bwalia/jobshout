import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryResult,
  type UseMutationResult,
} from "@tanstack/react-query";
import { toast } from "sonner";
import {
  getProjects,
  getProject,
  createProject,
  updateProject,
  deleteProject,
  type ProjectListParams,
} from "@/lib/api/projects";
import type {
  Project,
  CreateProjectRequest,
  UpdateProjectRequest,
} from "@/lib/types/project";
import type { PaginatedResponse } from "@/lib/types/common";

// ---------------------------------------------------------------------------
// Query key factory
// ---------------------------------------------------------------------------
export const projectKeys = {
  all: ["projects"] as const,
  lists: () => [...projectKeys.all, "list"] as const,
  list: (params: ProjectListParams) =>
    [...projectKeys.lists(), params] as const,
  details: () => [...projectKeys.all, "detail"] as const,
  detail: (id: string) => [...projectKeys.details(), id] as const,
};

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

/**
 * Returns a paginated list of projects, optionally filtered by status or
 * search term.
 */
export function useProjects(
  params: ProjectListParams = {}
): UseQueryResult<PaginatedResponse<Project>> {
  return useQuery({
    queryKey: projectKeys.list(params),
    queryFn: () => getProjects(params),
  });
}

/**
 * Returns a single project by ID.
 */
export function useProject(id: string): UseQueryResult<Project> {
  return useQuery({
    queryKey: projectKeys.detail(id),
    queryFn: () => getProject(id),
    enabled: Boolean(id),
  });
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

/**
 * Creates a new project and invalidates the project list cache on success.
 */
export function useCreateProject(): UseMutationResult<
  Project,
  Error,
  CreateProjectRequest
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: createProject,
    onSuccess: (newProject) => {
      queryClient.invalidateQueries({ queryKey: projectKeys.lists() });
      toast.success(`Project "${newProject.name}" created.`);
    },
    onError: (error: Error) => {
      toast.error(`Failed to create project: ${error.message}`);
    },
  });
}

/**
 * Updates an existing project and refreshes its cached detail entry.
 */
export function useUpdateProject(): UseMutationResult<
  Project,
  Error,
  { id: string; payload: UpdateProjectRequest }
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, payload }) => updateProject(id, payload),
    onSuccess: (updatedProject) => {
      queryClient.invalidateQueries({
        queryKey: projectKeys.detail(updatedProject.id),
      });
      queryClient.invalidateQueries({ queryKey: projectKeys.lists() });
      toast.success(`Project "${updatedProject.name}" updated.`);
    },
    onError: (error: Error) => {
      toast.error(`Failed to update project: ${error.message}`);
    },
  });
}

/**
 * Deletes a project and removes it from the cache.
 */
export function useDeleteProject(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: deleteProject,
    onSuccess: (_data, deletedId) => {
      queryClient.removeQueries({ queryKey: projectKeys.detail(deletedId) });
      queryClient.invalidateQueries({ queryKey: projectKeys.lists() });
      toast.success("Project deleted.");
    },
    onError: (error: Error) => {
      toast.error(`Failed to delete project: ${error.message}`);
    },
  });
}
