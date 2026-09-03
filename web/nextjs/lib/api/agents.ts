import axios from "axios";
import { apiClient, apiErrorMessage } from "@/lib/api/client";
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

export interface AgentPackIssue {
  severity: "error" | "warning" | "info" | string;
  code: string;
  message: string;
}

export interface AgentPackBody {
  name: string;
  role: string;
  description?: string;
  system_prompt?: string;
  model_provider?: string;
  model_name?: string;
  engine_type?: string;
  engine_config?: Record<string, unknown>;
  builtin?: string;
}

export interface AgentPackBindings {
  name?: string;
  model_provider?: string;
  model_name?: string;
  skip_tools?: string[];
  include_gated_tools?: boolean;
}

export interface AgentPackDiff {
  prompt_changed: boolean;
  model_changed: boolean;
  tools_added?: string[];
  tools_removed?: string[];
  knowledge_files: number;
  skills: number;
}

export interface AgentPackPreview {
  preview_id: string;
  mode: "create" | "overlay";
  target_agent_id?: string;
  target_name?: string;
  agent: AgentPackBody;
  issues: AgentPackIssue[];
  bindings: AgentPackBindings;
  diff?: AgentPackDiff;
  can_undo: boolean;
}

export interface AgentPackDocument {
  kind: string;
  schema_version: number;
  agent: AgentPackBody;
  tools?: string[];
  skills?: unknown[];
  knowledge?: { filename: string; content: string }[];
  warnings?: string[];
}

export interface ImportAgentResult {
  agent: Agent;
  mode: "create" | "overlay";
  can_undo: boolean;
}

export async function exportAgent(id: string): Promise<{
  blob: Blob;
  filename: string;
  warnings: string[];
}> {
  try {
    const res = await apiClient.get<Blob>(`/agents/${id}/export`, {
      responseType: "blob",
    });
    const disp = String(res.headers["content-disposition"] ?? "");
    const match = /filename="([^"]+)"/.exec(disp);
    const warnHeader = String(res.headers["x-agent-pack-warnings"] ?? "");
    return {
      blob: res.data,
      filename: match?.[1] ?? "agent.jobshout-agent.json",
      warnings: warnHeader
        ? warnHeader.split("; ").filter(Boolean)
        : [],
    };
  } catch (err) {
    throw new Error(await packErrorMessage(err, "Failed to export agent"));
  }
}

export async function previewAgentImport(
  pack: AgentPackDocument,
): Promise<AgentPackPreview> {
  try {
    const { data } = await apiClient.post<AgentPackPreview>(
      "/agents/import/preview",
      { package: pack },
    );
    return data;
  } catch (err) {
    throw new Error(await packErrorMessage(err, "Failed to validate agent package"));
  }
}

export async function importAgentPackage(payload: {
  preview_id?: string;
  package?: AgentPackDocument;
  bindings: AgentPackBindings;
}): Promise<ImportAgentResult> {
  try {
    const { data } = await apiClient.post<ImportAgentResult>(
      "/agents/import",
      payload,
    );
    return data;
  } catch (err) {
    throw new Error(await packErrorMessage(err, "Failed to import agent"));
  }
}

async function packErrorMessage(err: unknown, fallback: string): Promise<string> {
  if (axios.isAxiosError(err) && err.response?.data instanceof Blob) {
    try {
      const text = await err.response.data.text();
      const parsed = JSON.parse(text) as { error?: string };
      if (parsed.error) return parsed.error;
    } catch {
      /* fall through */
    }
  }
  return apiErrorMessage(err, fallback);
}
