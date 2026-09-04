"use client";

import { useState } from "react";
import Link from "next/link";
import {
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  Circle,
  ExternalLink,
  Loader2,
  Wrench,
  Zap,
} from "lucide-react";

import { useExecution } from "@/lib/hooks/useChatDetails";

function StatusPill({ status }: { status: string }) {
  const map: Record<string, { cls: string; icon: React.ReactNode }> = {
    completed: {
      cls: "text-signal-live",
      icon: <CheckCircle2 className="h-3.5 w-3.5" />,
    },
    running: {
      cls: "text-signal-live",
      icon: <Loader2 className="h-3.5 w-3.5 animate-spin" />,
    },
    pending: {
      cls: "text-muted-foreground",
      icon: <Loader2 className="h-3.5 w-3.5 animate-spin" />,
    },
    failed: {
      cls: "text-signal-error",
      icon: <AlertTriangle className="h-3.5 w-3.5" />,
    },
  };
  const m = map[status] ?? {
    cls: "text-muted-foreground",
    icon: <Circle className="h-3.5 w-3.5" />,
  };
  return (
    <span className={"inline-flex items-center gap-1 text-xs font-medium " + m.cls}>
      {m.icon}
      {status}
    </span>
  );
}

/** Compact agent-run card for chat. Links into Task Manager for full detail. */
export function ExecutionCard({
  executionId,
  agentName,
  agentId,
}: {
  executionId: string;
  agentName?: string;
  agentId?: string;
}) {
  const [open, setOpen] = useState(false);
  const { data: exec, isLoading, isError, refetch } = useExecution(executionId);
  const detailHref = agentId
    ? `/panel/task-manager?agent=${agentId}&run=${executionId}`
    : `/panel/task-manager`;

  if (isError) {
    return (
      <div className="mt-2 flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-xs text-muted-foreground">
        Couldn&apos;t load this run.
        <button
          type="button"
          onClick={() => void refetch()}
          className="rounded underline transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          Retry
        </button>
      </div>
    );
  }

  if (isLoading || !exec) {
    return (
      <div className="mt-2 flex items-center gap-2 rounded-lg border border-border bg-card px-3 py-2 text-xs text-muted-foreground">
        <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading agent activity…
      </div>
    );
  }

  const tools = exec.tool_calls ?? [];
  const duration =
    exec.latency_ms >= 1000
      ? `${(exec.latency_ms / 1000).toFixed(1)}s`
      : `${exec.latency_ms}ms`;

  return (
    <div className="mt-2 overflow-hidden rounded-lg border border-border bg-card">
      <div className="flex min-w-0 items-center gap-2 px-3 py-2">
        <Zap className="h-4 w-4 shrink-0 text-primary" />
        <span className="min-w-0 truncate text-sm font-medium">
          {agentName ?? "Agent run"}
        </span>
        <StatusPill status={exec.status} />
        <span className="ml-auto flex shrink-0 items-center gap-3 font-mono text-[11px] text-muted-foreground">
          <span>{exec.total_tokens} tok</span>
          <span>${exec.cost_usd.toFixed(4)}</span>
          <span>{duration}</span>
        </span>
        <Link
          href={detailHref}
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-sm:h-11 max-sm:w-11"
          aria-label="Open this run in Task Manager"
          title="Open in Task Manager"
        >
          <ExternalLink className="h-3.5 w-3.5" />
        </Link>
      </div>

      {(tools.length > 0 || exec.model_name) && (
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          className="flex w-full items-center gap-2 border-t border-border px-3 py-2 text-left text-xs text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring max-sm:min-h-[44px]"
        >
          <Wrench className="h-3.5 w-3.5" />
          {tools.length} tool call{tools.length === 1 ? "" : "s"}
          {exec.iterations ? ` · ${exec.iterations} steps` : ""}
          {exec.model_name ? ` · ${exec.model_name}` : ""}
          <ChevronDown
            className={
              "ml-auto h-4 w-4 transition-transform " + (open ? "rotate-180" : "")
            }
          />
        </button>
      )}

      {open && (
        <div className="space-y-1 border-t border-border px-3 py-2">
          {tools.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              No tools used — the agent answered directly.
            </p>
          ) : (
            tools.map((t, i) => (
              <div key={t.id} className="flex items-center gap-2 text-xs">
                <span className="font-mono text-muted-foreground">{i + 1}</span>
                <span className="font-mono font-medium">{t.tool_name}</span>
                {t.error_message ? (
                  <AlertTriangle className="h-3 w-3 text-signal-error" />
                ) : (
                  <CheckCircle2 className="h-3 w-3 text-signal-live" />
                )}
                <span className="ml-auto font-mono text-muted-foreground">
                  {t.duration_ms}ms
                </span>
              </div>
            ))
          )}
        </div>
      )}
    </div>
  );
}
