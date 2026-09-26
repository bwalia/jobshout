import { apiClient } from "@/lib/api/client";
import type { WireSchema } from "@/lib/agents/input-schemas";

export async function getAgentSchemas(): Promise<WireSchema[]> {
  const { data } = await apiClient.get<WireSchema[]>("/agent-schemas");
  return data ?? [];
}
