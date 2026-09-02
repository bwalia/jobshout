"use client";

import { useMemo, useState } from "react";
import { FieldHint, FieldLabel } from "@/components/ui/field-hint";
import type { CareerArtifact, CareerDoctorReport, CareerPatterns, CareerPortal } from "@/types/career";
import { CareerJDDialog, type CareerJDPreview } from "./CareerJDDialog";
import { CareerJobDetail } from "./CareerJobDetail";
import { postingHref, type CareerJob } from "./jobs";

export function CareerJobsPanel({
  doctor,
  jobs,
  selected,
  onSelect,
  artifacts,
  draftNote,
  busy,
  hasCV,
  jobUrl,
  setJobUrl,
  jdText,
  setJdText,
  onAddJob,
  scanBoard,
  setScanBoard,
  scanSlug,
  setScanSlug,
  portals,
  onScanAll,
  onScanOne,
  onSavePortal,
  patterns,
  onScore,
  onScoreSelected,
  onTailor,
  onCover,
  onEmail,
  onApplied,
  onSeeJD,
  jdPreview,
  onCloseJD,
}: {
  doctor: CareerDoctorReport | null;
  jobs: CareerJob[];
  selected: CareerJob | null;
  onSelect: (job: CareerJob) => void;
  artifacts: CareerArtifact[];
  draftNote: string;
  busy: boolean;
  hasCV: boolean;
  jobUrl: string;
  setJobUrl: (v: string) => void;
  jdText: string;
  setJdText: (v: string) => void;
  onAddJob: () => void;
  scanBoard: string;
  setScanBoard: (v: string) => void;
  scanSlug: string;
  setScanSlug: (v: string) => void;
  portals: CareerPortal[];
  onScanAll: () => void;
  onScanOne: () => void;
  onSavePortal: () => void;
  patterns: CareerPatterns | null;
  onScore: () => void;
  onScoreSelected: (jobs: CareerJob[]) => void;
  onTailor: () => void;
  onCover: () => void;
  onEmail: () => void;
  onApplied: () => void;
  onSeeJD: (job: CareerJob) => void;
  jdPreview: CareerJDPreview | null;
  onCloseJD: () => void;
}) {
  const [checked, setChecked] = useState<Record<string, boolean>>({});
  const selectedJobs = useMemo(() => jobs.filter((j) => checked[j.key]), [jobs, checked]);
  const allChecked = jobs.length > 0 && jobs.every((j) => checked[j.key]);

  function toggle(key: string, value: boolean) {
    setChecked((prev) => ({ ...prev, [key]: value }));
  }

  function toggleAll(value: boolean) {
    const next: Record<string, boolean> = {};
    if (value) {
      for (const j of jobs) next[j.key] = true;
    }
    setChecked(next);
  }

  return (
    <div className="space-y-5">
      {doctor && !doctor.ok && (
        <p className="rounded-md border border-signal-warn/40 bg-muted/40 px-3 py-2 text-sm">
          Finish Profile first (name and CV) so scores and tailored CVs are about you, not a blank page.
        </p>
      )}
      {doctor?.ok && doctor.warnings.length + doctor.info.length > 0 && (
        <p className="text-xs text-muted-foreground">{[...doctor.warnings, ...doctor.info].join(" · ")}</p>
      )}

      <section className="space-y-3">
        <h3 className="text-sm font-medium">Find jobs</h3>
        <p className="text-sm text-muted-foreground">
          Scan the CareerOps company list on Greenhouse, Ashby, and Lever, or paste one posting.
          This is not LinkedIn or Indeed.
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            disabled={busy}
            onClick={onScanAll}
            className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
          >
            {busy ? "Scanning…" : "Scan all companies"}
          </button>
          <FieldHint text="Hits every saved company on Greenhouse, Ashby, and Lever. Title filter comes from Profile. A minute or two." />
        </div>
        {portals.length > 0 && (
          <p className="text-xs text-muted-foreground">
            Watching {portals.length} companies
            {portalBoardSummary(portals) ? ` (${portalBoardSummary(portals)})` : ""}.
          </p>
        )}
        <form
          className="flex flex-wrap items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            onScanOne();
          }}
        >
          <label className="flex items-center gap-1.5 text-sm" htmlFor="career-board">
            Board
            <FieldHint text="All boards tries Greenhouse, Ashby, and Lever for this slug. Pick one if you already know the ATS." />
            <select
              id="career-board"
              value={scanBoard}
              onChange={(e) => setScanBoard(e.target.value)}
              className="rounded-md border border-input bg-background px-2 py-1"
            >
              <option value="all">All boards</option>
              <option value="greenhouse">Greenhouse</option>
              <option value="ashby">Ashby</option>
              <option value="lever">Lever</option>
            </select>
          </label>
          <label className="flex items-center gap-1.5 text-sm" htmlFor="career-slug">
            Company slug
            <FieldHint text="From the careers URL, e.g. anthropic from boards.greenhouse.io/anthropic." />
            <input
              id="career-slug"
              value={scanSlug}
              onChange={(e) => setScanSlug(e.target.value)}
              className="rounded-md border border-input bg-background px-2 py-1"
              placeholder="anthropic"
            />
          </label>
          <button
            type="submit"
            disabled={busy || !scanSlug.trim()}
            className="rounded-md border border-border px-3 py-1.5 text-sm disabled:opacity-50"
          >
            {busy ? "Scanning…" : "Scan this company"}
          </button>
          <button
            type="button"
            className="rounded-md border border-border px-3 py-1.5 text-sm"
            disabled={!scanSlug.trim() || scanBoard === "all"}
            onClick={onSavePortal}
          >
            Save board
          </button>
        </form>

        <div className="grid gap-3 lg:grid-cols-2">
          <div>
            <FieldLabel
              htmlFor="career-job-url"
              label="Or add one job URL"
              hint="Public posting URL. We fetch the JD, score it, and add a row. Tailor is a separate click."
            />
            <input
              id="career-job-url"
              value={jobUrl}
              onChange={(e) => setJobUrl(e.target.value)}
              placeholder="https://boards.greenhouse.io/…"
              className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
            />
          </div>
          <div>
            <FieldLabel
              htmlFor="career-jd"
              label="Or paste a job description"
              hint="Full job text. Treated as untrusted data. Use a URL or this paste — you do not need both."
            />
            <textarea
              id="career-jd"
              value={jdText}
              onChange={(e) => setJdText(e.target.value)}
              rows={4}
              className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              placeholder="Paste the JD."
            />
          </div>
        </div>
        <button
          type="button"
          disabled={busy || (!jobUrl.trim() && !jdText.trim())}
          onClick={onAddJob}
          className="rounded-md border border-border px-3 py-2 text-sm disabled:opacity-50"
        >
          {busy ? "Adding…" : "Add and score"}
        </button>
      </section>

      <section className="grid gap-4 lg:grid-cols-2">
        <div>
          <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
            <h3 className="text-sm font-medium">Your jobs</h3>
            {jobs.length > 0 && (
              <div className="flex flex-wrap items-center gap-2">
                <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
                  <input
                    type="checkbox"
                    checked={allChecked}
                    onChange={(e) => toggleAll(e.target.checked)}
                    aria-label="Select all jobs"
                  />
                  Select all
                </label>
                <button
                  type="button"
                  disabled={busy || selectedJobs.length === 0}
                  onClick={() => onScoreSelected(selectedJobs)}
                  className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
                >
                  {busy
                    ? "Scoring…"
                    : selectedJobs.length > 0
                      ? `Score selected (${selectedJobs.length})`
                      : "Score selected"}
                </button>
                <FieldHint text="Tick jobs, then score them against your profile. Up to 8 at a time. Score is advice — it does not apply for you." />
              </div>
            )}
          </div>
          {jobs.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              Scan all companies, or paste a job URL.
            </p>
          ) : (
            <ul className="divide-y divide-border rounded-md border border-border text-sm">
              {jobs.map((job) => {
                const href = postingHref(job.listing_url);
                return (
                  <li key={job.key} className="flex items-start gap-2 px-2 py-2">
                    <input
                      type="checkbox"
                      className="mt-1"
                      checked={!!checked[job.key]}
                      onChange={(e) => toggle(job.key, e.target.checked)}
                      aria-label={`Select ${job.role || job.company || "job"}`}
                    />
                    <div className="min-w-0 flex-1">
                      <button
                        type="button"
                        onClick={() => onSelect(job)}
                        className={`flex w-full flex-col items-start gap-0.5 rounded-md px-2 py-1 text-left hover:bg-muted/40 ${
                          selected?.key === job.key ? "bg-muted/60" : ""
                        }`}
                      >
                        <span className="font-medium">
                          {job.role || job.listing_url || "Job"}
                        </span>
                        <span className="text-xs text-muted-foreground">
                          {job.company || "Company"}
                          {job.score != null ? ` · ${job.score.toFixed(1)} / 5` : " · not scored"}
                          {job.status ? ` · ${job.status}` : ""}
                        </span>
                      </button>
                      <div className="mt-1 flex flex-wrap gap-2 px-2">
                        {href && (
                          <a
                            href={href}
                            target="_blank"
                            rel="noreferrer"
                            className="text-xs underline underline-offset-2"
                          >
                            Go to posting
                          </a>
                        )}
                        <button
                          type="button"
                          className="text-xs underline underline-offset-2"
                          onClick={() => onSeeJD(job)}
                        >
                          See JD
                        </button>
                      </div>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
        <div className="min-h-[12rem] rounded-md border border-border p-4">
          {selected ? (
            <CareerJobDetail
              job={selected}
              artifacts={artifacts}
              draftNote={draftNote}
              busy={busy}
              hasCV={hasCV}
              onScore={onScore}
              onTailor={onTailor}
              onCover={onCover}
              onEmail={onEmail}
              onApplied={onApplied}
              onSeeJD={() => onSeeJD(selected)}
            />
          ) : (
            <p className="text-sm text-muted-foreground">
              Tick jobs to score several at once, or open a row for the JD and posting.
            </p>
          )}
        </div>
      </section>

      {patterns && patterns.applications > 0 && (
        <p className="text-xs text-muted-foreground">
          {patterns.applications} scored
          {patterns.avg_score ? ` · average ${patterns.avg_score.toFixed(1)} / 5` : ""}
          {patterns.skill_gaps && patterns.skill_gaps.length > 0
            ? ` · gaps: ${patterns.skill_gaps.slice(0, 8).join(", ")}`
            : ""}
        </p>
      )}
      {jdPreview && <CareerJDDialog preview={jdPreview} onClose={onCloseJD} />}
    </div>
  );
}

function portalBoardSummary(portals: CareerPortal[]): string {
  const counts: Record<string, number> = {};
  for (const p of portals) {
    const board = p.board || "other";
    counts[board] = (counts[board] ?? 0) + 1;
  }
  const labels: Record<string, string> = {
    greenhouse: "Greenhouse",
    ashby: "Ashby",
    lever: "Lever",
  };
  return ["greenhouse", "ashby", "lever"]
    .filter((b) => counts[b])
    .map((b) => `${counts[b]} ${labels[b]}`)
    .join(", ");
}
