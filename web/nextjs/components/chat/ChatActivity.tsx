"use client";

import { CheckCircle2, Loader2, Wrench, XCircle } from "lucide-react";

export interface ToolActivity {
  name: string;
  running: boolean;
  ok?: boolean;
  durationMs?: number;
}

const STAGE_LABEL: Record<string, string> = {
  planning: "Planning…",
  agent_selected: "Agent selected",
  executing: "Executing…",
  completed: "Completed",
  failed: "Failed",
};

/**
 * The live execution strip shown while a turn streams. Renders the current
 * high-level stage plus each tool as it starts and finishes — the safe,
 * progressive view of what the agent is doing (never hidden reasoning).
 */
export function ChatActivity({
  stage,
  tools,
}: {
  stage: string | null;
  tools: ToolActivity[];
}) {
  return (
    <div className="mx-auto w-full max-w-3xl px-4 pb-3">
      <div className="rounded-lg border border-border bg-card px-3 py-2">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin text-primary" />
          {stage ? (STAGE_LABEL[stage] ?? stage) : "Working…"}
        </div>
        {tools.length > 0 && (
          <ul className="mt-2 space-y-1">
            {tools.map((t, i) => (
              <li key={i} className="flex items-center gap-2 text-xs">
                <Wrench className="h-3.5 w-3.5 text-muted-foreground" />
                <span className="font-mono">{t.name}</span>
                {t.running ? (
                  <Loader2 className="h-3 w-3 animate-spin text-muted-foreground" />
                ) : t.ok === false ? (
                  <XCircle className="h-3 w-3 text-signal-error" />
                ) : (
                  <CheckCircle2 className="h-3 w-3 text-signal-live" />
                )}
                {!t.running && typeof t.durationMs === "number" && (
                  <span className="ml-auto font-mono text-muted-foreground">
                    {t.durationMs}ms
                  </span>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
