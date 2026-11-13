"use client";

import Link from "next/link";
import { FormEvent, useState } from "react";
import { useSession } from "next-auth/react";
import { Button, ErrorNotice, Field, Input, Textarea, buttonClass } from "@/components/ui";
import { CheckCircleIcon, SendIcon } from "@/components/icons";
import { formatProjectBudget, submitBid, type Project } from "@/lib/projects";

type FieldErrors = Partial<
  Record<"amount" | "hourly_rate" | "estimated_hours" | "proposal" | "delivery_timeline", string>
>;

export function BidForm({ project }: { project: Project }) {
  const { data: session } = useSession();
  const isHourly = project.project_type === "hourly";
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
          the client.
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

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setFormError(null);
    if (!session?.user?.email) {
      setFormError("Sign in to submit a proposal.");
      return;
    }
    const fd = new FormData(e.currentTarget);
    const proposal = String(fd.get("proposal") ?? "").trim();
    const delivery = String(fd.get("delivery_timeline") ?? "").trim();
    const next: FieldErrors = {};

    if (proposal.length < 40) {
      next.proposal = "Write at least a short proposal (40+ characters).";
    }
    if (!delivery) next.delivery_timeline = "How long will this take?";

    let bid_amount: number | null = null;
    let hourly_rate: number | null = null;
    let estimated_hours: number | null = null;
    if (isHourly) {
      hourly_rate = Number(fd.get("hourly_rate")) || null;
      estimated_hours = Number(fd.get("estimated_hours")) || null;
      if (!hourly_rate || hourly_rate <= 0) next.hourly_rate = "Enter a positive hourly rate.";
      if (!estimated_hours || estimated_hours <= 0) next.estimated_hours = "Estimate hours.";
    } else {
      bid_amount = Number(fd.get("amount")) || null;
      if (!bid_amount || bid_amount <= 0) next.amount = "Enter a positive bid amount.";
    }

    setErrors(next);
    if (Object.keys(next).length) return;

    setPending(true);
    try {
      await submitBid(
        { email: session.user.email, name: session.user.name ?? null },
        project.id,
        {
          bid_amount,
          hourly_rate,
          estimated_hours,
          proposal,
          delivery_timeline: delivery,
        },
      );
      setSubmitted(true);
    } catch (err) {
      setFormError(err instanceof Error ? err.message : "Could not submit bid");
    } finally {
      setPending(false);
    }
  }

  return (
    <form onSubmit={onSubmit} className="surface-card space-y-4 p-5 sm:p-6">
      <div>
        <h3 className="font-display text-lg font-semibold text-ink">Your proposal</h3>
        <p className="mt-1 text-sm text-mute">Budget: {formatProjectBudget(project)}</p>
      </div>
      {formError ? <ErrorNotice title={formError} /> : null}
      {isHourly ? (
        <div className="grid grid-cols-2 gap-3">
          <Field label="Hourly rate" htmlFor="hourly_rate" error={errors.hourly_rate}>
            <Input id="hourly_rate" name="hourly_rate" type="number" min={1} step="1" />
          </Field>
          <Field label="Estimated hours" htmlFor="estimated_hours" error={errors.estimated_hours}>
            <Input id="estimated_hours" name="estimated_hours" type="number" min={1} step="1" />
          </Field>
        </div>
      ) : (
        <Field label="Bid amount" htmlFor="amount" error={errors.amount}>
          <Input id="amount" name="amount" type="number" min={1} step="1" />
        </Field>
      )}
      <Field label="Proposal" htmlFor="proposal" error={errors.proposal}>
        <Textarea id="proposal" name="proposal" rows={6} required />
      </Field>
      <Field label="Delivery timeline" htmlFor="delivery_timeline" error={errors.delivery_timeline}>
        <Input id="delivery_timeline" name="delivery_timeline" placeholder="2 weeks" />
      </Field>
      <Button type="submit" disabled={pending}>
        <SendIcon className="h-4 w-4" />
        {pending ? "Submitting…" : "Submit proposal"}
      </Button>
    </form>
  );
}
