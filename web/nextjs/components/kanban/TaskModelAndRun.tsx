"use client";

import Link from "next/link";
import { AlertTriangle, CheckCircle2, Cpu, Loader2, RotateCcw } from "lucide-react";
import { ModelPicker, providerName, type ModelSelection } from "@/components/agent/ModelPicker";
import { agentHref } from "@/lib/agents/links";
import { hasAgentClient, specialistKind } from "@/lib/agents/specialist";
import { useBlogConfig, useRetryBlogRun } from "@/lib/hooks/useBlog";
import { useRunFailures } from "@/lib/hooks/useRunFailures";
import { useCreateTaskRun, useTaskRun } from "@/lib/hooks/useTaskRuns";
import { runsHubHref, taskResultHref } from "@/lib/run-links";
import type { Agent } from "@/lib/types/agent";
import type { Task, TaskModelOverride } from "@/lib/types/project";

const PICKER_CLASS =
  "flex h-10 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50";

/** The override saved on a task, if any. */
export function savedModelOverride(task: Task): TaskModelOverride {
  const raw = task.metadata?.model_override as Partial<TaskModelOverride> | undefined;
  return { provider: raw?.provider ?? "", model: raw?.model ?? "" };
}

/**
 * A specialist (Article Writer, Course Generator…) runs through its own launch
 * flow, which reads the agent's models, not a task override.
 */
export function isSpecialistTask(task: Task, agent?: Agent): boolean {
  return Boolean(agent?.metadata?.builtin) || specialistKind(task) !== "";
}

function describe(sel: { provider?: string | null; model?: string | null }, inherited?: string): string {
  if (sel.provider === "auto") return "Automatic (chosen per request)";
  if (!sel.provider && !sel.model) return inherited ? `Platform default (${inherited})` : "Platform default";
  return [providerName(sel.provider ?? ""), sel.model || "provider default"].filter(Boolean).join(" · ");
}

/**
 * Which model this task's runs will use, in words, and (for ordinary tasks) a
 * per-task override. The override is saved with the rest of the panel.
 */
export function TaskModelSection({
  task,
  agent,
  override,
  onOverride,
}: {
  task: Task;
  agent?: Agent;
  override: ModelSelection;
  onOverride: (v: ModelSelection) => void;
}) {
  const { data: blogConfig } = useBlogConfig();
  const specialist = isSpecialistTask(task, agent);
  const writer = agent?.metadata?.builtin === "article_writer";
  const agentModel = describe(
    { provider: agent?.model_provider, model: agent?.model_name },
    writer ? blogConfig?.effective_models?.prose : undefined
  );
  const overridden = !specialist && Boolean(override.provider || override.model);

  return (
    <div className="space-y-2 rounded-lg border border-border bg-muted/20 p-3">
      <div className="flex items-start gap-2">
        <Cpu className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
        <p className="text-sm">
          {!agent ? (
            <span className="text-muted-foreground">Assign an agent to choose a model.</span>
          ) : overridden ? (
            <>
              This task uses <span className="font-medium">{describe(override)}</span>
              <span className="text-muted-foreground"> instead of {agent.name}&apos;s model ({agentModel}).</span>
            </>
          ) : (
            <>
              Uses {agent.name}&apos;s {writer ? "writing model" : "model"}:{" "}
              <span className="font-medium">{agentModel}</span>
            </>
          )}
        </p>
      </div>
      {agent && (
        <Link href={agentHref(agent.id, "models")} className="inline-block text-xs font-medium text-primary hover:underline">
          Change {agent.name}&apos;s model →
        </Link>
      )}
      {agent && !specialist && (
        <div className="space-y-1">
          <label htmlFor="task-model-override" className="text-xs font-medium text-muted-foreground">
            Model for this task only
          </label>
          <ModelPicker
            id="task-model-override"
            value={override}
            onChange={onOverride}
            defaultLabel={`Use ${agent.name}'s model`}
            className={PICKER_CLASS}
          />
        </div>
      )}
      {agent && specialist && (
        <p className="text-xs text-muted-foreground">
          {agent.name} is a specialist: its runs use the agent&apos;s own model settings.
        </p>
      )}
    </div>
  );
}

