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
  /** Platform-owned annotations. Not user-editable. */
  metadata: AgentMetadata | null;
  created_at: string;
  updated_at: string;
}

/**
 * Annotations the platform sets on an agent. `builtin` marks one it seeded
 * itself — "article_writer", "researcher", "mail" — which is how the UI tells a
 * built-in agent apart from one a user created.
 */
export interface AgentMetadata {
  builtin?: string;
  [key: string]: unknown;
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
  /**
   * Per-agent engine settings. Sent whole — the server replaces the stored
   * object rather than merging, so callers spread the existing config when
   * changing one key.
   */
  engine_config?: Record<string, unknown>;
}
