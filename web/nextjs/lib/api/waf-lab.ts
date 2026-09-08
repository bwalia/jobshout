import { apiClient } from "@/lib/api/client";

export type WAFLabRunStatus = "queued" | "running" | "completed" | "failed" | "cancelled";

export interface WAFLabCategoryScore {
  total: number;
  secure_blocked: number;
  open_leaked: number;
  false_positives: number;
}

export interface WAFLabScore {
  attacks_total: number;
  secure_blocked: number;
  secure_expected: number;
  open_leaked: number;
  open_expected_leak: number;
  false_positives: number;
  not_blocked_expected: number;
  by_category: Record<string, WAFLabCategoryScore>;
  suspicious: boolean;
  suspicious_reason?: string;
  bola_note?: string;
}

export interface WAFLabRun {
  id: string;
  agent_id: string;
  task_id?: string | null;
  org_id: string;
  status: WAFLabRunStatus;
  mode: string;
  secure_host: string;
  open_host: string;
  origin_upstream: string;
  policy_id: string;
  manage_dns: string;
  dns_zone: string;
  attack_set: string;
  instruction?: string | null;
  wslproxy_base_url: string;
  score?: WAFLabScore | null;
  error_message?: string | null;
  requested_by?: string | null;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface WAFLabStep {
  id: string;
  run_id: string;
  phase: string;
  status: string;
  message: string;
  started_at?: string | null;
  ended_at?: string | null;
  created_at: string;
}

export interface WAFLabResult {
  id: string;
  run_id: string;
  host_role: "secure" | "open" | string;
  host: string;
  attack_id: string;
  attack_name: string;
  category: string;
  method: string;
  path: string;
  payload?: string;
  status_code: number;
  blocked: boolean;
  expect_block: boolean;
  waf_rule?: string;
  waf_violation?: string;
  support_id?: string;
  latency_ms: number;
  verdict: string;
  notes?: string;
  created_at: string;
}

export interface CreateWAFLabRunRequest {
  agent_id: string;
  task_id?: string;
  wslproxy_base_url: string;
  secure_host: string;
  open_host: string;
  origin_upstream?: string;
  policy_id?: string;
  mode: string;
  manage_dns?: string;
  dns_zone?: string;
  attack_set?: string;
  instruction?: string;
}

export interface PaginatedWAFLabRuns {
  data: WAFLabRun[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export async function wafLabStatus() {
  const { data } = await apiClient.get<Record<string, unknown>>("/waf-lab/status");
  return data;
}

export async function listWafLabRuns(page = 1, perPage = 10) {
  const { data } = await apiClient.get<PaginatedWAFLabRuns>("/waf-lab/runs", {
    params: { page, per_page: perPage },
  });
  return data;
}

export async function createWafLabRun(body: CreateWAFLabRunRequest) {
  const { data } = await apiClient.post<WAFLabRun>("/waf-lab/runs", body);
  return data;
}

export async function getWafLabRun(runID: string) {
  const { data } = await apiClient.get<WAFLabRun>(`/waf-lab/runs/${runID}`);
  return data;
}

export async function listWafLabSteps(runID: string) {
  const { data } = await apiClient.get<WAFLabStep[]>(`/waf-lab/runs/${runID}/steps`);
  return data;
}

export async function listWafLabResults(runID: string) {
  const { data } = await apiClient.get<WAFLabResult[]>(`/waf-lab/runs/${runID}/results`);
  return data;
}

export async function cancelWafLabRun(runID: string) {
  const { data } = await apiClient.post<WAFLabRun>(`/waf-lab/runs/${runID}/cancel`);
  return data;
}
