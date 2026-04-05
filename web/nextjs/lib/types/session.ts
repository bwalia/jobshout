export interface SessionMsg {
  role: string;
  content: string;
  provider?: string;
  model?: string;
  token_count?: number;
  timestamp?: string;
}

export interface Session {
  id: string;
  org_id: string;
  name: string;
  description: string | null;
  provider_config_id: string | null;
  model_name: string | null;
  status: "active" | "archived" | "deleted";
  context_messages: SessionMsg[];
  total_tokens: number;
  message_count: number;
  tags: string[];
  created_by: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateSessionRequest {
  name: string;
  description?: string;
  provider_config_id?: string;
  model_name?: string;
  tags?: string[];
}

export interface UpdateSessionRequest {
  name?: string;
  description?: string;
  provider_config_id?: string;
  model_name?: string;
  status?: "active" | "archived" | "deleted";
  tags?: string[];
}

export interface CopyContextRequest {
  source_session_id: string;
  include_system?: boolean;
  max_messages?: number;
}

export interface SessionSnapshot {
  id: string;
  session_id: string;
  name: string;
  description: string | null;
  context_messages: SessionMsg[];
  provider_type: string | null;
  model_name: string | null;
  total_tokens: number;
  message_count: number;
  created_at: string;
}

export interface CreateSnapshotRequest {
  name: string;
  description?: string;
}
