import { useQuery, type UseQueryResult } from "@tanstack/react-query";
import {
  getBudgetAlerts,
  getBudgets,
  getTopAgentsBySpend,
  getUsageAnomalies,
  getUsageSummary,
  getUsageTimeSeries,
  type AgentAnalytics,
  type AnomalyAlert,
  type BudgetAlert,
  type OrgBudget,
  type OrgUsageSummary,
  type UsageRange,
  type UsageRollup,
} from "@/lib/api/analytics";

export type UsagePeriod = "7d" | "30d" | "month";

export function usageRange(period: UsagePeriod): UsageRange {
  const to = new Date();
  const from = new Date(to);
  if (period === "7d") from.setDate(from.getDate() - 7);
  else if (period === "30d") from.setDate(from.getDate() - 30);
  else {
    from.setDate(1);
    from.setHours(0, 0, 0, 0);
  }
  return { from: from.toISOString(), to: to.toISOString() };
}

export const usageKeys = {
  all: ["usage"] as const,
  summary: (period: UsagePeriod) => [...usageKeys.all, "summary", period] as const,
  series: (period: UsagePeriod) => [...usageKeys.all, "series", period] as const,
  topAgents: (period: UsagePeriod) => [...usageKeys.all, "top", period] as const,
  anomalies: (period: UsagePeriod) => [...usageKeys.all, "anomalies", period] as const,
  budgets: () => [...usageKeys.all, "budgets"] as const,
  alerts: () => [...usageKeys.all, "alerts"] as const,
};

export function useUsageSummary(period: UsagePeriod): UseQueryResult<OrgUsageSummary> {
  return useQuery({
    queryKey: usageKeys.summary(period),
    queryFn: () => getUsageSummary(usageRange(period)),
  });
}

export function useUsageSeries(period: UsagePeriod): UseQueryResult<UsageRollup[]> {
  return useQuery({
    queryKey: usageKeys.series(period),
    queryFn: () => getUsageTimeSeries({ ...usageRange(period), granularity: "daily" }),
  });
}

export function useTopSpendAgents(period: UsagePeriod): UseQueryResult<AgentAnalytics[]> {
  return useQuery({
    queryKey: usageKeys.topAgents(period),
    queryFn: () => getTopAgentsBySpend(usageRange(period), 10),
  });
}

export function useUsageAnomalies(period: UsagePeriod): UseQueryResult<AnomalyAlert[]> {
  return useQuery({
    queryKey: usageKeys.anomalies(period),
    queryFn: () => getUsageAnomalies(usageRange(period)),
  });
}

export function useBudgets(): UseQueryResult<OrgBudget[]> {
  return useQuery({
    queryKey: usageKeys.budgets(),
    queryFn: getBudgets,
  });
}

export function useBudgetAlerts(): UseQueryResult<BudgetAlert[]> {
  return useQuery({
    queryKey: usageKeys.alerts(),
    queryFn: getBudgetAlerts,
  });
}
