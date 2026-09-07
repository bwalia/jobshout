"use client";

import Link from "next/link";
import { useFormState, useFormStatus } from "react-dom";
import { useState } from "react";
import { applyAction, EMPTY_FORM_STATE } from "@/app/actions";
import { Button, ErrorNotice, Field, Input, Textarea, buttonClass } from "@/components/ui";
import { CheckCircleIcon, SendIcon } from "@/components/icons";
import type { Job } from "@/lib/api";

function SubmitButton() {
  const { pending } = useFormStatus();
  return (
    <Button type="submit" size="lg" disabled={pending} className="w-full sm:w-auto">
      {pending ? (
        <>
          <span
            aria-hidden
            className="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
          />
          Sending…
        </>
      ) : (
        <>
          <SendIcon className="h-4 w-4" />
          Send application
        </>
      )}
    </Button>
  );
}

export function ApplyForm({
  job,
  defaultName,
  defaultEmail,
}: {
  job: Job;
  defaultName: string;
  defaultEmail: string;
}) {
  const [state, formAction] = useFormState(applyAction, EMPTY_FORM_STATE);
  const [cvMode, setCvMode] = useState<"paste" | "link">("paste");

  if (state.ok) {
    return (
      <div className="surface-card animate-rise-in p-8 text-center sm:p-10">
        <span className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-good/10 text-good">
          <CheckCircleIcon className="h-7 w-7" />
        </span>
        <h2 className="mt-5 font-display text-2xl font-semibold text-ink">Application sent</h2>
        <p className="mx-auto mt-3 max-w-md text-sm leading-relaxed text-mute">
          Your application for <span className="font-semibold text-ink">{job.title}</span> is with
          the hiring team. You can track it any time from your applications page.
        </p>
        <div className="mt-7 flex flex-wrap justify-center gap-3">
          <Link href="/applications" className={buttonClass("primary", "md")}>
            Track my applications
          </Link>
          <Link href="/jobs" className={buttonClass("secondary", "md")}>
            Keep browsing
          </Link>
        </div>
      </div>
    );
  }

  const errors = state.fieldErrors ?? {};

  return (
    <form action={formAction} className="space-y-6" noValidate>
      <input type="hidden" name="job_id" value={job.id} />

      {state.message && !state.ok ? (
        <ErrorNotice title={state.message} />
      ) : null}

      <div className="grid gap-5 sm:grid-cols-2">
        <Field label="Full name" htmlFor="full_name" required error={errors.full_name}>
          <Input
            id="full_name"
            name="full_name"
            defaultValue={defaultName}
            autoComplete="name"
            placeholder="Ada Lovelace"
            aria-invalid={Boolean(errors.full_name)}
            aria-describedby={errors.full_name ? "full_name-error" : undefined}
          />
        </Field>

        <Field label="Email" htmlFor="email" required error={errors.email}>
          <Input
            id="email"
            name="email"
            type="email"
            defaultValue={defaultEmail}
            autoComplete="email"
            placeholder="you@example.com"
            aria-invalid={Boolean(errors.email)}
            aria-describedby={errors.email ? "email-error" : undefined}
          />
        </Field>
      </div>

      <Field label="Phone" htmlFor="phone" hint="Only shared with this employer.">
        <Input id="phone" name="phone" type="tel" autoComplete="tel" placeholder="+44 …" />
      </Field>

      <Field
        label="Message to the hiring team"
        htmlFor="cover_letter"
        hint="A short note beats a long one. Why this role, and what you have shipped."
        error={errors.cover_letter}
      >
        <Textarea
          id="cover_letter"
          name="cover_letter"
          rows={7}
          placeholder={`I would like to be considered for ${job.title}…`}
          aria-invalid={Boolean(errors.cover_letter)}
          aria-describedby={errors.cover_letter ? "cover_letter-error" : "cover_letter-hint"}
        />
      </Field>

      <div>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm font-semibold text-ink">Your CV</p>
          <div
            role="tablist"
            aria-label="How to supply your CV"
            className="inline-flex rounded-pill border border-line bg-raised p-1"
          >
            {(["paste", "link"] as const).map((mode) => (
              <button
                key={mode}
                type="button"
                role="tab"
                aria-selected={cvMode === mode}
                onClick={() => setCvMode(mode)}
                className={`cursor-pointer rounded-pill px-3.5 py-1.5 text-xs font-semibold transition-colors duration-200 ${
                  cvMode === mode ? "bg-surface text-ink shadow-card" : "text-mute hover:text-ink"
                }`}
              >
                {mode === "paste" ? "Paste text" : "Link"}
              </button>
            ))}
          </div>
        </div>

        <div className="mt-3">
          {/* Both inputs stay mounted so switching tabs never drops typed text. */}
          <div hidden={cvMode !== "paste"}>
            <Textarea
              id="cv_text"
              name="cv_text"
              rows={8}
              placeholder="Paste your CV as plain text — the matching agent reads this."
              aria-label="CV text"
            />
          </div>
          <div hidden={cvMode !== "link"}>
            <Input
              id="cv_url"
              name="cv_url"
              type="url"
              inputMode="url"
              placeholder="https://…"
              aria-label="Link to your CV"
              aria-invalid={Boolean(errors.cv_url)}
            />
            {errors.cv_url ? (
              <p role="alert" className="mt-1.5 text-xs font-medium text-shout">
                {errors.cv_url}
              </p>
            ) : null}
          </div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-4 border-t border-line pt-6">
        <SubmitButton />
        <p className="text-xs leading-relaxed text-mute">
          Applying again with the same email updates your existing application.
        </p>
      </div>
    </form>
  );
}
