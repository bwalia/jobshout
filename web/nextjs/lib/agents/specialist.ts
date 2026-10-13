import type { Task } from "@/lib/types/project";

/**
 * Builtins whose Workspace tab is their own screen (lib/agents/tab-clients.tsx,
 * which must list exactly these — the compiler checks). Kept apart so a page
 * can ask "does this agent have a screen to send the user to?" without
 * importing every screen.
 */
export const AGENT_CLIENT_BUILTINS = [
  "career_ops",
  "mail",
  "pentester",
  "pr_reviewer",
  "credit_controller",
  "simpro_payments",
  "waf_lab",
  "seo_analyst",
  "secrets_rotation",
  "linux_patch",
  "ab_testing",
  "article_writer",
  "images",
  "course_generator",
  "jobshout_com_writer",
  "linkedin_poster",
  "video_producer",
] as const;

export type AgentClientBuiltin = (typeof AGENT_CLIENT_BUILTINS)[number];

/** Whether the agent's Workspace tab shows its runs and results. */
export function hasAgentClient(builtin: unknown): boolean {
  return typeof builtin === "string" && (AGENT_CLIENT_BUILTINS as readonly string[]).includes(builtin);
}

/**
 * The specialist a task was launched as, or "" for an ordinary task. New task
 * records `launch_kind: "task_run"` for ordinary runs too, so the field being
 * set does not by itself mean a specialist.
 */
export function specialistKind(task: Task): string {
  const kind = task.metadata?.launch_kind;
  return typeof kind === "string" && kind !== "task_run" ? kind : "";
}
