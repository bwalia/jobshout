"use client";

import { FieldHint } from "@/components/ui/field-hint";
import { splitCareerTailorNote } from "@/lib/career/tailor-note";
import type { CareerArtifact } from "@/types/career";
import { postingHref, type CareerJob } from "./jobs";

export function CareerJobDetail({
  job,
  artifacts,
  draftNote,
  busy,
  hasCV,
  onScore,
  onTailor,
  onCover,
  onEmail,
  onApplied,
  onSeeJD,
  onBack,
}: {
  job: CareerJob;
  artifacts: CareerArtifact[];
  draftNote: string;
  busy: boolean;
  hasCV: boolean;
  onScore: () => void;
  onTailor: () => void;
  onCover: () => void;
  onEmail: () => void;
  onApplied: () => void;
  onSeeJD: () => void;
  onBack?: () => void;
}) {
  const ev = job.evaluation;
  const score = ev?.score?.overall ?? job.score;
  const hardStop = !!ev?.hard_stop;
  const below = score != null && score < 4;
  const posting = postingHref(job.listing_url);
  const cvArt = artifacts.find((a) => a.kind === "cv");
  const tailorNote = draftNote || (cvArt ? splitCareerTailorNote(cvArt.body_markdown).note : "");
  const drafts = artifacts.filter((a) => a.kind !== "cv");

  return (
    <div className="space-y-4 text-sm">
      {onBack && (
        <button
          type="button"
          className="text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
          onClick={onBack}
        >
          ← Back to jobs
        </button>
      )}
      <div>
        <p className="text-base font-medium">
          {job.role || "Role"} — {job.company || "Company"}
        </p>
        <p className="text-muted-foreground">
          {score != null ? `${score.toFixed(1)} / 5` : "Not scored yet"}
          {job.status ? ` · ${job.status}` : ""}
        </p>
      </div>

      {hardStop && (
        <p className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-destructive">
          {ev?.hard_stop_reason || "Hard stop — this posting may not be workable (often no visa sponsorship)."} You can still tailor a CV.
        </p>
      )}

      {below && !hardStop && (
        <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-muted-foreground">
          Below 4.0 — we would not apply. You still can, and you can still get a tailored CV.
        </p>
      )}

      <div className="flex flex-wrap items-center gap-2">
        {!ev && (
          <button
            type="button"
            className="rounded-md border border-border px-3 py-1.5 text-sm disabled:opacity-50"
            disabled={busy}
            onClick={onScore}
          >
            {busy ? "Scoring…" : "Score this job"}
          </button>
        )}
        <button
          type="button"
          className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground disabled:opacity-50"
          disabled={busy || !hasCV}
          onClick={onTailor}
        >
          Get tailored CV
        </button>
        <FieldHint text="The PDF keeps your layout. What we changed is shown on this page, not on the file. A human submits." />
        {ev && (
          <>
            <button
              type="button"
              className="rounded-md border border-border px-3 py-1.5 text-sm disabled:opacity-50"
              disabled={busy}
              onClick={onCover}
            >
              Cover letter
            </button>
            <button
              type="button"
              className="rounded-md border border-border px-3 py-1.5 text-sm disabled:opacity-50"
              disabled={busy}
              onClick={onEmail}
            >
              Email draft
            </button>
          </>
        )}
        {job.application && job.application.status !== "applied" && (
          <button
            type="button"
            className="rounded-md border border-border px-3 py-1.5 text-sm disabled:opacity-50"
            disabled={busy}
            onClick={onApplied}
          >
            I applied
          </button>
        )}
      </div>
      <p className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
        {posting && (
          <a href={posting} target="_blank" rel="noreferrer" className="underline underline-offset-2">
            Go to posting
          </a>
        )}
        <button type="button" className="underline underline-offset-2 disabled:opacity-50" disabled={busy} onClick={onSeeJD}>
          See JD
        </button>
      </p>
      {!hasCV && (
        <p className="text-xs text-muted-foreground">Upload a PDF CV on Profile before tailoring.</p>
      )}

      {tailorNote && (
        <div className="rounded-md border border-border bg-muted/40 px-3 py-2">
          <p className="text-xs font-medium">What changed on this CV</p>
          <p className="mt-1 text-sm leading-relaxed">{tailorNote}</p>
        </div>
      )}

      {ev?.report_markdown && (
        <details className="rounded-md border border-border">
          <summary className="cursor-pointer px-3 py-2 text-xs font-medium">Score report</summary>
          <article className="max-h-64 overflow-auto whitespace-pre-wrap border-t border-border px-3 py-2 text-xs leading-relaxed">
            {ev.report_markdown}
          </article>
        </details>
      )}

      {drafts.length > 0 && (
        <ul className="space-y-2">
          {drafts.map((a) => (
            <li key={a.id} className="rounded-md border border-border">
              <p className="px-3 py-2 text-xs font-medium">
                {a.kind === "cover" ? "Cover letter" : a.kind === "email" ? "Email draft" : a.title || a.kind}
              </p>
              <pre className="max-h-40 overflow-auto whitespace-pre-wrap border-t border-border bg-muted/30 p-3 text-xs">
                {a.body_markdown}
              </pre>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
