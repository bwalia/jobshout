/** The agent profile's tabs, as they appear in ?tab=. */
export type AgentTab =
  | "overview"
  | "models"
  | "workspace"
  | "tasks"
  | "metrics"
  | "knowledge"
  | "skills";

/** Tabs that start or track work — always first-class on the profile. */
export const AGENT_PRIMARY_TABS: readonly AgentTab[] = [
  "overview",
  "workspace",
  "tasks",
];

/** Setup / inspect tabs — grouped under Configure so they aren't peers of Workspace. */
export const AGENT_CONFIGURE_TABS: readonly AgentTab[] = [
  "models",
  "knowledge",
  "skills",
  "metrics",
];

/**
 * The agent profile URL, optionally on a tab. Every agent link uses this.
 * Explicit `tab=overview` is kept in the query so specialists that default to
 * Workspace do not bounce back when the user chooses Overview.
 */
export function agentHref(id: string, tab?: AgentTab): string {
  if (!tab) return `/panel/agents/${id}`;
  return `/panel/agents/${id}?tab=${tab}`;
}

/** New task with the agent preselected — where the profile's "New task" goes. */
export function newTaskHref(agentId?: string): string {
  return agentId ? `/panel/tasks?new=1&agent=${agentId}` : "/panel/tasks?new=1";
}

/**
 * Open Chat with a draft that names this agent. Chat drives agents via
 * agent_execute; the profile CTA should land there in one click.
 */
export function askInChatHref(agentName: string): string {
  const ask = `Help me work with ${agentName}`;
  return `/chat?ask=${encodeURIComponent(ask)}`;
}
