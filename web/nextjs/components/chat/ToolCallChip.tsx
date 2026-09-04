"use client";

import { Check, X, Loader2, Clock, Ban } from "lucide-react";
import { cn } from "@/lib/utils/cn";
import type { ActionRecord } from "@/lib/types/chat";

export function ToolCallChip({
  action,
  label,
  running,
}: {
  action?: ActionRecord;
  label?: string;
  running?: boolean;
}) {
  const status = running ? "running" : (action?.status ?? "ok");
  return (
    <details className="group min-w-0 rounded-md border border-border/70 bg-secondary/30 text-xs">
      <summary className="flex cursor-pointer list-none items-center gap-2 px-2.5 py-1.5 text-muted-foreground [&::-webkit-details-marker]:hidden">
        {statusIcon(status)}
        <span className="min-w-0 truncate font-medium text-foreground">
          {label || humanise(action?.tool)}
        </span>
        {typeof action?.duration_ms === "number" ? (
          <span className="ml-auto shrink-0 font-mono text-[10px] opacity-60">
            {action.duration_ms}ms
          </span>
        ) : null}
      </summary>
      {action?.args && Object.keys(action.args).length > 0 ? (
        <pre className="max-h-40 overflow-auto border-t border-border/60 px-2.5 py-2 font-mono text-[10px] text-muted-foreground">
          {JSON.stringify(stripIds(action.args), null, 2)}
        </pre>
      ) : null}
      {action?.error ? (
        <p className={cn("border-t border-border/60 px-2.5 py-1.5 text-destructive")}>
          {action.error}
        </p>
      ) : null}
    </details>
  );
}

function statusIcon(status: string) {
  if (status === "running") {
    return <Loader2 className="h-3 w-3 shrink-0 animate-spin text-primary" />;
  }
  if (status === "ok") {
    return <Check className="h-3 w-3 shrink-0 text-emerald-500" />;
  }
  if (status === "pending_confirmation") {
    return <Clock className="h-3 w-3 shrink-0 text-amber-500" />;
  }
  if (status === "denied") {
    return <Ban className="h-3 w-3 shrink-0 text-muted-foreground" />;
  }
  return <X className="h-3 w-3 shrink-0 text-destructive" />;
}

function humanise(tool?: string): string {
  if (!tool) return "Working…";
  return tool.replace(/_/g, " ");
}

function stripIds(args: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(args)) {
    if (k === "id" || /(^|_)id$/i.test(k)) continue;
    out[k] = v;
  }
  return out;
}
