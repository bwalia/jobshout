import dagre from "@dagrejs/dagre";
import type { Node, Edge } from "reactflow";

/** Width and height used for every agent node in the layout algorithm */
const NODE_WIDTH = 220;
const NODE_HEIGHT = 90;

/**
 * Applies a Dagre hierarchical layout to the provided React Flow nodes and
 * edges, returning a new nodes array with computed `position.x` / `position.y`
 * values.  The original `nodes` and `edges` arrays are not mutated.
 *
 * @param nodes - React Flow node array (without positions)
 * @param edges - React Flow edge array describing reporting lines
 * @param direction - 'TB' (top-to-bottom) or 'LR' (left-to-right); defaults to 'TB'
 */
export function applyDagreLayout(
  nodes: Node[],
  edges: Edge[],
  direction: "TB" | "LR" = "TB"
): Node[] {
  // Create a fresh directed graph for each layout pass to avoid stale state
  const graph = new dagre.graphlib.Graph();
  graph.setDefaultEdgeLabel(() => ({}));
  graph.setGraph({
    rankdir: direction,
    // Extra space between sibling nodes and between ranks (levels)
    nodesep: 60,
    ranksep: 80,
  });

  // Register every node with its display dimensions
  nodes.forEach((node) => {
    graph.setNode(node.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
  });

  // Register every reporting-line edge
  edges.forEach((edge) => {
    graph.setEdge(edge.source, edge.target);
  });

  // Run the Dagre layout algorithm
  dagre.layout(graph);

  // Map computed positions back onto a copy of each node
  return nodes.map((node) => {
    const dagreNode = graph.node(node.id);

    return {
      ...node,
      // Dagre positions use the node centre; React Flow uses the top-left corner
      position: {
        x: dagreNode.x - NODE_WIDTH / 2,
        y: dagreNode.y - NODE_HEIGHT / 2,
      },
    };
  });
}
