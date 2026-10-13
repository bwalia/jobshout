/** Deep link that opens a task with its History focused on a run. */
export function taskResultHref(taskId: string, runId?: string | null): string {
  const params = new URLSearchParams({ task: taskId });
  if (runId) params.set("run", runId);
  return `/panel/tasks?${params.toString()}`;
}

/** Org Runs hub, optionally scoped to an agent or failures only. */
export function runsHubHref(opts?: {
  agentId?: string | null;
  status?: string | null;
  failed?: boolean;
}): string {
  const params = new URLSearchParams();
  if (opts?.agentId) params.set("agent", opts.agentId);
  if (opts?.status) params.set("status", opts.status);
  if (opts?.failed) params.set("failed", "1");
  const q = params.toString();
  return q ? `/panel/runs?${q}` : "/panel/runs";
}
