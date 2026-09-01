"use client";

import type { ClarifyRequest } from "@/lib/types/chat";

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
  return (
    <div className="mt-3">
      <p className="text-sm text-foreground">{clarify.question}</p>
      {clarify.options && clarify.options.length > 0 ? (
        live ? (
          <div className="mt-2 flex flex-wrap gap-1.5">
            {clarify.options.map((opt) => (
              <button
                key={opt.value}
                type="button"
                onClick={() => onPick(opt.value, opt.label)}
                className="rounded-full border border-border bg-secondary/50 px-3 py-1 text-xs font-medium hover:border-primary/50 hover:bg-primary/10"
              >
                {opt.label}
              </button>
            ))}
          </div>
        ) : (
          <p className="mt-2 text-xs font-medium text-muted-foreground">
            {answeredAs ? `Chose “${answeredAs}”` : "Answered"}
          </p>
        )
      ) : null}
    </div>
  );
}
