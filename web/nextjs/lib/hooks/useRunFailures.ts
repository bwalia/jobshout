"use client";

import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import { getBlogRuns } from "@/lib/api/blog";
import { listCourseRuns } from "@/lib/api/courses";
import { useAllTasks } from "@/lib/hooks/useTasks";
import type { BlogRun } from "@/lib/types/blog";
import type { CourseRun } from "@/lib/types/course";
import type { Task } from "@/lib/types/project";

/** How far back a failure still counts as needing attention. */
const WINDOW_MS = 14 * 24 * 60 * 60 * 1000;

/** The writers whose runs live in the blog tables (one list call each). */
const BLOG_WRITERS = ["article_writer", "jobshout_com_writer"] as const;

export type RunKind = "task" | "article" | "course";

/** One failed run that is still the latest word on its task or run. */
export interface RunFailure {
  key: string;
  kind: RunKind;
  runId: string;
  agentId: string | null;
  /** The board task the run belongs to, when there is one. */
  taskId: string | null;
  title: string;
  /** Short, human error ("Gemini quota exceeded (HTTP 429)"). */
  error: string;
  at: string;
}

/** An agent's most recent run, of any kind. */
export interface LastRun {
  status: "completed" | "failed" | "running" | "other";
  at: string;
  error?: string;
}

export interface AgentRunStats {
  failed: number;
  last?: LastRun;
}

/**
 * Turns a raw provider error into something a person can act on. The full
 * text stays available on the run itself.
 */
export function shortError(raw: string | null | undefined): string {
  const s = (raw ?? "").trim();
  if (!s) return "Run failed";
  const status = s.match(/\b(?:HTTP|status)\s*(\d{3})\b/i)?.[1];
  const provider = /gemini/i.test(s)
    ? "Gemini"
    : /ollama/i.test(s)
      ? "Ollama"
      : /openai/i.test(s)
        ? "OpenAI"
        : /claude|anthropic/i.test(s)
          ? "Claude"
          : "";
  if (status === "429" || /quota|rate.?limit|resource.?exhausted/i.test(s)) {
    return `${provider || "Model"} quota exceeded${status ? ` (HTTP ${status})` : ""}`;
  }
  if (status === "401" || status === "403") {
    return `${provider || "Provider"} rejected the credentials (HTTP ${status})`;
  }
  if (/connection refused|dial tcp|no such host|ECONNREFUSED|unreachable/i.test(s)) {
    return `${provider || "The model provider"} is not reachable`;
  }
  if (/timed out|deadline|timeout/i.test(s)) return "Timed out";
  if (/not found|no such model|model .* not (?:found|installed)/i.test(s)) {
    return `${provider ? `${provider}: ` : ""}model not available`;
  }
  const first = s.split(/\n/)[0];
  return first.length > 90 ? `${first.slice(0, 87)}…` : first;
}

function recent(at: string | null | undefined, now: number): boolean {
  const t = at ? Date.parse(at) : NaN;
  return !Number.isNaN(t) && now - t <= WINDOW_MS;
}

function runIdOf(task: Task): string | null {
  const v = task.metadata?.run_id;
  return typeof v === "string" ? v : null;
}

/**
 * Failed runs across the three run kinds the platform has: generic task runs,
 * article runs (Article Writer, Content Writer) and course runs. Computed in
 * the browser from the lists each tab already uses, so it needs no new API.
 */
