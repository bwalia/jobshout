import { apiClient } from "./client";
import type { Organization } from "@/lib/types/project";

export interface OrgChartEntry {
  agent_id: string;
  manager_id: string | null;
}

export async function getOrganization(orgId: string): Promise<Organization> {
  const { data } = await apiClient.get<Organization>(
    `/organizations/${orgId}`
  );
  return data;
}

export async function updateOrganization(
  orgId: string,
  payload: { name: string; description?: string }
): Promise<Organization> {
  const { data } = await apiClient.put<Organization>(
    `/organizations/${orgId}`,
    payload
  );
  return data;
}

export async function updateOrgChart(
  orgId: string,
  entries: OrgChartEntry[]
): Promise<void> {
  await apiClient.put(`/organizations/${orgId}/chart`, { entries });
}
