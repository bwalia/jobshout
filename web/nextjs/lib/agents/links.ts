/** The agent profile's tabs, as they appear in ?tab=. */
export type AgentTab =
  | "overview"
  | "models"
  | "workspace"
  | "tasks"
  | "metrics"
  | "knowledge"
  | "skills";

/** The agent profile URL, optionally on a tab. Every agent link uses this. */
export function agentHref(id: string, tab?: AgentTab): string {
  return `/panel/agents/${id}${tab && tab !== "overview" ? `?tab=${tab}` : ""}`;
}

/** New task with the agent preselected — where the profile's "New task" goes. */
export function newTaskHref(agentId?: string): string {
  return agentId ? `/panel/tasks?new=1&agent=${agentId}` : "/panel/tasks?new=1";
}