function relative(at?: string | null): string {
  if (!at) return "";
  const mins = Math.round((Date.now() - Date.parse(at)) / 60000);
  if (Number.isNaN(mins)) return "";
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const h = Math.round(mins / 60);
  return h < 24 ? `${h}h ago` : `${Math.round(h / 24)}d ago`;
}

/**
 * The latest run, inline: its status, and when it failed, the error and Retry.
 * Covers ordinary task runs and the specialist runs a task launched.
 */
export function TaskLatestRun({ task, agent }: { task: Task; agent?: Agent }) {
  const { failures } = useRunFailures();
  const specialistFailure = failures.find((f) => f.taskId === task.id && f.kind !== "task");
  const genericFailed = task.last_run_status === "failed";
  const { data: run } = useTaskRun(genericFailed ? task.last_run_id ?? null : null);
  const createRun = useCreateTaskRun();
  const retryArticle = useRetryBlogRun();

  if (specialistFailure) {
    return (
      <FailedRun
        error={specialistFailure.error}
        at={specialistFailure.at}
        busy={retryArticle.isPending}
        onRetry={
          specialistFailure.kind === "article"
            ? () => retryArticle.mutate(specialistFailure.runId)
            : undefined
        }
        openHref={
          // An article run has its own page; other specialists show runs on
          // their Workspace tab only when they have a screen of their own.
          specialistFailure.kind === "article"
            ? `/articles/${specialistFailure.runId}`
            : agent && hasAgentClient(agent.metadata?.builtin)
              ? `${agentHref(agent.id, "workspace")}&run=${encodeURIComponent(specialistFailure.runId)}`
              : undefined
        }
      />
    );
  }
  if (genericFailed) {
    return (
      <FailedRun
        error={run?.error_message || "The last run failed."}
        at={task.last_run_at}
        busy={createRun.isPending}
        onRetry={task.assigned_agent_id ? () => createRun.mutate({ taskId: task.id, payload: {} }) : undefined}
        openHref={task.last_run_id ? taskResultHref(task.id, task.last_run_id) : undefined}
        runsHref={task.assigned_agent_id ? runsHubHref({ agentId: task.assigned_agent_id, failed: true }) : runsHubHref({ failed: true })}
      />
    );
  }
  if (!task.last_run_status) return null;
  const resultHref = task.last_run_id ? taskResultHref(task.id, task.last_run_id) : undefined;
  return (
    <div className="flex flex-wrap items-center gap-2 text-sm">
      <p className="flex items-center gap-1.5 text-muted-foreground">
        {task.last_run_status === "completed" ? (
          <CheckCircle2 className="h-4 w-4 text-status-done" />
        ) : (
          <Loader2 className="h-4 w-4 animate-spin" />
        )}
        Last run {task.last_run_status === "completed" ? "succeeded" : task.last_run_status}{" "}
        {relative(task.last_run_at)}
      </p>
      {resultHref && (
        <Link href={resultHref} className="text-xs font-semibold text-primary hover:underline">
          View result →
        </Link>
      )}
    </div>
  );
}

function FailedRun({
  error,
  at,
  busy,
  onRetry,
  openHref,
  runsHref,
}: {
  error: string;
  at?: string | null;
  busy: boolean;
  onRetry?: () => void;
  openHref?: string;
  runsHref?: string;
}) {
  return (
    <div role="alert" className="space-y-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3">
      <p className="flex items-start gap-1.5 text-sm font-medium text-destructive">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
        <span>
          Last run failed {relative(at)}: {error}
        </span>
      </p>
      <div className="flex flex-wrap gap-2">
        {onRetry && (
          <button
            type="button"
            onClick={onRetry}
            disabled={busy}
            className="inline-flex h-10 items-center gap-1.5 rounded-md bg-primary px-4 text-sm font-semibold text-primary-foreground hover:opacity-90 disabled:opacity-50"
          >
            {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <RotateCcw className="h-3.5 w-3.5" />}
            Retry
          </button>
        )}
        {openHref && (
          <Link href={openHref} className="inline-flex h-10 items-center rounded-md border border-border px-4 text-sm font-medium hover:bg-muted">
            View result
          </Link>
        )}
        {runsHref && (
          <Link href={runsHref} className="inline-flex h-10 items-center rounded-md border border-border px-4 text-sm font-medium hover:bg-muted">
            All failed runs
          </Link>
        )}
      </div>
    </div>
  );
}
