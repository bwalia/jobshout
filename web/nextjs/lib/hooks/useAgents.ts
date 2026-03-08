import {
  useQuery,
  useMutation,
  useQueryClient,
  type UseQueryResult,
  type UseMutationResult,
} from "@tanstack/react-query";
import { toast } from "sonner";
import {
  getAgents,
  getAgent,
  createAgent,
  updateAgent,
  deleteAgent,
  type AgentListParams,
} from "@/lib/api/agents";
import type { Agent, CreateAgentRequest, UpdateAgentRequest } from "@/lib/types/agent";
import type { PaginatedResponse } from "@/lib/types/common";

// ---------------------------------------------------------------------------
// Query key factory – keeps cache keys consistent across the app
// ---------------------------------------------------------------------------
export const agentKeys = {
  all: ["agents"] as const,
  lists: () => [...agentKeys.all, "list"] as const,
  list: (params: AgentListParams) => [...agentKeys.lists(), params] as const,
  details: () => [...agentKeys.all, "detail"] as const,
  detail: (id: string) => [...agentKeys.details(), id] as const,
};

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

/**
 * Returns a paginated list of agents, optionally filtered by status / role /
 * search term.
 */
export function useAgents(
  params: AgentListParams = {}
): UseQueryResult<PaginatedResponse<Agent>> {
  return useQuery({
    queryKey: agentKeys.list(params),
    queryFn: () => getAgents(params),
  });
}

/**
 * Returns a single agent by ID.
 */
export function useAgent(id: string): UseQueryResult<Agent> {
  return useQuery({
    queryKey: agentKeys.detail(id),
    queryFn: () => getAgent(id),
    // Only run when we have a valid id
    enabled: Boolean(id),
  });
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

/**
 * Creates a new agent and invalidates the agents list cache on success.
 */
export function useCreateAgent(): UseMutationResult<
  Agent,
  Error,
  CreateAgentRequest
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: createAgent,
    onSuccess: (newAgent) => {
      // Invalidate list so any open agent list re-fetches
      queryClient.invalidateQueries({ queryKey: agentKeys.lists() });
      toast.success(`Agent "${newAgent.name}" created successfully.`);
    },
    onError: (error: Error) => {
      toast.error(`Failed to create agent: ${error.message}`);
    },
  });
}

/**
 * Updates an existing agent by ID and refreshes its cached detail entry.
 */
export function useUpdateAgent(): UseMutationResult<
  Agent,
  Error,
  { id: string; payload: UpdateAgentRequest }
> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, payload }) => updateAgent(id, payload),
    onSuccess: (updatedAgent) => {
      // Refresh the specific agent detail
      queryClient.invalidateQueries({
        queryKey: agentKeys.detail(updatedAgent.id),
      });
      // Also refresh any lists so status badges, etc. stay current
      queryClient.invalidateQueries({ queryKey: agentKeys.lists() });
      toast.success(`Agent "${updatedAgent.name}" updated.`);
    },
    onError: (error: Error) => {
      toast.error(`Failed to update agent: ${error.message}`);
    },
  });
}

/**
 * Deletes an agent by ID and clears its entry from the cache.
 */
export function useDeleteAgent(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: deleteAgent,
    onSuccess: (_data, deletedId) => {
      queryClient.removeQueries({ queryKey: agentKeys.detail(deletedId) });
      queryClient.invalidateQueries({ queryKey: agentKeys.lists() });
      toast.success("Agent deleted.");
    },
    onError: (error: Error) => {
      toast.error(`Failed to delete agent: ${error.message}`);
    },
  });
}
