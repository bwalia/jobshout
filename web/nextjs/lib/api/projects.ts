import { apiClient } from "@/lib/api/client";
import type { Project, CreateProjectRequest, UpdateProjectRequest } from "@/lib/types/project";
import type { PaginatedResponse, PaginationParams, ProjectStatus } from "@/lib/types/common";

export interface ProjectListParams extends PaginationParams {
  status?: ProjectStatus;
  search?: string;
}

/**
 * Fetch a paginated list of projects for the current organisation.
 */
export async function getProjects(
  params: ProjectListParams = {}
): Promise<PaginatedResponse<Project>> {
  const { data } = await apiClient.get<PaginatedResponse<Project>>("/projects", {
    params,
  });
  return data;
}

/**
 * Fetch a single project by its ID.
 */
export async function getProject(id: string): Promise<Project> {
  const { data } = await apiClient.get<Project>(`/projects/${id}`);
  return data;
}

/**
 * Create a new project.
 */
export async function createProject(
  payload: CreateProjectRequest
): Promise<Project> {
  const { data } = await apiClient.post<Project>("/projects", payload);
  return data;
}

/**
 * Update an existing project by its ID.
 */
export async function updateProject(
  id: string,
  payload: UpdateProjectRequest
): Promise<Project> {
  const { data } = await apiClient.put<Project>(`/projects/${id}`, payload);
  return data;
}

/**
 * Delete a project by its ID.
 */
export async function deleteProject(id: string): Promise<void> {
  await apiClient.delete(`/projects/${id}`);
}
