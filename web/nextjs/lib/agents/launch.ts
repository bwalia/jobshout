import {
  defaultValuesForSchema,
  type AgentInputSchema,
} from "@/lib/agents/input-schemas";
import { apiClient } from "@/lib/api/client";
import type { Agent } from "@/lib/types/agent";
import type { Task } from "@/lib/types/project";

interface ResearchBrief {
  topic?: string;
  summary?: string;
  findings?: { claim?: string; source_url?: string }[];
  sources?: { url?: string; title?: string }[];
}

/** Unified result from POST /tasks/launch (Task Manager and chat share this). */
export interface LaunchResult {
  kind: string;
  task: Task;
  run_id?: string | null;
  evaluation_id?: string | null;
  sync_queued?: boolean;
  brief?: ResearchBrief;
  image_url?: string;
  message?: string;
}

interface LaunchAPIResponse {
  task: Task;
  kind: string;
  run_id?: string | null;
  evaluation_id?: string | null;
  sync_queued?: boolean;
  brief?: ResearchBrief;
  image_url?: string;
  message?: string;
}

/** Last specialist form values stored on the board task. */
export function launchValuesFromTask(
  task: Task | null | undefined
): Record<string, string> {
  const raw = task?.metadata?.launch_values;
  if (!raw || typeof raw !== "object") return {};
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
    if (v == null) continue;
    out[k] = String(v);
  }
  return out;
}

/** Stored launch_values, then title/description for generic tasks, then schema defaults. */
export function hydrateLaunchValues(
  task: Task | null | undefined,
  schema: AgentInputSchema
): Record<string, string> {
  return {
    ...defaultValuesForSchema(schema),
    ...deriveLaunchValues(task, schema),
    ...launchValuesFromTask(task),
  };
}

function deriveLaunchValues(
  task: Task | null | undefined,
  schema: AgentInputSchema
): Record<string, string> {
  if (!task) return {};
  if (schema.kind === "task_run") {
    const title = (task.title ?? "").trim();
    const desc = (task.description ?? "").trim();
    return {
      title,
      ...(desc ? { description: desc } : {}),
    };
  }
  return {};
}

/**
 * Start an agent through the server launcher. Creates or updates the board
 * task, then dispatches the specialist. One source of truth with chat.
 */
export async function launchAgent(opts: {
  agent: Agent;
  projectId: string;
  taskId?: string;
  values: Record<string, string>;
}): Promise<LaunchResult> {
  const { data } = await apiClient.post<LaunchAPIResponse>(
    "/tasks/launch",
    {
      agent_id: opts.agent.id,
      project_id: opts.projectId,
      task_id: opts.taskId,
      values: opts.values,
    },
    { timeout: 180_000 }
  );
  return {
    kind: data.kind,
    task: data.task,
    run_id: data.run_id,
    evaluation_id: data.evaluation_id,
    sync_queued: data.sync_queued,
    brief: data.brief,
    image_url: data.image_url,
    message: data.message,
  };
}

/** After the board task exists, kick off the executor for the chosen agent. */
export async function launchAgentForTask(opts: {
  agent: Agent;
  task: Task;
  values: Record<string, string>;
}): Promise<LaunchResult> {
  return launchAgent({
    agent: opts.agent,
    projectId: opts.task.project_id,
    taskId: opts.task.id,
    values: opts.values,
  });
}
