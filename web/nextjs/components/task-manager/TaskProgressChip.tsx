"use client";

import { AlertTriangle, CheckCircle2, Clock, Loader2 } from "lucide-react";
import type { Task } from "@/lib/types/project";
import { cn } from "@/lib/utils/cn";

function formatWhen(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

export function formatCompletedAt(iso: string | null | undefined): string | null {
  if (!iso) return null;
  const formatted = formatWhen(iso);
  return formatted || null;
}

export function TaskCountLabel({
  loaded,
  total,
}: {
  loaded: number;
  total?: number;
}) {
  if (total != null && total > loaded) {
    return (
      <>
        Showing {loaded} of {total} tasks
      </>
    );
  }
  return (
    <>
      {loaded} task{loaded === 1 ? "" : "s"}
    </>
  );
}

export function TaskProgressChip({ task }: { task: Task }) {
  const status = task.last_run_status;
  if (!status) return null;

  const when = formatWhen(task.last_run_at);
  const map = {
    queued: {
      label: "Queued",
      cls: "border-border bg-muted/60 text-muted-foreground",
      icon: <Clock className="h-3 w-3" />,
    },
    running: {
      label: "Running",
      cls: "border-status-progress/40 bg-status-progress/10 text-foreground",
      icon: <Loader2 className="h-3 w-3 animate-spin" />,
    },
    completed: {
      label: when ? `Completed ${when}` : "Completed",
      cls: "border-emerald-500/30 bg-emerald-500/10 text-emerald-800 dark:text-emerald-400",
      icon: <CheckCircle2 className="h-3 w-3" />,
    },
    failed: {
      label: "Failed",
      cls: "border-destructive/40 bg-destructive/10 text-destructive",
      icon: <AlertTriangle className="h-3 w-3" />,
    },
  } as const;
  const m = map[status];
  if (!m) return null;

  return (
    <span
      className={cn(
        "inline-flex max-w-full items-center gap-1 truncate rounded-full border px-2 py-0.5 text-[11px] font-medium",
        m.cls
      )}
    >
      {m.icon}
      <span className="truncate">{m.label}</span>
    </span>
  );
}
