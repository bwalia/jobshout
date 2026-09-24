"use client";

import { useFormState, useFormStatus } from "react-dom";
import { EMPTY_FORM_STATE } from "@/app/actions";
import { subscribeAction } from "@/app/insights/actions";
import { CheckCircleIcon, MailIcon } from "@/components/icons";
import { Button, cx } from "@/components/ui";

function Submit() {
  const { pending } = useFormStatus();
  return (
    <Button type="submit" disabled={pending} className="shrink-0">
      {pending ? "Subscribing…" : "Subscribe"}
    </Button>
  );
}

export function NewsletterSignup({ compact }: { compact?: boolean }) {
  const [state, action] = useFormState(subscribeAction, EMPTY_FORM_STATE);

  return (
    <section
      aria-labelledby="newsletter-heading"
      className={cx(
        "relative overflow-hidden rounded-card border border-line bg-surface",
        compact ? "p-5" : "p-6 sm:p-10",
      )}
    >
      {!compact ? <div aria-hidden className="pointer-events-none absolute inset-0 halo opacity-70" /> : null}
      <div className={cx("relative", !compact && "grid gap-6 md:grid-cols-[1fr_minmax(0,26rem)] md:items-center")}>
        <div>
          <span className="flex h-10 w-10 items-center justify-center rounded-xl border border-line bg-raised text-shout">
            <MailIcon className="h-5 w-5" />
          </span>
          <h2
            id="newsletter-heading"
            className={cx(
              "mt-4 font-display font-semibold tracking-[-0.02em] text-ink",
              compact ? "text-lg" : "text-2xl sm:text-3xl",
            )}
          >
            The week in AI and work
          </h2>
          <p className="mt-2 text-sm leading-relaxed text-mute">
            One email a week with what changed in AI and the job market, plus the best of Insights. Unsubscribe any time.
          </p>
        </div>

        {state.ok ? (
          <p role="status" className="mt-4 flex items-start gap-2.5 rounded-xl border border-good/30 bg-good/10 px-4 py-3 text-sm text-ink md:mt-0">
            <CheckCircleIcon className="mt-0.5 h-4 w-4 shrink-0 text-good" />
            {state.message}
          </p>
        ) : (
          <form action={action} className={cx("mt-4 md:mt-0", compact ? "space-y-2" : "")} noValidate>
            <div className={cx("flex gap-2", compact ? "flex-col" : "flex-col sm:flex-row")}>
              <label htmlFor={compact ? "nl-email-c" : "nl-email"} className="sr-only">
                Email address
              </label>
              <input
                id={compact ? "nl-email-c" : "nl-email"}
                name="email"
                type="email"
                autoComplete="email"
                required
                placeholder="you@example.com"
                aria-invalid={Boolean(state.fieldErrors?.email)}
                aria-describedby={state.fieldErrors?.email ? "nl-error" : undefined}
                className={cx("h-11 w-full min-w-0 rounded-pill", !compact && "sm:flex-1", " border border-line bg-surface px-4 text-sm text-ink placeholder:text-mute/70 focus:border-shout focus:outline-none focus:ring-2 focus:ring-shout/25")}
              />
              <Submit />
            </div>
            {state.message && !state.ok ? (
              <p id="nl-error" role="alert" className="mt-2 text-xs font-medium text-shout">
                {state.message}
              </p>
            ) : null}
          </form>
        )}
      </div>
    </section>
  );
}
