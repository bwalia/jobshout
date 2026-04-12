export type EngineType = "go_native" | "langchain" | "langgraph";

export interface WorkflowStep {
  id: string;
  workflow_id: string;
  name: string;
  agent_id: string;
  input_template: string;
  position: number;
  depends_on: string[];
  engine_type?: EngineType;
  created_at: string;
}

export interface Workflow {
  id: string;
  org_id: string;
  name: string;
  description: string | null;
  status: string;
  steps: WorkflowStep[];
  created_by: string | null;
  created_at: string;
  updated_at: string;
}

export interface WorkflowRun {
  id: string;
  workflow_id: string;
  org_id: string;
  status: string;
  input: Record<string, unknown>;
  outputs: Record<string, string>;
  error_message: string | null;
  started_at: string | null;
  completed_at: string | null;
  triggered_by: string | null;
  created_at: string;
}

export interface CreateWorkflowStepRequest {
  name: string;
  agent_id: string;
  input_template: string;
  position: number;
  depends_on: string[];
  engine_type?: EngineType;
}

export interface CreateWorkflowRequest {
  name: string;
  description?: string;
  steps: CreateWorkflowStepRequest[];
}

export interface UpdateWorkflowRequest {
  name?: string;
  description?: string;
  status?: string;
}

export interface ExecuteWorkflowRequest {
  input?: Record<string, unknown>;
}

// Plugin types
export interface Plugin {
  id: string;
  org_id: string;
  name: string;
  version: string;
  description: string | null;
  status: string;
  plugin_type: string;
  workflow_def: Record<string, unknown>;
  permissions: string[];
  config: Record<string, unknown>;
  created_by: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreatePluginRequest {
  name: string;
  version?: string;
  description?: string;
  plugin_type?: string;
  workflow_def: Record<string, unknown>;
  permissions?: string[];
  config?: Record<string, unknown>;
}

export interface UpdatePluginRequest {
  name?: string;
  version?: string;
  description?: string;
  status?: string;
  workflow_def?: Record<string, unknown>;
  permissions?: string[];
  config?: Record<string, unknown>;
}

export interface PluginExecution {
  id: string;
  plugin_id: string;
  execution_id: string | null;
  org_id: string;
  input: Record<string, unknown>;
  output: string | null;
  status: string;
  error_message: string | null;
  started_at: string | null;
  completed_at: string | null;
  created_at: string;
}

// Engine types
export interface EngineInfo {
  name: string;
  status: string;
  available: boolean;
}

// Graph builder types (for the visual builder)
export interface GraphNode {
  id: string;
  type: "llm" | "tool" | "decision" | "agent" | "start" | "end";
  name: string;
  config?: Record<string, unknown>;
  position?: { x: number; y: number };
}

export interface GraphEdge {
  id: string;
  from: string;
  to: string;
  label?: string;
}

export interface GraphDefinition {
  nodes: GraphNode[];
  edges: GraphEdge[];
  entry_point?: string;
}
