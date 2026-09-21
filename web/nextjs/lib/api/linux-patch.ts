import { apiClient } from "@/lib/api/client";

export interface LinuxPatchRun {
  id: string;
  status: string;
  mode: string;
  workload: string;
  hosts: string;
  dry_run: boolean;
  use_llm: boolean;
  vault_path?: string;
  phases?: Array<{ key: string; label: string; status: string; message?: string }>;
  host_results?: Array<{
    host: string;
    os_flavor?: string;
    status: string;
    packages?: string[];
    error?: string;
    messages?: string[];
  }>;
  plan?: {
    strategy?: string;
    zero_downtime?: boolean;
    order?: string[];
    steps?: string[];
    warnings?: string[];
    llm_summary?: string;
    docs_url?: string;
  };
  error_message?: string | null;
  created_at: string;
}

export async function linuxPatchStatus() {
  const { data } = await apiClient.get<Record<string, unknown>>("/linux-patch/status");
  return data;
}

export async function listLinuxPatchRuns(page = 1, perPage = 10) {
  const { data } = await apiClient.get<{ data: LinuxPatchRun[] }>("/linux-patch/runs", {
    params: { page, per_page: perPage },
  });
  return data;
}

export async function createLinuxPatchRun(body: Record<string, unknown>) {
  const { data } = await apiClient.post<LinuxPatchRun>("/linux-patch/runs", body);
  return data;
}

export async function getLinuxPatchRun(runID: string) {
  const { data } = await apiClient.get<LinuxPatchRun>(`/linux-patch/runs/${runID}`);
  return data;
}

export async function cancelLinuxPatchRun(runID: string) {
  const { data } = await apiClient.post<LinuxPatchRun>(`/linux-patch/runs/${runID}/cancel`);
  return data;
}
