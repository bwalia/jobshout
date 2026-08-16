import { apiClient } from "@/lib/api/client";
import type {
  AvailableModelsResponse,
  LLMProviderConfig,
  CreateLLMProviderRequest,
  UpdateLLMProviderRequest,
  BuiltinProvider,
} from "@/lib/types/llm-provider";

export async function getBuiltinProviders(): Promise<BuiltinProvider[]> {
  const { data } = await apiClient.get<BuiltinProvider[]>("/llm-providers/builtin");
  return data;
}

export async function getLLMProviders(): Promise<LLMProviderConfig[]> {
  const { data } = await apiClient.get<LLMProviderConfig[]>("/llm-providers");
  return data;
}

export async function getLLMProvider(id: string): Promise<LLMProviderConfig> {
  const { data } = await apiClient.get<LLMProviderConfig>(`/llm-providers/${id}`);
  return data;
}

export async function createLLMProvider(
  payload: CreateLLMProviderRequest
): Promise<LLMProviderConfig> {
  const { data } = await apiClient.post<LLMProviderConfig>("/llm-providers", payload);
  return data;
}

export async function updateLLMProvider(
  id: string,
  payload: UpdateLLMProviderRequest
): Promise<LLMProviderConfig> {
  const { data } = await apiClient.put<LLMProviderConfig>(`/llm-providers/${id}`, payload);
  return data;
}

export async function deleteLLMProvider(id: string): Promise<void> {
  await apiClient.delete(`/llm-providers/${id}`);
}

/**
 * Fetch the models each registered provider can actually run. Populates the
 * per-agent model picker.
 */
export async function getAvailableProviderModels(): Promise<AvailableModelsResponse> {
  const { data } = await apiClient.get<AvailableModelsResponse>("/llm-providers/models");
  return data;
}
