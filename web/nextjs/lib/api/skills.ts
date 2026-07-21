import { apiClient } from "./client";

export type SkillKind = "tool" | "prompt" | "bundle";
export type SkillStatus = "draft" | "published" | "deprecated";

export interface Skill {
  id: string;
  org_id?: string | null;
  slug: string;
  name: string;
  description?: string | null;
  kind: SkillKind;
  config_json: Record<string, unknown>;
  version: string;
  status: SkillStatus;
  created_by?: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateSkillRequest {
  slug: string;
  name: string;
  description?: string;
  kind?: SkillKind;
  config_json?: Record<string, unknown>;
  version?: string;
}

export interface UpdateSkillRequest {
  name?: string;
  description?: string;
  kind?: SkillKind;
  config_json?: Record<string, unknown>;
  version?: string;
  status?: SkillStatus;
}

// ─── Registry ────────────────────────────────────────────────────────────────

export async function getSkills(): Promise<Skill[]> {
  const { data } = await apiClient.get<Skill[]>("/skills");
  return data ?? [];
}

export async function getSkill(id: string): Promise<Skill> {
  const { data } = await apiClient.get<Skill>(`/skills/${id}`);
  return data;
}

export async function createSkill(payload: CreateSkillRequest): Promise<Skill> {
  const { data } = await apiClient.post<Skill>("/skills", payload);
  return data;
}

export async function updateSkill(
  id: string,
  payload: UpdateSkillRequest
): Promise<Skill> {
  const { data } = await apiClient.put<Skill>(`/skills/${id}`, payload);
  return data;
}

export async function deleteSkill(id: string): Promise<void> {
  await apiClient.delete(`/skills/${id}`);
}

// ─── Per-agent enablement ────────────────────────────────────────────────────

export async function getAgentSkills(agentId: string): Promise<Skill[]> {
  const { data } = await apiClient.get<Skill[]>(`/agents/${agentId}/skills`);
  return data ?? [];
}

export async function enableAgentSkill(
  agentId: string,
  skillId: string,
  configOverride: Record<string, unknown> = {}
): Promise<void> {
  await apiClient.post(`/agents/${agentId}/skills`, {
    skill_id: skillId,
    config_override: configOverride,
  });
}

export async function disableAgentSkill(
  agentId: string,
  skillId: string
): Promise<void> {
  await apiClient.delete(`/agents/${agentId}/skills/${skillId}`);
}
