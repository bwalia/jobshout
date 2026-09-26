import { apiClient } from "@/lib/api/client";

export type AgentActivityKind = "job" | "blog" | "mail" | "task_run";

/** One row on the agent board — matches `model.AgentBoardEntry` on the server. */
export interface AgentBoardEntry {
  agent_id: string;
  name: string;
  role: string;
  avatar_url: string | null;
  activity: AgentActivity;
  activity_kind?: AgentActivityKind;
  current_job_id?: string;
  task_id?: string;
  job_role?:
    | "planner"
    | "executor"
    | "reviewer"
    | "writer"
    | "mail"
    | "researcher";
  /**
   * What the agent is doing: the task prompt for a collaboration job, or the
   * label of the running step for an article run.
   */
  current_job_prompt?: string;
  last_active_at?: string;
}

/** Deep link for a live card. Idle cards and multi-agent jobs have no page. */
export function agentBoardHref(entry: AgentBoardEntry): string | null {
  if (entry.activity === "idle" || !entry.current_job_id) return null;
  switch (entry.activity_kind) {
    case "task_run":
      return entry.task_id
        ? `/panel/task-board?task=${entry.task_id}&run=${entry.current_job_id}`
        : null;
    case "blog":
      return `/articles/${entry.current_job_id}`;
    case "mail":
      return `/panel/task-manager?agent=mail&thread=${entry.current_job_id}`;
    default:
      return null;
  }
}

/**
 * Board columns. Must stay in step with the Activity* constants in
 * server/internal/model/multi_agent.go — the board drops any activity that has
 * no column here, which would make the agent disappear rather than error.
 */
export type AgentActivity =
  | "idle"
  | "planning"
  | "executing"
  | "reviewing"
  | "publishing"
  | "failed";

/**
 * Fetch the live agent board — every agent in the org with the activity
 * inferred from its most recent multi-agent collaboration.
 */
export async function getAgentBoard(): Promise<AgentBoardEntry[]> {
  const { data } = await apiClient.get<AgentBoardEntry[]>("/agents/board");
  return data;
}
