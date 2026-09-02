import { useQuery, type UseQueryResult } from "@tanstack/react-query";
import { getAgentSchemas } from "@/lib/api/agent-schemas";
import type { WireSchema } from "@/lib/agents/input-schemas";

export const agentSchemaKeys = {
  all: ["agent-schemas"] as const,
};

export function useAgentSchemas(): UseQueryResult<WireSchema[]> {
  return useQuery({
    queryKey: agentSchemaKeys.all,
    queryFn: getAgentSchemas,
    staleTime: 5 * 60_000,
  });
}
