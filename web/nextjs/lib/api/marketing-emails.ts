import { apiClient } from "@/lib/api/client";
import type { MarketingEmailRun } from "@/lib/types/marketing-email";

export async function listMarketingEmailRuns(): Promise<MarketingEmailRun[]> {
  const { data } = await apiClient.get<{ data: MarketingEmailRun[] }>("/marketing-emails/runs");
  return Array.isArray(data.data) ? data.data : [];
}

export async function getMarketingEmailRun(id: string): Promise<MarketingEmailRun> {
  const { data } = await apiClient.get<MarketingEmailRun>(
    `/marketing-emails/runs/${encodeURIComponent(id)}`
  );
  return data;
}

export async function regenerateMarketingEmailRun(id: string): Promise<MarketingEmailRun> {
  const { data } = await apiClient.post<MarketingEmailRun>(
    `/marketing-emails/runs/${encodeURIComponent(id)}/regenerate`
  );
  return data;
}

export async function unpublishMarketingEmailRun(id: string): Promise<MarketingEmailRun> {
  const { data } = await apiClient.post<MarketingEmailRun>(
    `/marketing-emails/runs/${encodeURIComponent(id)}/unpublish`
  );
  return data;
}

export async function saveMarketingEmailTemplate(id: string): Promise<MarketingEmailRun> {
  const { data } = await apiClient.post<MarketingEmailRun>(
    `/marketing-emails/runs/${encodeURIComponent(id)}/template`
  );
  return data;
}
