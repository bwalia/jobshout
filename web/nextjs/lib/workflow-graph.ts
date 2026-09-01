import type {
  CreateWorkflowStepRequest,
  EngineType,
  GraphDefinition,
} from "@/lib/types/workflow";

/**
 * Converts the visual builder's graph into the step list the workflows API
 * accepts. Dependencies are expressed by step name, which is how the server
 * resolves them within a workflow.
 */
export function graphToSteps(graph: GraphDefinition): CreateWorkflowStepRequest[] {
  return graph.nodes.map((node, index) => ({
    name: node.name,
    agent_id: (node.config?.agent_id as string) || "",
    input_template: (node.config?.input_template as string) || "",
    position: index,
    depends_on: graph.edges
      .filter((edge) => edge.to === node.id)
      .map((edge) => {
        const sourceNode = graph.nodes.find((n) => n.id === edge.from);
        return sourceNode?.name ?? edge.from;
      }),
    engine_type: (node.config?.engine_type as EngineType) || undefined,
  }));
}
