"use client";

import { Check, Lock } from "lucide-react";

export type CareerStepId = "profile" | "find" | "jobs" | "prepare";

export type CareerStepState = {
  /** Finished — the user can still go back to it. */
  done: boolean;
  /** Not reachable yet. `blockedReason` says what to do instead. */
  locked: boolean;
  blockedReason?: string;
  /** Short status under the title: "12 jobs found", "CV uploaded". */
  detail?: string;
};

const STEPS: { id: CareerStepId; title: string; blurb: string }[] = [
  { id: "profile", title: "Your profile", blurb: "Upload your CV and say what you want" },
  { id: "find", title: "Find jobs", blurb: "Scan company job boards" },
  { id: "jobs", title: "Score & prepare", blurb: "Rate the matches, write the materials" },
  { id: "prepare", title: "One job", blurb: "Tailored CV, cover letter, tracker" },
];

/**
 * The four steps of Career Agent, always visible, always showing where you are.
 *
 * The old version was a row of small buttons that looked identical whether a
 * step was finished, current, or impossible — so "I uploaded my CV, now what?"
 * had no answer on screen. Each step now carries its own state and, when it is
 * not reachable yet, says what to do first instead of silently doing nothing.
 */
export function CareerStepper({
  current,
  states,
  onGo,
}: {
  current: CareerStepId;
  states: Record<CareerStepId, CareerStepState>;
  onGo: (id: CareerStepId) => void;
}) {
  return (
    <nav aria-label="Career Agent steps">
      <ol className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {STEPS.map((step, i) => {
          const state = states[step.id];
          const isCurrent = current === step.id;
          const disabled = state.locked && !isCurrent;

          return (
            <li key={step.id}>
              <button
                type="button"
                onClick={() => !disabled && onGo(step.id)}
                disabled={disabled}
                aria-current={isCurrent ? "step" : undefined}
                title={disabled ? state.blockedReason : undefined}
                className={[
                  "group flex w-full items-start gap-3 rounded-lg border p-4 text-left transition-colors duration-200",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
                  isCurrent
                    ? "border-primary bg-primary/5 ring-1 ring-primary"
                    : disabled
                      ? "cursor-not-allowed border-border bg-muted/30 opacity-70"
                      : "cursor-pointer border-border hover:border-primary/50 hover:bg-muted/40",
                ].join(" ")}
              >
                <span
                  aria-hidden
                  className={[
                    "mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-semibold",
                    state.done
                      ? "bg-signal/15 text-signal"
                      : isCurrent
                        ? "bg-primary text-primary-foreground"
                        : "bg-muted text-muted-foreground",
                  ].join(" ")}
                >
                  {state.done ? (
                    <Check className="h-4 w-4" strokeWidth={2.5} />
                  ) : disabled ? (
                    <Lock className="h-4 w-4" strokeWidth={2} />
                  ) : (
                    i + 1
                  )}
                </span>

                <span className="min-w-0 flex-1">
                  <span className="block text-base font-semibold leading-tight text-foreground">
                    {step.title}
                  </span>
                  <span className="mt-1 block text-sm leading-snug text-muted-foreground">
                    {disabled ? state.blockedReason : (state.detail ?? step.blurb)}
                  </span>
                </span>
              </button>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}

/**
 * The single most useful thing on the page: one sentence saying what to do now,
 * and one button that does it. Rendered under the stepper on every step so the
 * answer to "what next?" is never more than a glance away.
 */
export function CareerNextStep({
  headline,
  detail,
  actionLabel,
  onAction,
  busy,
  tone = "primary",
}: {
  headline: string;
  detail?: string;
  actionLabel?: string;
  onAction?: () => void;
  busy?: boolean;
  tone?: "primary" | "done";
}) {
  return (
    <section
      aria-live="polite"
      className={[
        "flex flex-col gap-3 rounded-lg border p-4 sm:flex-row sm:items-center sm:justify-between",
        tone === "done"
          ? "border-signal/40 bg-signal/5"
          : "border-primary/30 bg-primary/5",
      ].join(" ")}
    >
      <div className="min-w-0">
        <p className="text-base font-semibold text-foreground">{headline}</p>
        {detail && <p className="mt-1 text-sm leading-relaxed text-muted-foreground">{detail}</p>}
      </div>
      {actionLabel && onAction && (
        <button
          type="button"
          onClick={onAction}
          disabled={busy}
          className="shrink-0 rounded-md bg-primary px-5 py-2.5 text-base font-medium text-primary-foreground transition-opacity duration-200 hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:opacity-50"
        >
          {busy ? "Working…" : actionLabel}
        </button>
      )}
    </section>
  );
}
