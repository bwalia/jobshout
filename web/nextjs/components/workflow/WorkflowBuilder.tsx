"use client";

import { useCallback, useEffect, useState } from "react";
import ReactFlow, {
  addEdge,
  Background,
  Controls,
  MiniMap,
  Panel,
  useEdgesState,
  useNodesState,
  type Connection,
  type Edge,
  type Node,
  type NodeTypes,
} from "reactflow";
import "reactflow/dist/style.css";

import { LLMNode } from "./nodes/LLMNode";
import { ToolNode } from "./nodes/ToolNode";
import { DecisionNode } from "./nodes/DecisionNode";
import { AgentNode } from "./nodes/AgentNode";
import { NodePalette } from "./NodePalette";
import type { GraphDefinition, GraphNode, GraphEdge } from "@/lib/types/workflow";

const nodeTypes: NodeTypes = {
  llm: LLMNode,
  tool: ToolNode,
  decision: DecisionNode,
  agent: AgentNode,
};

interface WorkflowBuilderProps {
  initialGraph?: GraphDefinition;
  onSave?: (graph: GraphDefinition) => void;
  readOnly?: boolean;
  executionStatus?: Record<string, "pending" | "running" | "completed" | "failed">;
}

let nodeIdCounter = 0;

export function WorkflowBuilder({
  initialGraph,
  onSave,
  readOnly = false,
  executionStatus,
}: WorkflowBuilderProps) {
  const initialNodes: Node[] = (initialGraph?.nodes || []).map((n) => ({
    id: n.id,
    type: n.type,
    position: n.position || { x: 250, y: Number(n.id) * 150 },
    data: {
      label: n.name,
      config: n.config || {},
      status: executionStatus?.[n.name],
    },
  }));

  const initialEdges: Edge[] = (initialGraph?.edges || []).map((e) => ({
    id: e.id || `${e.from}-${e.to}`,
    source: e.from,
    target: e.to,
    label: e.label,
    animated: true,
    style: { stroke: "#6366f1" },
  }));

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);

  // Sync executionStatus changes into node data
  useEffect(() => {
    if (!executionStatus) return;
    setNodes((nds) =>
      nds.map((node) => {
        const status = executionStatus[node.data.label as string];
        if (status !== node.data.status) {
          return { ...node, data: { ...node.data, status } };
        }
        return node;
      }),
    );
  }, [executionStatus, setNodes]);

  const onConnect = useCallback(
    (connection: Connection) => {
      setEdges((eds) =>
        addEdge({ ...connection, animated: true, style: { stroke: "#6366f1" } }, eds)
      );
    },
    [setEdges]
  );

  const onNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    setSelectedNode(node);
  }, []);

  const addNode = useCallback(
    (type: string) => {
      nodeIdCounter++;
      const id = `node-${nodeIdCounter}-${Date.now()}`;
      const newNode: Node = {
        id,
        type,
        position: { x: 250 + Math.random() * 200, y: 100 + Math.random() * 300 },
        data: {
          label: `${type.charAt(0).toUpperCase() + type.slice(1)} Node`,
          config: {},
        },
      };
      setNodes((nds) => [...nds, newNode]);
    },
    [setNodes]
  );

  const handleSave = useCallback(() => {
    const graphNodes: GraphNode[] = nodes.map((n) => ({
      id: n.id,
      type: (n.type as GraphNode["type"]) || "llm",
      name: n.data.label || n.id,
      config: n.data.config || {},
      position: n.position,
    }));

    const graphEdges: GraphEdge[] = edges.map((e) => ({
      id: e.id,
      from: e.source,
      to: e.target,
      label: typeof e.label === "string" ? e.label : undefined,
    }));

    const graph: GraphDefinition = {
      nodes: graphNodes,
      edges: graphEdges,
      entry_point: graphNodes[0]?.id,
    };

    onSave?.(graph);
  }, [nodes, edges, onSave]);

  const deleteNode = useCallback(() => {
    if (!selectedNode) return;
    setNodes((nds) => nds.filter((n) => n.id !== selectedNode.id));
    setEdges((eds) =>
      eds.filter((e) => e.source !== selectedNode.id && e.target !== selectedNode.id)
    );
    setSelectedNode(null);
  }, [selectedNode, setNodes, setEdges]);

  return (
    <div className="flex h-full w-full">
      {/* Node Palette */}
      {!readOnly && (
        <div className="w-56 border-r border-border bg-card p-4 space-y-2">
          <h3 className="text-sm font-semibold text-foreground mb-3">Node Types</h3>
          <NodePalette onAddNode={addNode} />

          <div className="pt-4 border-t border-border mt-4 space-y-2">
            {selectedNode && (
              <button
                onClick={deleteNode}
                className="w-full rounded-md bg-destructive px-3 py-2 text-sm text-destructive-foreground hover:bg-destructive/90"
              >
                Delete Selected
              </button>
            )}
            <button
              onClick={handleSave}
              className="w-full rounded-md bg-primary px-3 py-2 text-sm text-primary-foreground hover:bg-primary/90"
            >
              Save Workflow
            </button>
          </div>
        </div>
      )}

      {/* React Flow Canvas */}
      <div className="flex-1">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={readOnly ? undefined : onNodesChange}
          onEdgesChange={readOnly ? undefined : onEdgesChange}
          onConnect={readOnly ? undefined : onConnect}
          onNodeClick={onNodeClick}
          nodeTypes={nodeTypes}
          fitView
          deleteKeyCode={readOnly ? null : "Backspace"}
          className="bg-background"
        >
          <Background color="#333" gap={20} />
          <Controls />
          <MiniMap
            nodeColor={(n) => {
              const status = n.data?.status;
              if (status === "completed") return "#22c55e";
              if (status === "running") return "#eab308";
              if (status === "failed") return "#ef4444";
              switch (n.type) {
                case "llm":
                  return "#6366f1";
                case "tool":
                  return "#f59e0b";
                case "decision":
                  return "#ec4899";
                case "agent":
                  return "#06b6d4";
                default:
                  return "#6b7280";
              }
            }}
          />
          {readOnly && (
            <Panel position="top-right">
              <div className="rounded-md bg-card/80 px-3 py-1 text-xs text-muted-foreground backdrop-blur">
                Read-only mode
              </div>
            </Panel>
          )}
        </ReactFlow>
      </div>

      {/* Config Panel */}
      {selectedNode && !readOnly && (
        <div className="w-72 border-l border-border bg-card p-4 overflow-y-auto">
          <h3 className="text-sm font-semibold text-foreground mb-3">
            Node Config
          </h3>
          <div className="space-y-3">
            <div>
              <label className="text-xs text-muted-foreground">Name</label>
              <input
                className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
                value={selectedNode.data.label || ""}
                onChange={(e) => {
                  setNodes((nds) =>
                    nds.map((n) =>
                      n.id === selectedNode.id
                        ? { ...n, data: { ...n.data, label: e.target.value } }
                        : n
                    )
                  );
                }}
              />
            </div>
            <div>
              <label className="text-xs text-muted-foreground">Type</label>
              <p className="text-sm text-foreground">{selectedNode.type}</p>
            </div>
            <div>
              <label className="text-xs text-muted-foreground">ID</label>
              <p className="text-xs text-muted-foreground font-mono">
                {selectedNode.id}
              </p>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
