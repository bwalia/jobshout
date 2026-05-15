import { apiClient } from "@/lib/api/client";

export type SprintStatus = "planning" | "active" | "completed" | "cancelled";

export interface Sprint {
  id: string;
  org_id: string;
  name: string;
  goal?: string;
  status: SprintStatus;
  start_at?: string;
  end_at?: string;
  velocity?: number;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

export interface SprintAgentInfo {
  agent_id: string;
  name: string;
  role: string;
  avatar_url: string | null;
  role_label: "planner" | "executor" | "reviewer" | "any";
}

export interface SprintJob {
  id: string;
  org_id: string;
  task_prompt: string;
  planner_id: string;
  executor_id: string;
  reviewer_id: string;
  status:
    | "pending"
    | "planning"
    | "executing"
    | "reviewing"
    | "completed"
    | "failed";
  approved?: boolean;
  iterations: number;
  max_review: number;
  error_msg?: string;
  created_at: string;
  completed_at?: string;
}

export interface SprintStats {
  total_jobs: number;
  completed_jobs: number;
  failed_jobs: number;
  in_flight_jobs: number;
}

export interface SprintDetail extends Sprint {
  jobs: SprintJob[];
  agents: SprintAgentInfo[];
  stats: SprintStats;
}

export interface CreateSprintRequest {
  name: string;
  goal?: string;
  start_at?: string;
  end_at?: string;
}

export async function listSprints(): Promise<Sprint[]> {
  const { data } = await apiClient.get<Sprint[]>("/sprints");
  return data;
}

export async function getSprint(id: string): Promise<SprintDetail> {
  const { data } = await apiClient.get<SprintDetail>(`/sprints/${id}`);
  return data;
}

export async function createSprint(req: CreateSprintRequest): Promise<Sprint> {
  const { data } = await apiClient.post<Sprint>("/sprints", req);
  return data;
}

export async function updateSprintStatus(
  id: string,
  status: SprintStatus
): Promise<Sprint> {
  const { data } = await apiClient.put<Sprint>(`/sprints/${id}`, { status });
  return data;
}
