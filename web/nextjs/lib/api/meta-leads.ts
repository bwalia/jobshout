import { apiClient } from "@/lib/api/client";

export type MetaLeadConnection = {
  id: string;
  org_id: string;
  name: string;
  page_id: string;
  page_name: string;
  created_at: string;
  updated_at: string;
};

export type MetaLeadRun = {
  id: string;
  status: string;
  form_id?: string;
  form_name?: string;
  focus?: string;
  lead_count: number;
  rollup?: {
    themes?: string[];
    quality_mix?: string;
    recommended_followups?: string[];
    lead_count?: number;
  };
  error_message?: string;
  source: string;
  created_at: string;
};

export type MetaLead = {
  id: string;
  leadgen_id: string;
  form_id?: string;
  field_data?: { name: string; values: string[] }[];
  analysis?: {
    intent?: string;
    fit_score?: number;
    next_action?: string;
    spam_risk?: string;
    flags?: string[];
    summary?: string;
  };
  status: string;
  error_message?: string;
  created_at: string;
};

export type MetaLeadStatus = {
  configured: boolean;
  can_seal_tokens: boolean;
  graph_version: string;
  webhook_verify: boolean;
  llm: boolean;
};

export async function getMetaLeadStatus() {
  const { data } = await apiClient.get<MetaLeadStatus>("/meta-leads/status");
  return data;
}

export async function listMetaConnections() {
  const { data } = await apiClient.get<{ data: MetaLeadConnection[] }>("/meta-leads/connections");
  return data.data ?? [];
}

export async function upsertMetaConnection(body: {
  name: string;
  page_id: string;
  page_name?: string;
  access_token: string;
}) {
  const { data } = await apiClient.put<MetaLeadConnection>("/meta-leads/connections", body);
  return data;
}

export async function listMetaRuns() {
  const { data } = await apiClient.get<{ data: MetaLeadRun[] }>("/meta-leads/runs");
  return data.data ?? [];
}

export async function getMetaRun(runId: string) {
  const { data } = await apiClient.get<MetaLeadRun>(`/meta-leads/runs/${runId}`);
  return data;
}

export async function listMetaRunLeads(runId: string) {
  const { data } = await apiClient.get<{ data: MetaLead[] }>(`/meta-leads/runs/${runId}/leads`);
  return data.data ?? [];
}

export async function listMetaLeads() {
  const { data } = await apiClient.get<{ data: MetaLead[] }>("/meta-leads/leads");
  return data.data ?? [];
}
