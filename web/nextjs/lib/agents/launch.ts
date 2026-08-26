import { apiClient } from "@/lib/api/client";
import { createTaskRun } from "@/lib/api/task-runs";
import { generateBlog } from "@/lib/api/blog";
import type { AgentInputSchema } from "@/lib/agents/input-schemas";
import { runInputsFromValues } from "@/lib/agents/input-schemas";
import type { Agent } from "@/lib/types/agent";
import type { Task } from "@/lib/types/project";
import type { TaskRun } from "@/lib/types/task-run";
import type { ScanMode } from "@/types/pentest";
import type { PentestRun } from "@/types/pentest";
import type { ReviewRun } from "@/types/review";
import type { BlogRun } from "@/lib/types/blog";

export type LaunchResult =
  | { kind: "task_run"; run: TaskRun; task: Task }
  | { kind: "pentester"; run: PentestRun; task: Task }
  | { kind: "pr_reviewer"; run: ReviewRun; task: Task }
  | { kind: "article_writer"; run: BlogRun; task: Task }
  | { kind: "researcher"; brief: unknown; task: Task };

/**
 * After the board task exists, kick off the right executor for the chosen agent.
 */
export async function launchAgentForTask(opts: {
  agent: Agent;
  task: Task;
  schema: AgentInputSchema;
  values: Record<string, string>;
}): Promise<LaunchResult> {
  const { agent, task, schema, values } = opts;

  switch (schema.kind) {
    case "pentester": {
      const payload: Record<string, unknown> = {
        agent_id: agent.id,
        task_id: task.id,
        target: values.target.trim(),
        scan_mode: (values.scan_mode || "quick") as ScanMode,
      };
      if (values.max_budget.trim()) {
        payload.max_budget = parseInt(values.max_budget, 10);
      }
      if (values.instruction.trim()) {
        payload.instruction = values.instruction.trim();
      }
      const { data: run } = await apiClient.post<PentestRun>(
        "/pentest-runs",
        payload
      );
      return { kind: "pentester", run, task };
    }
    case "pr_reviewer": {
      const { data: run } = await apiClient.post<ReviewRun>("/review-runs", {
        repo: values.repo.trim(),
        pr_number: parseInt(values.pr_number, 10),
        dry_run: values.dry_run === "true",
        agent_id: agent.id,
      });
      return { kind: "pr_reviewer", run, task };
    }
    case "article_writer": {
      const run = await generateBlog({
        briefs: [
          {
            topic: values.topic.trim(),
            context: values.context.trim() || undefined,
          },
        ],
        model: values.model?.trim() || undefined,
      });
      return { kind: "article_writer", run, task };
    }
    case "researcher": {
      // Research runs synchronously and can take longer than the default 30s.
      const { data: brief } = await apiClient.post(
        "/research",
        {
          topic: values.topic.trim(),
          context: values.context.trim() || undefined,
        },
        { timeout: 180_000 }
      );
      return { kind: "researcher", brief, task };
    }
    default: {
      const inputs = runInputsFromValues(schema, values);
      const payload: Parameters<typeof createTaskRun>[1] = {
        agent_id: agent.id,
      };
      if (Object.keys(inputs).length) payload.inputs = inputs;
      // Specialists store their prompt in title/description already.
      if (schema.kind === "task_run" && values.description?.trim()) {
        // title+description used by default; no override needed
      }
      const run = await createTaskRun(task.id, payload);
      return { kind: "task_run", run, task };
    }
  }
}
