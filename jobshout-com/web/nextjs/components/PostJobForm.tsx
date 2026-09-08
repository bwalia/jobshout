"use client";

import Link from "next/link";
import { useState } from "react";
import { useFormState, useFormStatus } from "react-dom";
import { createJobAction, EMPTY_FORM_STATE } from "@/app/actions";
import { JobCard } from "@/components/JobCard";
import {
  Button,
  Checkbox,
  ErrorNotice,
  Field,
  Input,
  Select,
  Textarea,
  buttonClass,
} from "@/components/ui";
import { CheckCircleIcon } from "@/components/icons";
import { EMPLOYMENT_TYPES, type EmploymentType, type Job } from "@/lib/api";
import { employmentLabel } from "@/lib/format";

const CURRENCIES = ["GBP", "USD", "EUR", "INR", "AUD", "CAD"];
const PERIODS = [
  { value: "annual", label: "per year" },
  { value: "monthly", label: "per month" },
  { value: "weekly", label: "per week" },
  { value: "daily", label: "per day" },
  { value: "hourly", label: "per hour" },
];

function SubmitButton({ draft }: { draft: boolean }) {
  const { pending } = useFormStatus();
  return (
    <Button
      type="submit"
      size="lg"
      name="publish"
      value={draft ? "draft" : "publish"}
      variant={draft ? "secondary" : "primary"}
      disabled={pending}
    >
      {pending ? "Saving…" : draft ? "Save as draft" : "Publish to the board"}
    </Button>
  );
}

