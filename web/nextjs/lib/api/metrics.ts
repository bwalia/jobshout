import { apiClient } from "./client";

export interface DashboardSummary {
  total_agents: number;
  active_agents: number;
  total_projects: number;
  total_tasks: number;
  tasks_completed: number;
  tasks_in_progress: number;
}

export interface AgentMetric {
  id: string;
  agent_id: string;
  metric_type: string;
  value: number;
  recorded_at: string;
}

export interface TaskCompletionDataPoint {
  date: string;
  completed: number;
}

export async function getDashboardSummary(): Promise<DashboardSummary> {
  const { data } = await apiClient.get<DashboardSummary>("/metrics/summary");
  return data;
}

export async function getAgentMetrics(
  agentId: string,
  from?: string,
  to?: string
): Promise<AgentMetric[]> {
  const params: Record<string, string> = {};
  if (from) params.from = from;
  if (to) params.to = to;
  const { data } = await apiClient.get<AgentMetric[]>(
    `/metrics/agents/${agentId}`,
    { params }
  );
  return data;
}

export async function getTaskCompletion(
  days?: number
): Promise<TaskCompletionDataPoint[]> {
  const params = days ? { days: String(days) } : {};
  const { data } = await apiClient.get<TaskCompletionDataPoint[]>(
    "/metrics/task-completion",
    { params }
  );
  return data;
}
