"use client";

import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";
import type {
  CandidateProfile,
  EmploymentType,
  UpsertCandidateProfileInput,
} from "@/lib/api";
import { upsertProfile } from "@/lib/api";

const EMPLOYMENT: { value: EmploymentType; label: string }[] = [
  { value: "permanent", label: "Permanent" },
  { value: "contract", label: "Contract" },
  { value: "freelance", label: "Freelance" },
  { value: "temporary", label: "Temporary" },
  { value: "part_time", label: "Part-time" },
  { value: "internship", label: "Internship" },
  { value: "apprenticeship", label: "Apprenticeship" },
];

function splitCsv(value: string): string[] {
  return value
    .split(/[,;\n]/)
    .map((s) => s.trim())
    .filter(Boolean);
}

type Props = {
  initial?: CandidateProfile | null;
  defaultEmail?: string;
  defaultName?: string;
};

export function ProfileForm({ initial, defaultEmail = "", defaultName = "" }: Props) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState("");
  const [email, setEmail] = useState(initial?.email ?? defaultEmail);
  const [displayName, setDisplayName] = useState(initial?.display_name ?? defaultName);
  const [headline, setHeadline] = useState(initial?.headline ?? "");
  const [summary, setSummary] = useState(initial?.summary ?? "");
  const [skills, setSkills] = useState((initial?.skills ?? []).join(", "));
  const [years, setYears] = useState(
    initial?.years_experience != null ? String(initial.years_experience) : "",
  );
  const [roles, setRoles] = useState((initial?.preferred_roles ?? []).join(", "));
  const [country, setCountry] = useState(initial?.preferred_locations?.[0]?.country ?? "GB");
  const [city, setCity] = useState(initial?.preferred_locations?.[0]?.city ?? "");
  const [openToRemote, setOpenToRemote] = useState(initial?.open_to_remote ?? true);
  const [employment, setEmployment] = useState<EmploymentType[]>(
    initial?.preferred_employment_types?.length
      ? initial.preferred_employment_types
      : ["permanent", "contract"],
  );
  const [currency, setCurrency] = useState(initial?.salary_expectation?.currency ?? "GBP");
  const [minSalary, setMinSalary] = useState(
    initial?.salary_expectation?.min_amount != null
      ? String(initial.salary_expectation.min_amount)
      : "",
  );
  const [cvText, setCvText] = useState(initial?.cv_text ?? "");
  const [notes, setNotes] = useState(initial?.matching_notes ?? "");

  function toggleEmployment(value: EmploymentType) {
    setEmployment((prev) =>
      prev.includes(value) ? prev.filter((x) => x !== value) : [...prev, value],
    );
  }

  function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    const payload: UpsertCandidateProfileInput = {
      email,
      display_name: displayName,
      headline,
      summary,
      skills: splitCsv(skills),
      years_experience: years.trim() === "" ? null : Number(years),
      preferred_roles: splitCsv(roles),
      preferred_locations: [
        {
          country: country.trim() || "GB",
          city: city.trim() || null,
          remote: openToRemote,
        },
      ],
      preferred_employment_types: employment,
      open_to_remote: openToRemote,
      salary_expectation: {
        currency: currency || null,
        min_amount: minSalary.trim() === "" ? null : Number(minSalary),
        period: "annual",
      },
      cv_text: cvText,
      matching_notes: notes,
    };

    startTransition(async () => {
      try {
        const saved = await upsertProfile(payload);
        if (typeof window !== "undefined") {
          window.localStorage.setItem("jobshout_profile_email", saved.email);
          window.localStorage.setItem("jobshout_profile_id", saved.id);
        }
        router.push(`/profile/matches?id=${saved.id}`);
        router.refresh();
      } catch (err) {
        setError(err instanceof Error ? err.message : "Could not save profile");
      }
    });
  }

  const field =
    "mt-1.5 w-full border border-line bg-white px-3 py-2.5 text-sm text-ink outline-none transition focus:border-signal";

  return (
    <form onSubmit={onSubmit} className="space-y-8">
      {error && (
        <p className="border border-shout/30 bg-shout/5 px-4 py-3 text-sm text-ink">{error}</p>
      )}

      <section className="space-y-4">
        <h2 className="font-display text-2xl tracking-tight">Who you are</h2>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block text-sm font-medium">
            Email
            <input
              required
              type="email"
              className={field}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </label>
          <label className="block text-sm font-medium">
            Display name
            <input
              required
              className={field}
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
            />
          </label>
        </div>
        <label className="block text-sm font-medium">
          Headline
          <input
            className={field}
            placeholder="Senior Rust engineer · marketplace systems"
            value={headline}
            onChange={(e) => setHeadline(e.target.value)}
          />
        </label>
        <label className="block text-sm font-medium">
          Summary for the matching agent
          <textarea
            rows={4}
            className={field}
            placeholder="What you want next, strengths, domains you care about…"
            value={summary}
            onChange={(e) => setSummary(e.target.value)}
          />
        </label>
      </section>

      <section className="space-y-4">
        <h2 className="font-display text-2xl tracking-tight">Skills & roles</h2>
        <p className="text-sm text-mute">
          Skills and preferred roles are the primary signals the Career agent uses to rank jobs.
        </p>
        <label className="block text-sm font-medium">
          Skills (comma-separated)
          <input
            className={field}
            placeholder="Rust, Axum, PostgreSQL, Kubernetes"
            value={skills}
            onChange={(e) => setSkills(e.target.value)}
          />
        </label>
        <label className="block text-sm font-medium">
          Preferred roles (comma-separated)
          <input
            className={field}
            placeholder="Rust Engineer, Platform Engineer"
            value={roles}
            onChange={(e) => setRoles(e.target.value)}
          />
        </label>
        <label className="block text-sm font-medium">
          Years of experience
          <input
            type="number"
            min={0}
            max={60}
            className={field}
            value={years}
            onChange={(e) => setYears(e.target.value)}
          />
        </label>
        <fieldset>
          <legend className="text-sm font-medium">Employment types</legend>
          <div className="mt-2 flex flex-wrap gap-2">
            {EMPLOYMENT.map((opt) => {
              const on = employment.includes(opt.value);
              return (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => toggleEmployment(opt.value)}
                  className={`px-3 py-1.5 text-xs font-semibold ${
                    on ? "bg-ink text-white" : "bg-paper text-mute"
                  }`}
                >
                  {opt.label}
                </button>
              );
            })}
          </div>
        </fieldset>
      </section>

      <section className="space-y-4">
        <h2 className="font-display text-2xl tracking-tight">Location & pay</h2>
        <div className="grid gap-4 sm:grid-cols-3">
          <label className="block text-sm font-medium">
            Country
            <input className={field} value={country} onChange={(e) => setCountry(e.target.value)} />
          </label>
          <label className="block text-sm font-medium">
            City
            <input className={field} value={city} onChange={(e) => setCity(e.target.value)} />
          </label>
          <label className="flex items-end gap-2 pb-2 text-sm font-medium">
            <input
              type="checkbox"
              checked={openToRemote}
              onChange={(e) => setOpenToRemote(e.target.checked)}
              className="h-4 w-4 border-line"
            />
            Open to remote
          </label>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block text-sm font-medium">
            Salary currency
            <input
              className={field}
              value={currency}
              onChange={(e) => setCurrency(e.target.value)}
            />
          </label>
          <label className="block text-sm font-medium">
            Minimum annual expectation
            <input
              type="number"
              className={field}
              value={minSalary}
              onChange={(e) => setMinSalary(e.target.value)}
            />
          </label>
        </div>
      </section>

      <section className="space-y-4">
        <h2 className="font-display text-2xl tracking-tight">Agent context</h2>
        <label className="block text-sm font-medium">
          CV / resume text (optional)
          <textarea
            rows={6}
            className={field}
            placeholder="Paste plain-text CV content the agent can scan…"
            value={cvText}
            onChange={(e) => setCvText(e.target.value)}
          />
        </label>
        <label className="block text-sm font-medium">
          Matching notes for the agent
          <textarea
            rows={3}
            className={field}
            placeholder="e.g. Prefer deep systems work; avoid pure frontend; OK with EU timezone overlap"
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
          />
        </label>
      </section>

      <button
        type="submit"
        disabled={pending}
        className="bg-shout px-6 py-3 text-sm font-semibold text-white transition hover:brightness-110 disabled:opacity-60"
      >
        {pending ? "Saving…" : "Save profile & see matches"}
      </button>
    </form>
  );
}
