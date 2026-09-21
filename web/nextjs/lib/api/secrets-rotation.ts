import { apiClient } from "@/lib/api/client";

export type SecretsRotationStatus =
  | "queued"
  | "running"
  | "completed"
  | "failed"
  | "cancelled";

export type SecretsRotationMode = "plan" | "rotate" | "verify" | "rollback";

export interface SecretsRotationPhase {
  key: string;
  label: string;
  status: string;
  message?: string;
  started_at?: string | null;
  ended_at?: string | null;
}

export interface SecretsRotationResult {
  ui_login_url?: string;
  docs_url?: string;
  mount: string;
  path: string;
  engine: string;
  previous_version?: number;
  current_version?: number;
  rolled_back_to?: number;
  keys_rotated?: string[];
  dual_window_seconds?: number;
  old_version_retired?: boolean;
  zero_downtime?: boolean;
  strategy?: string;
  warnings?: string[];
  plan_steps?: string[];
}

export interface SecretsRotationRun {
  id: string;
  agent_id: string;
  task_id?: string | null;
  org_id: string;
  status: SecretsRotationStatus;
  mode: SecretsRotationMode | string;
  provider: string;
  detected_provider?: string;
  vault_addr: string;
  mount: string;
  path: string;
  engine: string;
  keys?: string;
  grace_seconds: number;
  retire_old: boolean;
  dry_run: boolean;
  instruction?: string | null;
  phases?: SecretsRotationPhase[];
  result?: SecretsRotationResult | null;
  error_message?: string | null;
  requested_by?: string | null;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateSecretsRotationRequest {
  agent_id: string;
  task_id?: string;
  mode: SecretsRotationMode | string;
  provider?: string;
  vault_addr?: string;
  mount?: string;
  path: string;
  engine?: string;
  keys?: string;
  grace_seconds?: number;
  retire_old?: boolean;
  dry_run?: boolean;
  new_secret_json?: string;
  instruction?: string;
}

export interface PaginatedSecretsRotationRuns {
  data: SecretsRotationRun[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export async function secretsRotationStatus() {
  const { data } = await apiClient.get<Record<string, unknown>>("/secrets-rotation/status");
  return data;
}

export async function listSecretsRotationRuns(page = 1, perPage = 10) {
  const { data } = await apiClient.get<PaginatedSecretsRotationRuns>("/secrets-rotation/runs", {
    params: { page, per_page: perPage },
  });
  return data;
}

export async function createSecretsRotationRun(body: CreateSecretsRotationRequest) {
  const { data } = await apiClient.post<SecretsRotationRun>("/secrets-rotation/runs", body);
  return data;
}

export async function getSecretsRotationRun(runID: string) {
  const { data } = await apiClient.get<SecretsRotationRun>(`/secrets-rotation/runs/${runID}`);
  return data;
}

export async function cancelSecretsRotationRun(runID: string) {
  const { data } = await apiClient.post<SecretsRotationRun>(
    `/secrets-rotation/runs/${runID}/cancel`
  );
  return data;
}
