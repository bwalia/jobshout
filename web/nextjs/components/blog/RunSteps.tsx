"use client";

import { SignalDot, type SignalStatus } from "@/components/ui/signal-dot";
import { cn } from "@/lib/utils/cn";
import type { BlogStep, StepStatus } from "@/lib/types/blog";

/** Step status → the SignalDot vocabulary used everywhere else in the app. */
const DOT: Record<StepStatus, SignalStatus> = {
  pending: "queued",
  running: "live",
  done: "done",
  failed: "error",
  // A skipped step did not fail and did not do anything — it reads as an
  // inactive stop on the trace, not a problem.
  skipped: "queued",
};

/** Elapsed time for a finished step, so a slow phase is visible at a glance. */
function duration(step: BlogStep): string | null {
  if (!step.started_at || !step.completed_at) return null;
  const ms =
    new Date(step.completed_at).getTime() - new Date(step.started_at).getTime();
  if (Number.isNaN(ms) || ms < 0) return null;
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.round(ms / 60_000)}m`;
}

/**
 * Vertical trace of what the Article Writer did, and where it is now.
 *
 * Steps are pre-seeded as pending by the server, so the whole pipeline is
 * visible up front and lights up as it runs rather than appearing from nothing.
 */
export function RunSteps({ steps }: { steps: BlogStep[] }) {
  if (steps.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">No steps recorded yet.</p>
    );
  }

  return (
    <ol className="space-y-0">
      {steps.map((step, i) => {
        const isLast = i === steps.length - 1;
        return (
          <li key={`${step.key}-${i}`} className="flex gap-3">
            {/* Rail: dot plus the connector down to the next step. */}
            <div className="flex flex-col items-center">
              <span className="flex h-5 items-center">
                <SignalDot status={DOT[step.status] ?? "queued"} size="md" />
              </span>
              {!isLast && (
                <span
                  aria-hidden
                  className={cn(
                    "w-px flex-1",
                    step.status === "done" ? "bg-border" : "bg-border/40"
                  )}
                />
              )}
            </div>

            <div className={cn("min-w-0 flex-1", !isLast && "pb-4")}>
              <div className="flex items-baseline justify-between gap-3">
                <div className="min-w-0">
                  <p
                    className={cn(
                      "text-sm",
                      step.status === "pending" || step.status === "skipped"
                        ? "text-muted-foreground"
                        : "font-medium text-foreground"
                    )}
                  >
                    {step.label}
                  </p>
                  {/* Which agent is doing this. Omitted rather than guessed
                      for runs that predate the field. */}
                  {step.agent && (
                    <p className="mt-0.5 text-2xs text-muted-foreground">
                      {step.agent}
                    </p>
                  )}
                </div>
                {step.status === "skipped" ? (
                  <span className="shrink-0 text-2xs text-muted-foreground">
                    not needed
                  </span>
                ) : (
                  duration(step) && (
                    <span className="shrink-0 font-mono text-2xs text-muted-foreground">
                      {duration(step)}
                    </span>
                  )
                )}
              </div>
              {step.error && (
                <p className="mt-1 rounded border border-destructive/40 bg-destructive/5 px-2 py-1 text-2xs text-destructive">
                  {step.error}
                </p>
              )}
            </div>
          </li>
        );
      })}
    </ol>
  );
}
