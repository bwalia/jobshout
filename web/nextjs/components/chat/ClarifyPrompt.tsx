"use client";

import { Check, HelpCircle } from "lucide-react";
import type { ClarifyRequest } from "@/lib/types/chat";

/**
 * The agent narrowing something down before it acts. It sits inside an agent
 * turn, so it wears the same card chrome as the run cards — a question that is
 * waiting on an answer should look like one, not like another paragraph.
 */
export function ClarifyPrompt({
  clarify,
  onPick,
  live,
  answeredAs,
}: {
  clarify: ClarifyRequest;
  /**
   * value is the machine value the agent resolves on (often an ID); label is
   * what the user clicked and what the transcript should show.
   */
  onPick: (value: string, label: string) => void;
  live?: boolean;
  answeredAs?: string;
}) {
  const options = clarify.options ?? [];

  return (
    <div className="mt-2 overflow-hidden rounded-lg border border-border bg-card">
      <div className="flex min-w-0 items-start gap-2 px-3 py-2.5">
        <HelpCircle className="mt-0.5 h-4 w-4 shrink-0 text-signal-info" />
        <p className="min-w-0 break-words text-[15px] leading-6 text-foreground">
          {clarify.question}
        </p>
      </div>

      {options.length === 0 ? null : live ? (
        <div className="flex flex-wrap gap-1.5 border-t border-border px-3 py-2">
          {options.map((opt) => (
            <button
              key={opt.value}
              type="button"
              onClick={() => onPick(opt.value, opt.label)}
              className="inline-flex items-center rounded-full border border-border bg-secondary/50 px-3 py-1.5 text-xs font-medium text-foreground transition-colors hover:border-primary/50 hover:bg-primary/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-sm:min-h-[44px] max-sm:px-4"
            >
              {opt.label}
            </button>
          ))}
        </div>
      ) : (
        <p className="flex items-center gap-1.5 border-t border-border px-3 py-2 text-xs font-medium text-muted-foreground">
          <Check className="h-3.5 w-3.5 shrink-0 text-signal-live" />
          {answeredAs ? `Chose “${answeredAs}”` : "Answered"}
        </p>
      )}
    </div>
  );
}
