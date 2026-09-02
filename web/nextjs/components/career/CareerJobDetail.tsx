"use client";

import { FieldHint } from "@/components/ui/field-hint";
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
}) {
  const ev = job.evaluation;
  const score = ev?.score?.overall ?? job.score;
  const hardStop = !!ev?.hard_stop;
  const below = score != null && score < 4;
  const posting = postingHref(job.listing_url);

  return (
    <div className="space-y-3 text-sm">
      <div>
        <p className="font-medium">
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
        <FieldHint text="Updates your saved CV for this job and downloads a PDF. Score is advice — it does not lock this button. A human submits." />
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
        {posting && (
          <a
            href={posting}
            target="_blank"
            rel="noreferrer"
            className="rounded-md border border-border px-3 py-1.5 text-sm"
          >
            Go to posting
          </a>
        )}
        <button
          type="button"
          className="rounded-md border border-border px-3 py-1.5 text-sm disabled:opacity-50"
          disabled={busy}
          onClick={onSeeJD}
        >
          See JD
        </button>
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
      {!hasCV && (
        <p className="text-xs text-muted-foreground">Upload a PDF CV on Profile before tailoring.</p>
      )}
      {draftNote && <p className="text-xs text-muted-foreground">{draftNote}</p>}

      {ev?.report_markdown && (
        <article className="max-h-72 overflow-auto whitespace-pre-wrap rounded-md border border-border p-3 text-xs leading-relaxed">
          {ev.report_markdown}
        </article>
      )}

      {artifacts.length > 0 && (
        <ul className="space-y-2 border-t border-border pt-2">
          {artifacts.map((a) => (
            <li key={a.id}>
              <p className="text-xs font-medium">
                {a.kind} — {a.title || "draft"}
              </p>
              <pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap rounded bg-muted/40 p-2 text-xs">
                {a.body_markdown}
              </pre>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
