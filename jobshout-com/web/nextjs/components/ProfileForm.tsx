"use client";

import Link from "next/link";
import { useState } from "react";
import { useFormState, useFormStatus } from "react-dom";
import { EMPTY_FORM_STATE, saveProfileAction } from "@/app/actions";
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
import { CheckCircleIcon, TargetIcon } from "@/components/icons";
import { EMPLOYMENT_TYPES, type CandidateProfile } from "@/lib/api";
import { employmentLabel } from "@/lib/format";

const CURRENCIES = ["GBP", "USD", "EUR", "INR", "AUD", "CAD"];
const PERIODS = ["annual", "monthly", "weekly", "daily", "hourly"];

function SubmitButton() {
  const { pending } = useFormStatus();
  return (
    <Button type="submit" size="lg" disabled={pending}>
      {pending ? "Saving…" : "Save profile"}
    </Button>
  );
}

export function ProfileForm({
  initial,
  defaultEmail,
  defaultName,
}: {
  initial: CandidateProfile | null;
  defaultEmail: string;
  defaultName: string;
}) {
  const [state, formAction] = useFormState(saveProfileAction, EMPTY_FORM_STATE);
  const [skills, setSkills] = useState(initial?.skills.join(", ") ?? "");

  const errors = state.fieldErrors ?? {};
  const savedId = state.result?.profileId ?? initial?.id;
  const location = initial?.preferred_locations?.[0];
  const skillCount = skills.split(/[\n,]/).filter((s) => s.trim()).length;

  return (
    <form action={formAction} className="space-y-10" noValidate>
      {state.ok ? (
        <div
          role="status"
          className="flex flex-wrap items-center gap-4 rounded-card border border-good/30 bg-good/[0.08] px-5 py-4"
        >
          <CheckCircleIcon className="h-5 w-5 shrink-0 text-good" />
          <p className="flex-1 text-sm font-medium text-ink">
            {state.message} Your Career Agent can rank open roles against it now.
          </p>
          {savedId ? (
            <Link
              href={`/profile/matches?id=${savedId}`}
              className={buttonClass("primary", "sm")}
            >
              <TargetIcon className="h-4 w-4" />
              See my matches
            </Link>
          ) : null}
        </div>
      ) : null}

      {state.message && !state.ok ? <ErrorNotice title={state.message} /> : null}

      <Section title="Who you are" step={1}>
        <div className="grid gap-5 sm:grid-cols-2">
          <Field label="Display name" htmlFor="display_name" required error={errors.display_name}>
            <Input
              id="display_name"
              name="display_name"
              defaultValue={initial?.display_name || defaultName}
              autoComplete="name"
              placeholder="Ada Lovelace"
              aria-invalid={Boolean(errors.display_name)}
            />
          </Field>

          <Field
            label="Email"
            htmlFor="email"
            required
            hint="Your profile is keyed to this address."
            error={errors.email}
          >
            <Input
              id="email"
              name="email"
              type="email"
              defaultValue={initial?.email || defaultEmail}
              autoComplete="email"
              placeholder="you@example.com"
              aria-invalid={Boolean(errors.email)}
            />
          </Field>
        </div>

        <Field label="Headline" htmlFor="headline" hint="One line, the way you would introduce yourself.">
          <Input
            id="headline"
            name="headline"
            defaultValue={initial?.headline}
            placeholder="Backend engineer — Rust, Postgres, distributed systems"
            maxLength={120}
          />
        </Field>

        <Field label="Summary" htmlFor="summary">
          <Textarea
            id="summary"
            name="summary"
            rows={4}
            defaultValue={initial?.summary}
            placeholder="What you have built, and what you want to build next."
          />
        </Field>
      </Section>

      <Section title="What you do" step={2}>
        <Field
          label="Skills"
          htmlFor="skills"
          hint="Comma separated. These carry the most weight in matching."
        >
          <Textarea
            id="skills"
            name="skills"
            rows={3}
            value={skills}
            onChange={(e) => setSkills(e.target.value)}
            placeholder="Rust, PostgreSQL, Kubernetes, Axum"
          />
          <p className="mt-1.5 text-xs text-mute">
            {skillCount} {skillCount === 1 ? "skill" : "skills"} listed
            {skillCount > 0 && skillCount < 4 ? " — add a few more for better matches." : ""}
          </p>
        </Field>

        <div className="grid gap-5 sm:grid-cols-2">
          <Field label="Preferred roles" htmlFor="preferred_roles" hint="Comma separated.">
            <Input
              id="preferred_roles"
              name="preferred_roles"
              defaultValue={initial?.preferred_roles.join(", ")}
              placeholder="Senior Engineer, Staff Engineer"
            />
          </Field>

          <Field label="Years of experience" htmlFor="years_experience">
            <Input
              id="years_experience"
              name="years_experience"
              inputMode="numeric"
              defaultValue={initial?.years_experience ?? ""}
              placeholder="8"
            />
          </Field>
        </div>

        <fieldset>
          <legend className="text-sm font-semibold text-ink">Work types you would take</legend>
          <div className="mt-3 flex flex-wrap gap-x-6">
            {EMPLOYMENT_TYPES.map((t) => (
              <Checkbox
                key={t}
                name="preferred_employment_types"
                value={t}
                label={employmentLabel(t)}
                defaultChecked={initial?.preferred_employment_types.includes(t)}
              />
            ))}
          </div>
        </fieldset>
      </Section>

      <Section title="Where and for how much" step={3}>
        <div className="grid gap-5 sm:grid-cols-2">
          <Field label="Country" htmlFor="country">
            <Input
              id="country"
              name="country"
              defaultValue={location?.country ?? ""}
              placeholder="GB"
            />
          </Field>
          <Field label="City" htmlFor="city">
            <Input
              id="city"
              name="city"
              defaultValue={location?.city ?? ""}
              placeholder="London"
            />
          </Field>
        </div>

        <div className="rounded-xl border border-line bg-raised px-4 py-2">
          <Checkbox
            name="open_to_remote"
            label="Open to remote work"
            defaultChecked={initial?.open_to_remote ?? true}
          />
        </div>

        <div className="grid gap-5 sm:grid-cols-3">
          <Field label="Currency" htmlFor="currency">
            <Select
              id="currency"
              name="currency"
              defaultValue={initial?.salary_expectation.currency ?? "GBP"}
            >
              {CURRENCIES.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </Select>
          </Field>

          <Field label="Salary floor" htmlFor="salary_min">
            <Input
              id="salary_min"
              name="salary_min"
              inputMode="numeric"
              defaultValue={initial?.salary_expectation.min_amount ?? ""}
              placeholder="90000"
            />
          </Field>

          <Field label="Period" htmlFor="period">
            <Select
              id="period"
              name="period"
              defaultValue={initial?.salary_expectation.period ?? "annual"}
            >
              {PERIODS.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </Select>
          </Field>
        </div>
      </Section>

      <Section title="For the agent" step={4}>
        <Field
          label="CV text"
          htmlFor="cv_text"
          hint="Paste your CV as plain text. Never shared without your say-so."
        >
          <Textarea id="cv_text" name="cv_text" rows={8} defaultValue={initial?.cv_text} />
        </Field>

        <Field
          label="Matching notes"
          htmlFor="matching_notes"
          hint="Hard constraints your Career Agent must respect."
        >
          <Textarea
            id="matching_notes"
            name="matching_notes"
            rows={3}
            defaultValue={initial?.matching_notes}
            placeholder="No relocation. Four-day week preferred. Not interested in adtech."
          />
        </Field>
      </Section>

      <div className="flex flex-wrap items-center gap-4 border-t border-line pt-8">
        <SubmitButton />
        {savedId ? (
          <Link href={`/profile/matches?id=${savedId}`} className={buttonClass("secondary", "lg")}>
            See ranked matches
          </Link>
        ) : null}
      </div>
    </form>
  );
}

function Section({
  title,
  step,
  children,
}: {
  title: string;
  step: number;
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
      {children}
    </section>
  );
}