export function PostJobForm() {
  const [state, formAction] = useFormState(createJobAction, EMPTY_FORM_STATE);

  // Mirrored locally so the preview updates as the employer types.
  const [draft, setDraft] = useState({
    title: "",
    summary: "",
    description: "",
    employment_type: "permanent" as EmploymentType,
    country: "GB",
    city: "",
    region: "",
    remote: true,
    currency: "GBP",
    min_amount: "",
    max_amount: "",
    period: "annual",
    requirements: "",
  });

  function set<K extends keyof typeof draft>(key: K, value: (typeof draft)[K]) {
    setDraft((d) => ({ ...d, [key]: value }));
  }

  if (state.ok && state.result?.jobId) {
    const published = state.result.status === "published";
    return (
      <div className="surface-card animate-rise-in p-8 text-center sm:p-12">
        <span className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-good/10 text-good">
          <CheckCircleIcon className="h-7 w-7" />
        </span>
        <h2 className="mt-5 font-display text-2xl font-semibold text-ink">
          {published ? "Your job is live" : "Draft saved"}
        </h2>
        <p className="mx-auto mt-3 max-w-md text-sm leading-relaxed text-mute">
          {published
            ? "It is on the public board now and Hiring Agents can start ranking candidates against it."
            : "It is saved but not on the public board. Publish it when the details are settled."}
        </p>
        <div className="mt-7 flex flex-wrap justify-center gap-3">
          <Link href={`/jobs/${state.result.jobId}`} className={buttonClass("primary", "md")}>
            View the listing
          </Link>
          <Link href="/post-job" className={buttonClass("secondary", "md")}>
            Post another
          </Link>
        </div>
      </div>
    );
  }

  const errors = state.fieldErrors ?? {};

  return (
    <div className="grid gap-10 lg:grid-cols-[1fr_22rem] lg:gap-14">
      <form action={formAction} className="min-w-0 space-y-10" noValidate>
        {state.message && !state.ok ? <ErrorNotice title={state.message} /> : null}

        <Section title="The role" step={1}>
          <Field label="Job title" htmlFor="title" required error={errors.title}>
            <Input
              id="title"
              name="title"
              value={draft.title}
              onChange={(e) => set("title", e.target.value)}
              placeholder="Senior Rust Engineer"
              aria-invalid={Boolean(errors.title)}
            />
          </Field>

          <Field
            label="One-line summary"
            htmlFor="summary"
            hint="The line candidates read first, on the card and in search results."
          >
            <Input
              id="summary"
              name="summary"
              value={draft.summary}
              onChange={(e) => set("summary", e.target.value)}
              placeholder="Build the marketplace core in Rust, with a team that ships weekly."
              maxLength={160}
            />
          </Field>

          <Field
            label="Description"
            htmlFor="description"
            required
            hint="What the job actually is, who it reports to, and what success looks like."
            error={errors.description}
          >
            <Textarea
              id="description"
              name="description"
              rows={11}
              value={draft.description}
              onChange={(e) => set("description", e.target.value)}
              placeholder={"## About the role\n\nYou will…\n\n## Responsibilities\n- …"}
              aria-invalid={Boolean(errors.description)}
            />
          </Field>

          <Field
            label="Requirements"
            htmlFor="requirements"
            hint="One per line, or comma separated. These drive candidate matching."
          >
            <Textarea
              id="requirements"
              name="requirements"
              rows={5}
              value={draft.requirements}
              onChange={(e) => set("requirements", e.target.value)}
              placeholder={"Rust\nPostgreSQL\nKubernetes"}
            />
          </Field>
        </Section>

        <Section title="Where and how" step={2}>
          <div className="grid gap-5 sm:grid-cols-2">
            <Field label="Work type" htmlFor="employment_type" required>
              <Select
                id="employment_type"
                name="employment_type"
                value={draft.employment_type}
                onChange={(e) => set("employment_type", e.target.value as EmploymentType)}
              >
                {EMPLOYMENT_TYPES.map((t) => (
                  <option key={t} value={t}>
                    {employmentLabel(t)}
                  </option>
                ))}
              </Select>
            </Field>

            <Field label="Country" htmlFor="country" required error={errors.country}>
              <Input
                id="country"
                name="country"
                value={draft.country}
                onChange={(e) => set("country", e.target.value)}
                placeholder="GB"
                aria-invalid={Boolean(errors.country)}
              />
            </Field>

            <Field label="City" htmlFor="city">
              <Input
                id="city"
                name="city"
                value={draft.city}
                onChange={(e) => set("city", e.target.value)}
                placeholder="London"
              />
            </Field>

            <Field label="Region" htmlFor="region">
              <Input
                id="region"
                name="region"
                value={draft.region}
                onChange={(e) => set("region", e.target.value)}
                placeholder="England"
              />
            </Field>
          </div>

          <div className="mt-1 rounded-xl border border-line bg-raised px-4 py-2">
            <Checkbox
              label="Open to remote candidates"
              name="remote"
              checked={draft.remote}
              onChange={(e) => set("remote", e.target.checked)}
            />
          </div>
        </Section>

        <Section
          title="Pay"
          step={3}
          note="Listings with a salary get materially more applications. Leave it blank only if you must."
        >
          <div className="grid gap-5 sm:grid-cols-4">
            <Field label="Currency" htmlFor="currency">
              <Select
                id="currency"
                name="currency"
                value={draft.currency}
                onChange={(e) => set("currency", e.target.value)}
              >
                {CURRENCIES.map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </Select>
            </Field>

            <Field label="Minimum" htmlFor="min_amount">
              <Input
                id="min_amount"
                name="min_amount"
                inputMode="numeric"
                value={draft.min_amount}
                onChange={(e) => set("min_amount", e.target.value)}
                placeholder="90000"
              />
            </Field>

            <Field label="Maximum" htmlFor="max_amount" error={errors.max_amount}>
              <Input
                id="max_amount"
                name="max_amount"
                inputMode="numeric"
                value={draft.max_amount}
                onChange={(e) => set("max_amount", e.target.value)}
                placeholder="130000"
                aria-invalid={Boolean(errors.max_amount)}
              />
            </Field>

            <Field label="Period" htmlFor="period">
              <Select
                id="period"
                name="period"
                value={draft.period}
                onChange={(e) => set("period", e.target.value)}
              >
                {PERIODS.map((p) => (
                  <option key={p.value} value={p.value}>
                    {p.label}
                  </option>
                ))}
              </Select>
            </Field>
          </div>
        </Section>

        <div className="flex flex-wrap items-center gap-3 border-t border-line pt-8">
          <SubmitButton draft={false} />
          <SubmitButton draft={true} />
          <p className="w-full text-xs leading-relaxed text-mute sm:w-auto sm:flex-1">
            Published roles appear on the board straight away.
          </p>
        </div>
      </form>

      {/* Live preview: exactly the card candidates will see on the board. */}
      <aside className="lg:sticky lg:top-24 lg:self-start">
        <p className="text-xs font-semibold uppercase tracking-[0.14em] text-mute">
          Live preview
        </p>
        <p className="mt-2 text-sm leading-relaxed text-mute">
          How your listing appears on the board.
        </p>
        <div className="mt-5">
          <JobCard job={previewJob(draft)} />
        </div>
      </aside>
    </div>
  );
}

function Section({
  title,
  step,
  note,
  children,
}: {
  title: string;
  step: number;
  note?: string;
  children: React.ReactNode;
}) {
  return (
    <section className="space-y-5">
      <div className="flex items-center gap-3 border-b border-line pb-3">
        <span className="flex h-7 w-7 items-center justify-center rounded-full bg-ink text-xs font-bold text-bg">
          {step}
        </span>
        <h2 className="font-display text-lg font-semibold text-ink">{title}</h2>
      </div>
      {note ? <p className="text-sm leading-relaxed text-mute">{note}</p> : null}
      {children}
    </section>
  );
}

/** Shape the in-progress form into a Job so the real card can render it. */
function previewJob(draft: {
  title: string;
  summary: string;
  description: string;
  employment_type: EmploymentType;
  country: string;
  city: string;
  region: string;
  remote: boolean;
  currency: string;
  min_amount: string;
  max_amount: string;
  period: string;
  requirements: string;
}): Job {
  const num = (v: string) => {
    const n = Number(v.replace(/[, ]/g, ""));
    return Number.isFinite(n) && n > 0 ? n : null;
  };
  const now = new Date().toISOString();

  return {
    id: "preview",
    organisation_id: "preview",
    title: draft.title || "Your job title",
    summary: draft.summary || "A one-line summary helps candidates self-select quickly.",
    description: draft.description,
    employment_type: draft.employment_type,
    location: {
      country: draft.country || "—",
      city: draft.city || null,
      region: draft.region || null,
      remote: draft.remote,
    },
    compensation: {
      currency: draft.currency,
      min_amount: num(draft.min_amount),
      max_amount: num(draft.max_amount),
      period: draft.period,
    },
    requirements: draft.requirements
      .split(/[\n,]/)
      .map((s) => s.trim())
      .filter(Boolean),
    status: "published",
    created_at: now,
    updated_at: now,
    published_at: now,
  };
}
