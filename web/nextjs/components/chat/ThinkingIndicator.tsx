"use client";

import { useEffect, useRef, useState } from "react";
import { Sparkles } from "lucide-react";
import { cn } from "@/lib/utils/cn";

/**
 * Replaces the bare "Working…" chip. A run can spend a long time inside one
 * tool, so the wait needs three facts: that we are alive, what we are doing,
 * and how long it has been. Elapsed time only appears after 2s so quick turns
 * do not flash a stopwatch.
 */
export function ThinkingIndicator({
  label,
  model,
  className,
}: {
  label?: string | null;
  /** Model serving this turn. A 30B model on a busy host can take minutes,
   *  and naming it is the difference between waiting and giving up. */
  model?: string | null;
  className?: string;
}) {
  const startedAt = useRef(Date.now());
  const [elapsed, setElapsed] = useState(0);

  useEffect(() => {
    const t = setInterval(() => {
      setElapsed(Math.floor((Date.now() - startedAt.current) / 1000));
    }, 1000);
    return () => clearInterval(t);
  }, []);

  return (
    <div className={cn("flex items-start gap-3", className)} role="status">
      <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full border border-border bg-card">
        <Sparkles className="h-3.5 w-3.5 animate-chat-pulse text-primary" />
      </span>
      <div className="flex min-w-0 flex-1 flex-col gap-2 pt-1">
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">
            {label?.trim() || "Thinking"}
          </span>
          <ChatDots />
          {model ? (
            <span className="truncate font-mono text-xs text-muted-foreground/70">
              {model}
            </span>
          ) : null}
          {elapsed >= 2 ? (
            <span className="tabular text-xs text-muted-foreground/70">{elapsed}s</span>
          ) : null}
        </div>
        {/* Shimmer bars read as "text is coming", which a spinner does not. */}
        <div className="flex flex-col gap-1.5" aria-hidden="true">
          <span className="h-2 w-[62%] rounded-full bg-muted animate-chat-shimmer" />
          <span className="h-2 w-[38%] rounded-full bg-muted animate-chat-shimmer [animation-delay:150ms]" />
        </div>
      </div>
    </div>
  );
}

function ChatDots() {
  return (
    <span className="flex items-center gap-0.5" aria-hidden="true">
      {[0, 1, 2].map((i) => (
        <span
          key={i}
          className="h-1 w-1 rounded-full bg-muted-foreground animate-chat-bounce"
          style={{ animationDelay: `${i * 140}ms` }}
        />
      ))}
    </span>
  );
}
