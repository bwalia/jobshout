import { apiClient } from "@/lib/api/client";
import type {
  AvailableModelsResponse,
  LLMProviderConfig,
  CreateLLMProviderRequest,
  UpdateLLMProviderRequest,
  BuiltinProvider,
  LLMRouting,
  ProviderTestResult,
  PutLLMRoutingRequest,
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

/** Sends one tiny prompt to a configured provider and reports whether it answered. */
export async function testLLMProvider(id: string, model?: string): Promise<ProviderTestResult> {
  const { data } = await apiClient.post<ProviderTestResult>(`/llm-providers/${id}/test`, model ? { model } : {});
  return data;
}

export async function getLLMRouting(): Promise<LLMRouting> {
  const { data } = await apiClient.get<LLMRouting>("/llm-routing");
  return data;
}

export async function putLLMRouting(payload: PutLLMRoutingRequest): Promise<LLMRouting> {
  const { data } = await apiClient.put<LLMRouting>("/llm-routing", payload);
  return data;
}
