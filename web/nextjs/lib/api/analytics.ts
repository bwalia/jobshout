import { apiClient } from "./client";

export interface ProviderBreakdown {
  provider: string;
  cost_usd: number;
  exec_count: number;
  input_tokens: number;
  output_tokens: number;
}

export interface OrgUsageSummary {
  org_id: string;
  total_cost_usd: number;
  total_execs: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_errors: number;
  avg_latency_ms: number;
  by_provider: ProviderBreakdown[];
}

export interface UsageRollup {
  id: string;
  org_id: string;
  agent_id: string | null;
  model_provider: string;
  model_name: string;
  period_type: string;
  period_start: string;
  exec_count: number;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  cost_usd: number;
  avg_latency_ms: number;
  error_count: number;
}

export interface AgentAnalytics {
  agent_id: string;
  agent_name?: string;
  total_runs: number;
  success_rate: number;
  avg_latency_ms: number;
  total_cost_usd: number;
  total_tokens: number;
  error_count: number;
}

export interface AnomalyAlert {
  agent_id: string;
  agent_name: string;
  metric: string;
  value: number;
  mean: number;
  std_dev: number;
  z_score: number;
  severity: string;
}

export interface OrgBudget {
  id: string;
  org_id: string;
  period: string;
  soft_limit_usd: number | null;
  hard_limit_usd: number | null;
  alert_threshold: number;
  enabled: boolean;
}

export interface BudgetAlert {
  id: string;
  org_id: string;
  budget_id: string;
  alert_type: string;
  spend_usd: number;
  limit_usd: number;
  triggered_at: string;
}

export interface UsageRange {
  from: string;
  to: string;
}

export async function getUsageSummary(range: UsageRange): Promise<OrgUsageSummary> {
  const { data } = await apiClient.get<OrgUsageSummary>("/analytics/usage/summary", {
    params: range,
  });
  return data;
}

export async function getUsageTimeSeries(
  range: UsageRange & { granularity?: "hourly" | "daily"; agent_id?: string }
): Promise<UsageRollup[]> {
  const { data } = await apiClient.get<UsageRollup[]>("/analytics/usage", {
    params: { granularity: "daily", ...range },
  });
  return data;
}

export async function getTopAgentsBySpend(
  range: UsageRange,
  limit = 10
): Promise<AgentAnalytics[]> {
  const { data } = await apiClient.get<AgentAnalytics[]>("/analytics/top-agents", {
    params: { ...range, limit },
  });
  return data;
}

export async function getUsageAnomalies(range: UsageRange): Promise<AnomalyAlert[]> {
  const { data } = await apiClient.get<AnomalyAlert[]>("/leaderboard/anomalies", {
    params: range,
  });
  return data;
}

export async function getBudgets(): Promise<OrgBudget[]> {
  const { data } = await apiClient.get<OrgBudget[]>("/governance/budgets");
  return data;
}

export async function getBudgetAlerts(): Promise<BudgetAlert[]> {
  const { data } = await apiClient.get<BudgetAlert[]>("/governance/budgets/alerts");
  return data;
}
