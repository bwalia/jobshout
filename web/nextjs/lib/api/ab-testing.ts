import { apiClient } from "@/lib/api/client";

export type ABBackend = {
  label: string;
  weight: number;
  address?: string;
  role?: string;
};

export type ABExperiment = {
  id: string;
  name: string;
  host: string;
  rule_id: string;
  profile_id: string;
  mode: string;
  backends: ABBackend[];
  observe_path: string;
  public_url: string;
  note?: string;
};

export type ABObserveResult = {
  mode: string;
  experiment: string;
  url: string;
  n: number;
  counts: Record<string, number>;
  expected: Record<string, number>;
  samples: Array<{
    variant: string;
    status: number;
    latency_ms: number;
    body?: string;
    error?: string;
  }>;
  message: string;
};

export async function abTestStatus() {
  const { data } = await apiClient.get<Record<string, unknown>>("/ab-testing/status");
  return data;
}

export async function listABExperiments() {
  const { data } = await apiClient.get<{
    mode: string;
    count: number;
    experiments: ABExperiment[];
  }>("/ab-testing/experiments");
  return data;
}

export async function setABWeights(id: string, backends: Array<{ label: string; weight: number }>) {
  const { data } = await apiClient.post<Record<string, unknown>>(
    `/ab-testing/experiments/${encodeURIComponent(id)}/weights`,
    { backends }
  );
  return data;
}

export async function promoteAB(id: string, label: string) {
  const { data } = await apiClient.post<Record<string, unknown>>(
    `/ab-testing/experiments/${encodeURIComponent(id)}/promote`,
    { label }
  );
  return data;
}

export async function rollbackAB(id: string) {
  const { data } = await apiClient.post<Record<string, unknown>>(
    `/ab-testing/experiments/${encodeURIComponent(id)}/rollback`
  );
  return data;
}

export async function observeAB(id: string, n = 40) {
  const { data } = await apiClient.get<ABObserveResult>(
    `/ab-testing/experiments/${encodeURIComponent(id)}/observe`,
    { params: { n } }
  );
  return data;
}
