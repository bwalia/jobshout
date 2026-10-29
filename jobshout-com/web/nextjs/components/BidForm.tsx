"use client";

import Link from "next/link";
import { FormEvent, useState } from "react";
import { Button, ErrorNotice, Field, Input, Textarea, buttonClass } from "@/components/ui";
import { CheckCircleIcon, SendIcon } from "@/components/icons";
import type { MockProject } from "@/lib/projects";
import { formatProjectBudget } from "@/lib/projects";

type FieldErrors = Partial<Record<"amount" | "hourly_rate" | "estimated_hours" | "proposal" | "delivery_timeline", string>>;

export function BidForm({ project }: { project: MockProject }) {
  const isHourly = project.type === "hourly" || project.budget.kind === "hourly";
  const [submitted, setSubmitted] = useState(false);
  const [errors, setErrors] = useState<FieldErrors>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  if (submitted) {
    return (
      <div className="surface-card animate-rise-in p-8 text-center sm:p-10">
        <span className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-good/10 text-good">
          <CheckCircleIcon className="h-7 w-7" />
        </span>
        <h2 className="mt-5 font-display text-2xl font-semibold text-ink">Bid submitted</h2>
        <p className="mx-auto mt-3 max-w-md text-sm leading-relaxed text-mute">
          Your proposal for <span className="font-semibold text-ink">{project.title}</span> is with
          the client. They can shortlist or message you from their project inbox.
        </p>
        <div className="mt-7 flex flex-wrap justify-center gap-3">
          <Link href={`/projects/${project.id}`} className={buttonClass("primary", "md")}>
            Back to project
          </Link>
          <Link href="/projects" className={buttonClass("secondary", "md")}>
            Browse more projects
          </Link>
        </div>
      </div>
    );
  }

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setFormError(null);
    const fd = new FormData(e.currentTarget);
    const proposal = String(fd.get("proposal") ?? "").trim();
    const delivery = String(fd.get("delivery_timeline") ?? "").trim();
    const next: FieldErrors = {};

    if (proposal.length < 40) {
      next.proposal = "Write at least a short proposal (40+ characters) so the client knows your plan.";
    }
    if (!delivery) {
      next.delivery_timeline = "How long will this take?";
    }

    if (isHourly) {
      const rate = Number(fd.get("hourly_rate"));
      const hours = Number(fd.get("estimated_hours"));
      if (!rate || rate <= 0) next.hourly_rate = "Enter a positive hourly rate.";
      if (!hours || hours <= 0) next.estimated_hours = "Estimate total hours.";
    } else {
      const amount = Number(fd.get("amount"));
      if (!amount || amount <= 0) next.amount = "Enter your fixed bid amount.";
      if (project.budget.kind === "fixed") {
        if (amount < project.budget.min) {
          next.amount = `Below the client’s minimum ($${project.budget.min}).`;
        } else if (amount > project.budget.max * 1.5) {
          next.amount = `Far above the stated budget ($${project.budget.max}). Explain why in the proposal if intentional.`;
        }
      }
    }

    setErrors(next);
    if (Object.keys(next).length > 0) {
      setFormError("Fix the highlighted fields and try again.");
      return;
    }

    setPending(true);
    // Mock submit until the freelance API is wired to these pages.
    window.setTimeout(() => {
      setPending(false);
      setSubmitted(true);
    }, 600);
  }

  return (
    <form onSubmit={onSubmit} className="space-y-6" noValidate>
      {formError ? <ErrorNotice title={formError} /> : null}

      <p className="rounded-xl border border-line bg-subtle px-4 py-3 text-sm text-body">
        Client budget: <span className="font-semibold text-ink">{formatProjectBudget(project.budget)}</span>
        {" · "}
        {project.type.replace(/_/g, " ")}
      </p>

      {isHourly ? (
        <div className="grid gap-5 sm:grid-cols-2">
          <Field label="Your hourly rate (USD)" htmlFor="hourly_rate" required error={errors.hourly_rate}>
            <Input
              id="hourly_rate"
              name="hourly_rate"
              type="number"
              min={1}
              step="1"
              placeholder={project.budget.kind === "hourly" ? String(project.budget.rate) : "55"}
              aria-invalid={Boolean(errors.hourly_rate)}
            />
          </Field>
          <Field label="Estimated hours" htmlFor="estimated_hours" required error={errors.estimated_hours}>
            <Input
              id="estimated_hours"
              name="estimated_hours"
              type="number"
              min={1}
              step="1"
              placeholder="40"
              aria-invalid={Boolean(errors.estimated_hours)}
            />
          </Field>
        </div>
      ) : (
        <Field label="Your bid amount (USD)" htmlFor="amount" required error={errors.amount}>
          <Input
            id="amount"
            name="amount"
            type="number"
            min={1}
            step="1"
            placeholder={
              project.budget.kind === "fixed"
                ? String(Math.round((project.budget.min + project.budget.max) / 2))
                : "2500"
            }
            aria-invalid={Boolean(errors.amount)}
          />
        </Field>
      )}

      <Field
        label="Delivery timeline"
        htmlFor="delivery_timeline"
        required
        hint="Be specific — e.g. “3 weeks after kickoff” or “10 business days”."
        error={errors.delivery_timeline}
      >
        <Input
          id="delivery_timeline"
          name="delivery_timeline"
          placeholder={project.duration}
          aria-invalid={Boolean(errors.delivery_timeline)}
        />
      </Field>

      <Field
        label="Proposal"
        htmlFor="proposal"
        required
        hint="Cover approach, relevant experience, and how you’ll hit the deliverables."
        error={errors.proposal}
      >
        <Textarea
          id="proposal"
          name="proposal"
          rows={8}
          placeholder="I’ve shipped similar storefronts… Here’s how I’d approach your milestones…"
          aria-invalid={Boolean(errors.proposal)}
        />
      </Field>

      <div className="flex flex-wrap items-center gap-3 pt-2">
        <Button type="submit" size="lg" disabled={pending}>
          {pending ? (
            <>
              <span
                aria-hidden
                className="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
              />
              Submitting…
            </>
          ) : (
            <>
              <SendIcon className="h-4 w-4" />
              Submit bid
            </>
          )}
        </Button>
        <Link href={`/projects/${project.id}`} className={buttonClass("ghost", "md")}>
          Cancel
        </Link>
      </div>
    </form>
  );
}
