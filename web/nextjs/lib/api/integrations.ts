import { apiClient } from "@/lib/api/client";

export type IntegrationProvider = "opsapi" | "jira" | "github" | "slack" | "teams";

/**
 * An integration as the API returns it. Secret credential values come back
 * masked ("••••abcd"); `credential_keys` names the ones that are set.
 */
export interface Integration {
  id: string;
  name: string;
  provider: IntegrationProvider;
  base_url: string;
  credentials?: Record<string, string>;
  credential_keys: string[];
  config: Record<string, unknown>;
  status: string;
  last_synced_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateIntegrationRequest {
  name: string;
  provider: IntegrationProvider;
  base_url?: string;
  credentials: Record<string, string>;
  config?: Record<string, unknown>;
}

/** Credentials are merged on the server: a blank or missing key keeps its value. */
export interface UpdateIntegrationRequest {
  name?: string;
  base_url?: string;
  credentials?: Record<string, string>;
  config?: Record<string, unknown>;
}

export async function listIntegrations(): Promise<Integration[]> {
  const { data } = await apiClient.get<Integration[]>("/integrations");
  return Array.isArray(data) ? data : [];
}

export async function createIntegration(payload: CreateIntegrationRequest): Promise<Integration> {
  const { data } = await apiClient.post<Integration>("/integrations", payload);
  return data;
}

export async function updateIntegration(
  id: string,
  payload: UpdateIntegrationRequest
): Promise<Integration> {
  const { data } = await apiClient.put<Integration>(
    `/integrations/${encodeURIComponent(id)}`,
    payload
  );
  return data;
}

export async function deleteIntegration(id: string): Promise<void> {
  await apiClient.delete(`/integrations/${encodeURIComponent(id)}`);
}
