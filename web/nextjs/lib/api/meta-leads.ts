import { apiClient } from "@/lib/api/client";

export type MetaLeadConnection = {
  id: string;
  org_id: string;
  name: string;
  page_id: string;
  page_name: string;
  /** The Page's linked Instagram account; empty means Facebook only. */
  ig_account_id?: string;
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
  /** lead_form, or facebook_/instagram_ + dm, comment, mention; instagram_tag. */
  source: string;
  author_name?: string;
  /** The message; a DM thread's messages so far, newest last. */
  body?: string;
  permalink?: string;
  form_id?: string;
  field_data?: { name: string; values: string[] }[];
  analysis?: {
    is_lead?: boolean;
    category?: string;
    evidence?: string;
    intent?: string;
    fit_score?: number;
    next_action?: string;
    spam_risk?: string;
    flags?: string[];
    summary?: string;
  };
  status: string;
  error_message?: string;
  crm_contact_id?: string;
  crm_website?: string;
  crm_synced_at?: string;
  crm_error?: string;
  /** A person's verdict, the eval's ground truth; absent means unlabelled. */
  label_is_lead?: boolean;
  created_at: string;
};

export type MetaLeadEvalRow = {
  /** provider:model */
  model: string;
  error?: string;
  accuracy: number;
  precision: number;
  recall: number;
  consistency: number;
  grounded: number;
  failures: number;
  avg_latency_ms: number;
  misses?: { lead_id: string; body: string; label: boolean; votes: string }[];
};

export type MetaLeadEval = {
  id: string;
  status: string;
  models: string[];
  runs: number;
  case_count: number;
  report?: MetaLeadEvalRow[];
  error_message?: string;
  created_at: string;
};

export type MetaLeadStatus = {
  configured: boolean;
  can_seal_tokens: boolean;
  graph_version: string;
  webhook_verify: boolean;
  llm: boolean;
  crm: boolean;
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

export async function pushMetaLeadCRM(leadId: string, body?: { website?: string; force?: boolean }) {
  const { data } = await apiClient.post<MetaLead>(`/meta-leads/leads/${leadId}/push-crm`, body ?? {});
  return data;
}

/** Records a person's verdict on a DM, comment or mention; null clears it. */
export async function labelMetaLead(leadId: string, isLead: boolean | null) {
  const { data } = await apiClient.put<MetaLead>(`/meta-leads/leads/${leadId}/label`, { is_lead: isLead });
  return data;
}

export async function listMetaEvals() {
  const { data } = await apiClient.get<{ data: MetaLeadEval[] | null; labelled: number }>("/meta-leads/evals");
  return { evals: data.data ?? [], labelled: data.labelled };
}

/** Compares models (provider:model) on the labelled messages, runs times each. */
export async function startMetaEval(models: string[], runs: number) {
  const { data } = await apiClient.post<MetaLeadEval>("/meta-leads/evals", { models, runs });
  return data;
}
