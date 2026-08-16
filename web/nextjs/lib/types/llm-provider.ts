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

/** One model a provider can actually run, as reported by GET /llm-providers/models. */
export interface AvailableModel {
  name: string;
  context_tokens: number;
  parameter_size?: string;
  capabilities: string[];
  supports_tools: boolean;
  supports_vision: boolean;
}

/**
 * One provider's models. `source` distinguishes a live probe ("discovered")
 * from a fallback ("static" / "stale"), and `error` is set only when a
 * registered provider could not be reached.
 */
export interface ProviderModelGroup {
  provider: string;
  is_default: boolean;
  source: "discovered" | "static" | "stale";
  models: AvailableModel[];
  error?: string;
}

export interface AvailableModelsResponse {
  auto: { available: boolean; label: string };
  providers: ProviderModelGroup[];
}
