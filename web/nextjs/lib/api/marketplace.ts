import { apiClient } from "./client";

export interface MarketplaceAgent {
  id: string;
  name: string;
  description: string;
  category: string;
  model_provider: string;
  model_name: string;
  system_prompt: string;
  author_id: string | null;
  downloads: number;
  rating: number;
  is_public: boolean;
  created_at: string;
  updated_at: string;
}

export async function getMarketplaceAgents(
  category?: string
): Promise<MarketplaceAgent[]> {
  const params = category ? { category } : {};
  const { data } = await apiClient.get<MarketplaceAgent[]>("/marketplace", {
    params,
  });
  return data;
}

export async function getMarketplaceAgent(
  id: string
): Promise<MarketplaceAgent> {
  const { data } = await apiClient.get<MarketplaceAgent>(
    `/marketplace/${id}`
  );
  return data;
}

export async function importMarketplaceAgent(
  id: string
): Promise<{ id: string; message: string }> {
  const { data } = await apiClient.post<{ id: string; message: string }>(
    `/marketplace/${id}/import`
  );
  return data;
}
