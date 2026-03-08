import { apiClient } from "@/lib/api/client";
import type { Agent, CreateAgentRequest, UpdateAgentRequest } from "@/lib/types/agent";
import type { PaginatedResponse, PaginationParams } from "@/lib/types/common";

export interface AgentListParams extends PaginationParams {
  status?: string;
  role?: string;
  search?: string;
}

/**
 * Fetch a paginated list of agents for the current organisation.
 */
export async function getAgents(
  params: AgentListParams = {}
): Promise<PaginatedResponse<Agent>> {
  const { data } = await apiClient.get<PaginatedResponse<Agent>>("/agents", {
    params,
  });
  return data;
}

/**
 * Fetch a single agent by its ID.
 */
export async function getAgent(id: string): Promise<Agent> {
  const { data } = await apiClient.get<Agent>(`/agents/${id}`);
  return data;
}

/**
 * Create a new agent.
 */
export async function createAgent(payload: CreateAgentRequest): Promise<Agent> {
  const { data } = await apiClient.post<Agent>("/agents", payload);
  return data;
}

/**
 * Update an existing agent by its ID.
 */
export async function updateAgent(
  id: string,
  payload: UpdateAgentRequest
): Promise<Agent> {
  const { data } = await apiClient.put<Agent>(`/agents/${id}`, payload);
  return data;
}

/**
 * Delete an agent by its ID.
 */
export async function deleteAgent(id: string): Promise<void> {
  await apiClient.delete(`/agents/${id}`);
}
