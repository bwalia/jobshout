import type { AgentStatus } from "./common";
import type { EngineType } from "./workflow";

export interface Agent {
  id: string;
  org_id: string;
  name: string;
  role: string;
  description: string | null;
  avatar_url: string | null;
  status: AgentStatus;
  model_provider: string | null;
  model_name: string | null;
  system_prompt: string | null;
  performance_score: number;
  manager_id: string | null;
  created_by: string | null;
  engine_type: EngineType;
  engine_config: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export interface CreateAgentRequest {
  name: string;
  role: string;
  description?: string;
  model_provider?: string;
  model_name?: string;
  system_prompt?: string;
  manager_id?: string;
  engine_type?: EngineType;
  engine_config?: Record<string, unknown>;
}

export interface UpdateAgentRequest {
  name?: string;
  role?: string;
  description?: string;
  avatar_url?: string;
  status?: AgentStatus;
  model_provider?: string;
  model_name?: string;
  system_prompt?: string;
  manager_id?: string | null;
}
