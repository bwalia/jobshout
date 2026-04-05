export interface LLMProviderConfig {
  id: string;
  org_id: string;
  name: string;
  provider_type: "ollama" | "openai" | "claude";
  base_url: string;
  api_key: string;
  default_model: string;
  is_default: boolean;
  is_active: boolean;
  config_json: Record<string, unknown>;
  created_by: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateLLMProviderRequest {
  name: string;
  provider_type: "ollama" | "openai" | "claude";
  base_url: string;
  api_key?: string;
  default_model: string;
  is_default?: boolean;
  config_json?: Record<string, unknown>;
}

export interface UpdateLLMProviderRequest {
  name?: string;
  base_url?: string;
  api_key?: string;
  default_model?: string;
  is_default?: boolean;
  is_active?: boolean;
  config_json?: Record<string, unknown>;
}

export interface BuiltinProvider {
  name: string;
  is_default: boolean;
}
