import { apiClient } from "@/lib/api/client";
import type { PaginatedResponse, PaginationParams } from "@/lib/types/common";
import type {
  Plugin,
  CreatePluginRequest,
  UpdatePluginRequest,
  PluginExecution,
} from "@/lib/types/workflow";

export async function getPlugins(
  params: PaginationParams = {}
): Promise<PaginatedResponse<Plugin>> {
  const { data } = await apiClient.get("/plugins", { params });
  return data;
}

export async function getPlugin(id: string): Promise<Plugin> {
  const { data } = await apiClient.get(`/plugins/${id}`);
  return data;
}

export async function createPlugin(
  payload: CreatePluginRequest
): Promise<Plugin> {
  const { data } = await apiClient.post("/plugins", payload);
  return data;
}

export async function updatePlugin(
  id: string,
  payload: UpdatePluginRequest
): Promise<Plugin> {
  const { data } = await apiClient.put(`/plugins/${id}`, payload);
  return data;
}

export async function deletePlugin(id: string): Promise<void> {
  await apiClient.delete(`/plugins/${id}`);
}

export async function executePlugin(
  id: string,
  input: Record<string, unknown> = {}
): Promise<PluginExecution> {
  const { data } = await apiClient.post(`/plugins/${id}/execute`, { input });
  return data;
}

export async function getPluginExecutions(
  pluginId: string,
  params: PaginationParams = {}
): Promise<PaginatedResponse<PluginExecution>> {
  const { data } = await apiClient.get(`/plugins/${pluginId}/executions`, {
    params,
  });
  return data;
}
