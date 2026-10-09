import { useCallback, useMemo } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNodes, useEdges } from "reactflow";
import { toast } from "sonner";
import { updateOrgChart, type OrgChartEntry } from "@/lib/api/organizations";
import { useAgents } from "@/lib/hooks/useAgents";
import { useAuthStore } from "@/lib/store/auth-store";
import { applyDagreLayout } from "@/lib/utils/org-chart-layout";
import type { Agent } from "@/lib/types/agent";
import type { Node, Edge } from "reactflow";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Data payload stored on each agentNode */
export interface AgentNodeData {
  agent: Agent;
}

// ---------------------------------------------------------------------------
// Conversion helpers
// ---------------------------------------------------------------------------

/**
 * Converts an array of Agent objects into React Flow nodes and edges.
 *
 * Each agent becomes a node; a `manager_id` relationship becomes a directed
 * edge from manager → report.
 */
function agentsToFlowElements(agents: Agent[]): {
  nodes: Node<AgentNodeData>[];
  edges: Edge[];
} {
  const rawNodes: Node<AgentNodeData>[] = agents.map((agent) => ({
    id: agent.id,
    type: "agentNode",
    // Positions are placeholder values; applyDagreLayout will overwrite them
    position: { x: 0, y: 0 },
    data: { agent },
  }));

  const edges: Edge[] = agents
    .filter((agent): agent is Agent & { manager_id: string } =>
      agent.manager_id !== null
    )
    .map((agent) => ({
      id: `${agent.manager_id}->${agent.id}`,
      source: agent.manager_id,
      target: agent.id,
      type: "smoothstep",
    }));

  // Apply automatic hierarchical layout before returning
  const nodes = applyDagreLayout(rawNodes, edges);

  return { nodes, edges };
}

/**
 * Converts the canvas edges back into one reporting line per agent. Every
 * agent is included so that a deleted edge clears its manager_id.
 */
function edgesToChartEntries(agents: Agent[], edges: Edge[]): OrgChartEntry[] {
  const managerOf = new Map(edges.map((edge) => [edge.target, edge.source]));
  return agents.map((agent) => ({
    agent_id: agent.id,
    manager_id: managerOf.get(agent.id) ?? null,
  }));
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

/**
 * Wraps `useAgents` to expose React Flow–ready nodes/edges for the org chart,
 * plus a `saveChart` mutation that persists the current edge set to the API.
 */
export function useOrgChart() {
  const user = useAuthStore((s) => s.user);
  const orgId = user?.org_id ?? "";
  const queryClient = useQueryClient();

  // Fetch agents list via the existing useAgents hook
  const { data, isLoading, isError } = useAgents();
  const agents = data?.data ?? [];

  // Derive the initial React Flow elements from the agents data
  const initialElements = useMemo(
    () => agentsToFlowElements(agents),
    [agents]
  );

  // Mutation that serialises the current edges and sends them to the backend
  const saveMutation = useMutation({
    mutationFn: (edges: Edge[]) =>
      updateOrgChart(orgId, edgesToChartEntries(agents, edges)),
    onSuccess: () => {
      toast.success("Organisation chart saved.");
      // Invalidate agents so manager_id values stay fresh
      queryClient.invalidateQueries({ queryKey: ["agents"] });
    },
    onError: (error: Error) => {
      toast.error(`Failed to save chart: ${error.message}`);
    },
  });

  const saveChart = useCallback(
    (edges: Edge[]) => saveMutation.mutate(edges),
    [saveMutation]
  );

  return {
    initialNodes: initialElements.nodes,
    initialEdges: initialElements.edges,
    isLoading,
    isError,
    saveChart,
    isSaving: saveMutation.isPending,
  };
}
