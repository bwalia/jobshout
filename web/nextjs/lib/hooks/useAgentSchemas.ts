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
    // Task Manager rail and specialist Run/New-task forms cannot proceed
    // without this list. Retry more than the app default of 1.
    retry: 2,
  });
}
