"use client";

import { Rocket, X } from "lucide-react";
import { TaskRunView } from "@/components/task-manager/TaskRunView";
import { formatCompletedAt } from "@/components/task-manager/TaskProgressChip";
import { useTaskHistory } from "@/lib/hooks/useTasks";
import { statusLabel } from "@/lib/task-labels";
import type { Agent } from "@/lib/types/agent";
import type { Task } from "@/lib/types/project";
import type { TaskHistoryEvent } from "@/lib/api/tasks";

const SPECIALIST_LABELS: Record<string, string> = {
  article_writer: "Article run",
  images: "Image generation",
  pentester: "Security test",
  pr_reviewer: "PR review",
  mail: "Mailbox sync",
  researcher: "Research brief",
};

function formatWhen(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString();
}

function eventSummary(e: TaskHistoryEvent): string {
  if (e.kind === "status") {
    const from = e.old_status ? statusLabel(e.old_status) : "—";
    const to = e.new_status ? statusLabel(e.new_status) : "—";
    return `Status ${from} → ${to}`;
  }
  if (e.kind === "specialist") {
    const kind = e.launch_kind ?? "";
    return SPECIALIST_LABELS[kind] ?? "Specialist launch";
  }
  const runStatus = e.status ? statusLabel(e.status) : "run";
  return `Agent run · ${runStatus}`;
}

export function TaskHistoryPanel({
  task,
  agents,
  focusRunId,
  onClose,
  onReRun,
}: {
  task: Task;
  agents: Agent[];
  focusRunId?: string | null;
  onClose: () => void;
  onReRun?: () => void;
}) {
  const { data: history, isLoading } = useTaskHistory(task.id);
  const completed =
    formatCompletedAt(history?.completed_at ?? task.completed_at) ??
    "Not completed";

  return (
    <div
      className="fixed inset-0 z-[60] flex items-start justify-end bg-black/40"
      onClick={(e) => {
        e.stopPropagation();
        onClose();
      }}
    >
      <div
        className="flex h-full w-full max-w-lg flex-col overflow-y-auto border-l border-border bg-card shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-3 border-b border-border px-6 py-4">
          <div className="min-w-0">
            <h2 className="text-base font-semibold">History</h2>
            <p className="mt-0.5 truncate text-sm text-muted-foreground">
              {task.title}
            </p>
            <p className="mt-1 text-xs text-muted-foreground">
              {statusLabel(task.status)}
              {" · "}
              {completed === "Not completed"
                ? "Not completed"
                : `Completed ${completed}`}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {task.status === "done" && onReRun ? (
              <button
                type="button"
                onClick={onReRun}
                className="inline-flex h-8 items-center gap-1.5 rounded-md border border-border px-2.5 text-xs hover:bg-secondary"
              >
                <Rocket className="h-3.5 w-3.5" /> Re-run
              </button>
            ) : null}
            <button
              type="button"
              onClick={onClose}
              className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
              aria-label="Close history"
            >
              <X className="h-5 w-5" />
            </button>
          </div>
        </div>

        <div className="space-y-6 px-6 py-5">
          <section>
            <h3 className="mb-2 text-sm font-medium">Timeline</h3>
            {isLoading ? (
              <p className="text-sm text-muted-foreground">Loading history…</p>
            ) : !history || history.events.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                Nothing recorded yet. Run this task to start a history.
              </p>
            ) : (
              <ol className="space-y-2">
                {history.events.map((e) => (
                  <li
                    key={`${e.kind}-${e.id}`}
                    className="rounded-md border border-border bg-muted/30 px-3 py-2"
                  >
                    <p className="text-sm">{eventSummary(e)}</p>
                    <p className="mt-0.5 font-mono text-[11px] text-muted-foreground">
                      {formatWhen(e.changed_at ?? e.completed_at ?? e.created_at)}
                    </p>
                  </li>
                ))}
              </ol>
            )}
          </section>

          <section>
            <h3 className="mb-2 text-sm font-medium">Runs</h3>
            <TaskRunView
              task={task}
              agents={agents}
              focusRunId={focusRunId}
            />
          </section>
        </div>
      </div>
    </div>
  );
}
