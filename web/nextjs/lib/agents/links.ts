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
