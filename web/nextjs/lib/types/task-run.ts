export type TaskRunStatus = "queued" | "running" | "completed" | "failed";

/**
 * One on-demand execution of a board task by an agent, launched from the Task
 * Manager. Mirrors server/internal/model/task_run.go. The heavy execution
 * telemetry (tool calls, per-iteration detail) lives on the linked
 * AgentExecution via execution_id.
 */
export interface TaskRun {
  id: string;
  task_id: string;
  agent_id: string;
  org_id: string;
  execution_id: string | null;
  status: TaskRunStatus;
  prompt: string;
  engine: string | null;
  model_provider: string | null;
  model_name: string | null;
  skill_slugs: string[];
  inputs: Record<string, unknown>;
  debug: boolean;
  output: string | null;
  error_message: string | null;
  total_tokens: number;
  cost_usd: number;
  latency_ms: number;
  iterations: number;
  requested_by: string | null;
  started_at: string | null;
  completed_at: string | null;
  created_at: string;
  updated_at: string;
}

/**
 * Body of POST /api/v1/tasks/{taskID}/run. Every field is an override: with an
 * empty body the run uses the task's assigned agent, the task's title +
 * description as the prompt, and the agent's own model, engine and skills.
 */
export interface CreateTaskRunRequest {
  /** Override which agent runs the task. Defaults to the task's assigned agent. */
  agent_id?: string;
  /** Fully replace the derived prompt (title + description). Inputs still append. */
  prompt?: string;
  /** Override the execution engine for this run only. */
  engine?: "go_native" | "langchain" | "langgraph";
  /** Override the agent's model for this run only. */
  model_provider?: string;
  model_name?: string;
  /** Extra skills to load for this run, by slug, on top of the agent's own. */
  skill_slugs?: string[];
  /** Free-form key/value inputs appended to the prompt as context. */
  inputs?: Record<string, unknown>;
  /** Surface the full engine trace for this run. */
  debug?: boolean;
}

export const TASK_RUN_ACTIVE: TaskRunStatus[] = ["queued", "running"];

export function isTaskRunActive(status: TaskRunStatus): boolean {
  return TASK_RUN_ACTIVE.includes(status);
}
