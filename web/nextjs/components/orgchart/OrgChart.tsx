"use client";

import { useCallback, useState } from "react";
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  addEdge,
  useNodesState,
  useEdgesState,
  type Connection,
  type Edge,
  type NodeTypes,
} from "reactflow";
import "reactflow/dist/style.css";

import { AgentNode } from "@/components/orgchart/AgentNode";
import { useOrgChart } from "@/lib/hooks/useOrgChart";
import type { Agent } from "@/lib/types/agent";

// Register the custom node type so React Flow can render it
const nodeTypes: NodeTypes = {
  agentNode: AgentNode,
};

// ---------------------------------------------------------------------------
// Internal save-status indicator
// ---------------------------------------------------------------------------

interface SaveStatusProps {
  isSaving: boolean;
}

function SaveStatus({ isSaving }: SaveStatusProps) {
  return (
    <span className="text-xs text-muted-foreground">
      {isSaving ? "Saving…" : "All changes saved"}
    </span>
  );
}

// ---------------------------------------------------------------------------
// OrgChart
// ---------------------------------------------------------------------------

interface OrgChartProps {
  /** List of agents already fetched by the parent page */
  agents: Agent[];
}

/**
 * Full-screen React Flow canvas for the organisation chart.
 *
 * Features:
 * - Automatic Dagre layout on first render
 * - Drag-to-connect creates a new reporting-line edge
 * - Delete an edge to remove the reporting line
 * - "Save" button persists the current edge set via PUT /organizations/{orgId}/chart
 */
export function OrgChart({ agents: _agents }: OrgChartProps) {
  const { initialNodes, initialEdges, isSaving, saveChart } = useOrgChart();

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  // Re-initialise the canvas when the agents data changes (e.g. after a save
  // that updates manager_id values on the backend).
  // Using a key on the ReactFlow wrapper would cause an unmount/remount cycle,
  // so we instead reset state when initialNodes/initialEdges change via
  // the hook itself (which is triggered by the query invalidation in saveChart).

  // Called when the user drags a connection between two Handles
  const onConnect = useCallback(
    (connection: Connection) => {
      setEdges((currentEdges) =>
        addEdge(
          {
            ...connection,
            type: "smoothstep",
            id: `${connection.source}->${connection.target}`,
          },
          currentEdges
        )
      );
    },
    [setEdges]
  );

  // Called when the user selects an edge and presses Delete / Backspace
  const onEdgesDelete = useCallback(
    (deletedEdges: Edge[]) => {
      // React Flow's built-in delete behaviour handles removing the edges from
      // state; we only need to handle any side-effect logic here if required.
      // The edges state is already updated by ReactFlow before this callback fires.
      void deletedEdges;
    },
    []
  );

  function handleSave() {
    saveChart(edges);
  }

  return (
    <div className="relative h-full w-full rounded-xl border border-border bg-background">
      {/* Toolbar */}
      <div className="absolute left-4 top-4 z-10 flex items-center gap-3">
        <button
          type="button"
          onClick={handleSave}
          disabled={isSaving}
          className="inline-flex h-9 items-center gap-2 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:pointer-events-none disabled:opacity-50"
        >
          {isSaving ? (
            <>
              <svg
                className="h-4 w-4 animate-spin"
                xmlns="http://www.w3.org/2000/svg"
                fill="none"
                viewBox="0 0 24 24"
              >
                <circle
                  className="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  strokeWidth="4"
                />
                <path
                  className="opacity-75"
                  fill="currentColor"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
                />
              </svg>
              Saving…
            </>
          ) : (
            "Save"
          )}
        </button>
        <SaveStatus isSaving={isSaving} />
      </div>

      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onEdgesDelete={onEdgesDelete}
        deleteKeyCode="Delete"
        fitView
        fitViewOptions={{ padding: 0.2 }}
        className="rounded-xl"
      >
        {/* Dotted background grid */}
        <Background gap={20} size={1} color="hsl(var(--border))" />
        {/* Zoom / pan controls */}
        <Controls className="[&>button]:border-border [&>button]:bg-card [&>button]:text-foreground" />
        {/* Mini-map overview */}
        <MiniMap
          nodeColor="hsl(var(--primary))"
          maskColor="hsl(var(--background)/80)"
          className="!rounded-lg !border !border-border"
        />
      </ReactFlow>
    </div>
  );
}