export function useRunFailures() {
  const tasksQuery = useAllTasks();
  const results = useQueries({
    queries: [
      ...BLOG_WRITERS.map((writer) => ({
        queryKey: ["blogs", "list", { per_page: 50, writer }],
        queryFn: () => getBlogRuns({ per_page: 50, writer }),
        staleTime: 60_000,
      })),
      {
        queryKey: ["courses", "list", { per_page: 50 }],
        queryFn: () => listCourseRuns({ per_page: 50 }),
        staleTime: 60_000,
      },
    ],
  });

  const blogRuns = results
    .slice(0, BLOG_WRITERS.length)
    .flatMap((r) => ((r.data as { data?: BlogRun[] } | undefined)?.data ?? []));
  const courseRuns = ((results[BLOG_WRITERS.length]?.data as { data?: CourseRun[] } | undefined)?.data ?? []);
  const tasks = useMemo(() => tasksQuery.data?.data ?? [], [tasksQuery.data]);
  const isLoading = tasksQuery.isLoading || results.some((r) => r.isLoading);
  const dataKey = results.map((r) => r.dataUpdatedAt).join(",");

  return useMemo(() => {
    const now = Date.now();
    const taskByRun = new Map<string, Task>();
    for (const t of tasks) {
      const rid = runIdOf(t);
      if (rid) taskByRun.set(rid, t);
    }

    const failures: RunFailure[] = [];
    const stats = new Map<string, AgentRunStats>();
    const touch = (agentId: string | null, last: LastRun, failed: boolean) => {
      if (!agentId) return;
      const s = stats.get(agentId) ?? { failed: 0 };
      if (failed) s.failed += 1;
      if (!s.last || s.last.at < last.at) s.last = last;
      stats.set(agentId, s);
    };

    for (const r of blogRuns) {
      const task = taskByRun.get(r.id) ?? null;
      const status = r.status === "completed" ? "completed" : r.status === "failed" ? "failed" : r.status === "running" ? "running" : "other";
      const failed = r.status === "failed" && recent(r.created_at, now);
      touch(r.agent_id, { status, at: r.created_at, error: r.error_message ?? undefined }, failed);
      if (failed) {
        failures.push({
          key: `article:${r.id}`,
          kind: "article",
          runId: r.id,
          agentId: r.agent_id,
          taskId: task?.id ?? null,
          title: task?.title ?? r.briefs?.[0]?.topic ?? r.topics?.[0] ?? "Article run",
          error: shortError(r.error_message),
          at: r.created_at,
        });
      }
    }

    for (const r of courseRuns) {
      const status = r.status === "completed" ? "completed" : r.status === "failed" ? "failed" : r.status === "running" ? "running" : "other";
      const at = r.created_at;
      const failed = r.status === "failed" && recent(at, now);
      touch(r.agent_id, { status, at, error: r.error_message ?? undefined }, failed);
      if (failed) {
        failures.push({
          key: `course:${r.id}`,
          kind: "course",
          runId: r.id,
          agentId: r.agent_id,
          taskId: r.task_id,
          title: r.brief?.topic || "Course run",
          error: shortError(r.error_message),
          at,
        });
      }
    }

    for (const t of tasks) {
      if (!t.last_run_status || !t.last_run_at) continue;
      // A specialist's task points at its own run, already counted above.
      if (runIdOf(t)) continue;
      const failed = t.last_run_status === "failed" && recent(t.last_run_at, now);
      touch(
        t.assigned_agent_id ?? null,
        { status: t.last_run_status === "completed" ? "completed" : failed ? "failed" : t.last_run_status === "running" ? "running" : "other", at: t.last_run_at },
        failed
      );
      if (failed && t.last_run_id) {
        failures.push({
          key: `task:${t.last_run_id}`,
          kind: "task",
          runId: t.last_run_id,
          agentId: t.assigned_agent_id ?? null,
          taskId: t.id,
          title: t.title,
          error: "Last run failed",
          at: t.last_run_at,
        });
      }
    }

    failures.sort((a, b) => b.at.localeCompare(a.at));
    const failedTaskIds = new Set(failures.map((f) => f.taskId).filter(Boolean) as string[]);
    return {
      failures,
      byAgent: stats,
      failedTaskIds,
      failedAgentCount: Array.from(stats.values()).filter((s) => s.failed > 0).length,
      isLoading,
    };
    // dataKey stands in for the query results' identities.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tasks, dataKey, isLoading]);
}
