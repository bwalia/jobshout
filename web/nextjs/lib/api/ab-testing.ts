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
  rule_name?: string;
  profile_id: string;
  mode: string;
  backends: ABBackend[];
  observe_path: string;
  public_url: string;
  note?: string;
  /** Other wslproxy hosts attached to the same rule. */
  shared_with?: string[];
  /** False when changing this rule is unsafe or impossible; see read_only_reason. */
  writable: boolean;
  read_only_reason?: string;
  write_path?: "mcp" | "rest" | "demo";
};

export type ABStatus = {
  mode: "demo" | "live" | "read_only" | "unavailable";
  ok: boolean;
  writable: boolean;
  write_path?: "mcp" | "rest" | "demo" | "";
  message: string;
  demo_host: string;
  public_url: string;
  mcp_configured: boolean;
  api_configured: boolean;
  mcp?: { reachable: boolean; tools_enabled: boolean; tool_count: number; message: string };
  api?: { reachable: boolean; message?: string };
};

export type ABObserveResult = {
  mode: string;
  experiment: string;
  url: string;
  n: number;
  counts: Record<string, number>;
  variants?: string[];
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
  const { data } = await apiClient.get<ABStatus>("/ab-testing/status");
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

/** An empty label promotes the rule's second backend. */
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
