"use client";

import { useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { useState } from "react";
import { WorkflowBuilder } from "@/components/workflow/WorkflowBuilder";
import { useCreateWorkflow } from "@/lib/hooks/useWorkflows";
import type { GraphDefinition } from "@/lib/types/workflow";

export default function NewWorkflowPage() {
  const router = useRouter();
  const createWorkflow = useCreateWorkflow();
  const [workflowName, setWorkflowName] = useState("Untitled Workflow");
  const [workflowDescription, setWorkflowDescription] = useState("");

  const handleSave = (graph: GraphDefinition) => {
    createWorkflow.mutate(
      {
        name: workflowName,
        description: workflowDescription || undefined,
        steps: graph.nodes.map((node, index) => ({
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
          engine_type: (node.config?.engine_type as "go_native" | "langchain" | "langgraph") || undefined,
        })),
      },
      {
        onSuccess: () => router.push("/workflows"),
      }
    );
  };

  return (
    <div className="flex h-[calc(100vh-4rem)] flex-col">
      <div className="flex items-center gap-4 border-b border-border px-4 py-3">
        <button
          onClick={() => router.push("/workflows")}
          className="rounded-md p-1 text-muted-foreground hover:text-foreground transition-colors"
        >
          <ArrowLeft className="h-5 w-5" />
        </button>

        <div className="flex flex-1 items-center gap-3">
          <input
            value={workflowName}
            onChange={(e) => setWorkflowName(e.target.value)}
            className="border-none bg-transparent text-lg font-semibold text-foreground outline-none focus:ring-0"
            placeholder="Workflow name"
          />
          <input
            value={workflowDescription}
            onChange={(e) => setWorkflowDescription(e.target.value)}
            className="border-none bg-transparent text-sm text-muted-foreground outline-none focus:ring-0 flex-1"
            placeholder="Add a description..."
          />
        </div>
      </div>

      <div className="flex-1">
        <WorkflowBuilder onSave={handleSave} />
      </div>
    </div>
  );
}
