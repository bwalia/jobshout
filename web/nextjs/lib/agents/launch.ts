import { apiClient } from "@/lib/api/client";
import type { AgentInputSchema } from "@/lib/agents/input-schemas";
import { agentRunInputs } from "@/lib/agents/input-schemas";
import type { Agent } from "@/lib/types/agent";
import type { Task } from "@/lib/types/project";

/** One execution of one agent — mirrors `model.AgentRun` on the server. */
export interface AgentRun {
  id: string;
  org_id: string;
  agent_id: string;
  task_id?: string;
  builtin?: string;
  source: string;
  status: "queued" | "running" | "completed" | "failed";
  external_run_id?: string;
  external_kind?: string;
  error_message?: string;
  created_at: string;
}

/** 202 body of POST /agent-runs. */
export interface LaunchResult {
  /** The schema kind that was launched, so callers can word their own toast. */
  kind: AgentInputSchema["kind"];
  run: AgentRun;
  agent: string;
  task: Task;
}

/** 400 body when a required input is empty — the same shape chat renders. */
export interface MissingInput {
  missing: string[];
  question: string;
  options?: { label: string; value: string }[];
}

/**
 * Start an agent for a board task.
 *
 * This used to be a switch: a branch per agent, each posting to a different
 * endpoint from the browser. That meant the server had no single entry point
 * for "run agent X" — chat had to reimplement the fan-out, a closed tab could
 * lose a run, and most run types never reached the agent board because the
 * board reads specialist tables and each branch wrote to a different one.
 *
 * There is now one call. The server resolves the agent's builtin, validates the
 * inputs against the same schema this form was rendered from, records the run
 * and dispatches it.
 */
export async function launchAgentForTask(opts: {
  agent: Agent;
  task: Task;
  schema: AgentInputSchema;
  values: Record<string, string>;
}): Promise<LaunchResult> {
  const { agent, task, schema, values } = opts;

  const { data } = await apiClient.post<{
    run: AgentRun;
    agent: string;
    kind: string;
  }>("/agent-runs", {
    agent_id: agent.id,
    task_id: task.id,
    inputs: agentRunInputs(schema, values),
  });

  return { kind: schema.kind, run: data.run, agent: data.agent, task };
}

/**
 * Read a 400 from `launchAgentForTask` as a missing-input prompt.
 *
 * Returns null for any other failure, so a caller can fall back to showing the
 * error rather than inventing a field to blame.
 */
export function asMissingInput(err: unknown): MissingInput | null {
  const body = (err as { response?: { status?: number; data?: unknown } })
    ?.response;
  if (body?.status !== 400) return null;
  const data = body.data as Partial<MissingInput> | undefined;
  if (!data || !Array.isArray(data.missing) || data.missing.length === 0) {
    return null;
  }
  return {
    missing: data.missing,
    question: data.question ?? "That field is required.",
    options: data.options,
  };
}
