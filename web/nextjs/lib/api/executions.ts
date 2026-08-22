import { apiClient } from "@/lib/api/client";

/** A single tool invocation captured during an agent execution. */
export interface ExecutionToolCall {
  id: string;
  execution_id: string;
  tool_name: string;
  input: Record<string, unknown>;
  output: string | null;
  error_message: string | null;
  duration_ms: number;
  called_at: string;
}

/**
 * A single agent execution — the rich telemetry a chat turn references via
 * metadata.execution_id. Mirrors server/internal/model/execution.go.
 */
export interface AgentExecution {
  id: string;
  agent_id: string;
  org_id: string;
  input_prompt: string;
  output: string | null;
  status: "pending" | "running" | "completed" | "failed";
  error_message: string | null;
  total_tokens: number;
  input_tokens: number;
  output_tokens: number;
  latency_ms: number;
  cost_usd: number;
  model_name: string | null;
  model_provider: string | null;
  iterations: number;
  engine_type: string;
  started_at: string | null;
  completed_at: string | null;
  created_at: string;
  tool_calls?: ExecutionToolCall[];
}

/** Fetch a single execution, including its tool-call timeline. */
export async function getExecution(id: string): Promise<AgentExecution> {
  const { data } = await apiClient.get<AgentExecution>(`/executions/${id}`);
  return data;
}
