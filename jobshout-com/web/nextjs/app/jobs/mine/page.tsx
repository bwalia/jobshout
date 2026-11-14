import Link from "next/link";
import type { Metadata } from "next";
import { manageJobAction } from "@/app/actions";
import { ConfirmSubmit } from "@/components/ConfirmSubmit";
import { Badge, EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import { BriefcaseIcon } from "@/components/icons";
import { countApplications, listMyJobs, type Job, type JobStatus } from "@/lib/api";
import { employmentLabel, relativeTime, shortLocation } from "@/lib/format";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "My posted jobs",
  description: "Every job you have posted on JobShout.com — drafts, live and closed.",
};

const STATUS: Record<JobStatus, { label: string; tone: "neutral" | "good" | "warn" }> = {
  draft: { label: "Draft — not on the board", tone: "warn" },
  published: { label: "Live on the board", tone: "good" },
  closed: { label: "Closed", tone: "neutral" },
  archived: { label: "Archived", tone: "neutral" },
};

const DONE: Record<string, string> = {
  publish: "Published. The job is on the public board now.",
  close: "Closed. The job is off the board and no longer takes applications.",
  delete: "Draft deleted.",
};

export default async function MyJobsPage({
  searchParams,
}: {
  searchParams: { done?: string; error?: string };
}) {
  const viewer = await currentViewer();
  const done = searchParams.done ? DONE[searchParams.done] : undefined;
  const failed = searchParams.error;

  let jobs: Job[] = [];
  let applicantCounts: Record<string, number> = {};
  let error = "";
  if (viewer) {
    try {
      jobs = await listMyJobs(viewer);
      const counts = await Promise.all(jobs.map((j) => countApplications(j.id, viewer)));
      applicantCounts = Object.fromEntries(jobs.map((j, i) => [j.id, counts[i]]));
    } catch (e) {
      error = e instanceof Error ? e.message : "Could not load your jobs";
    }
  }

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div className="max-w-2xl">
          <h1 className="font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
            My posted jobs
          </h1>
          <p className="mt-4 text-base leading-relaxed text-mute">
            Everything you have posted, newest first. Drafts only appear here until you publish
            them.
          </p>
        </div>
        <Link href="/post-job" className={buttonClass("primary", "md")}>
          Post a job
        </Link>
      </header>

      {done ? (
        <p
          role="status"
          className="mt-8 rounded-xl border border-good/30 bg-good/10 px-4 py-3 text-sm font-medium text-good"
        >
          {done}
        </p>
      ) : null}

      {failed ? (
        <div className="mt-8">
          <ErrorNotice title={failed} />
        </div>
      ) : null}

      {!viewer ? (
        <div className="mt-8">
          <ErrorNotice title="Sign in to see the jobs you have posted." />
        </div>
      ) : null}

      {error ? (
        <div className="mt-8">
          <ErrorNotice title={error} />
        </div>
      ) : null}

      {viewer && !error && jobs.length === 0 ? (
        <div className="mt-8">
          <EmptyState
            icon={<BriefcaseIcon className="h-6 w-6" />}
            title="You have not posted a job yet"
            body="Publishing takes about two minutes, and you can save a draft first."
            action={
              <Link href="/post-job" className={buttonClass("primary", "md")}>
                Post a job
              </Link>
            }
          />
        </div>
      ) : null}

      {jobs.length > 0 ? (
        <ul className="mt-8 space-y-3">
          {jobs.map((job) => {
            const status = STATUS[job.status] ?? STATUS.draft;
            return (
              <li
                key={job.id}
                className="surface-card flex flex-wrap items-center justify-between gap-4 p-5"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <Link
                      href={`/jobs/${job.id}`}
                      className="font-display text-lg font-semibold text-ink hover:text-shout"
                    >
                      {job.title}
                    </Link>
                    <Badge tone={status.tone}>{status.label}</Badge>
                  </div>
                  <p className="mt-1 text-sm text-mute">
                    {employmentLabel(job.employment_type)} · {shortLocation(job.location)} ·
                    updated {relativeTime(job.updated_at)}
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Link href={`/jobs/${job.id}`} className={buttonClass("ghost", "sm")}>
                    View
                  </Link>
                  {job.status !== "draft" ? (
                    <Link
                      href={`/jobs/${job.id}/applicants`}
                      className={buttonClass("secondary", "sm")}
                    >
                      Applicants ({applicantCounts[job.id] ?? 0})
                    </Link>
                  ) : null}
                  {job.status === "draft" || job.status === "published" ? (
                    <Link href={`/jobs/${job.id}/edit`} className={buttonClass("secondary", "sm")}>
                      Edit
                    </Link>
                  ) : null}
                  {job.status === "draft" ? (
                    <form action={manageJobAction}>
                      <input type="hidden" name="job_id" value={job.id} />
                      <input type="hidden" name="op" value="publish" />
                      <button type="submit" className={buttonClass("primary", "sm")}>
                        Publish
                      </button>
                    </form>
                  ) : null}
                  {job.status === "draft" ? (
                    <form action={manageJobAction}>
                      <input type="hidden" name="job_id" value={job.id} />
                      <input type="hidden" name="op" value="delete" />
                      <ConfirmSubmit
                        confirm={`Delete the draft "${job.title}"? This cannot be undone.`}
                        className={buttonClass("ghost", "sm")}
                      >
                        Delete
                      </ConfirmSubmit>
                    </form>
                  ) : null}
                  {job.status === "published" ? (
                    <form action={manageJobAction}>
                      <input type="hidden" name="job_id" value={job.id} />
                      <input type="hidden" name="op" value="close" />
                      <ConfirmSubmit
                        confirm={`Close "${job.title}"? It comes off the board and stops taking applications.`}
                        className={buttonClass("secondary", "sm")}
                      >
                        Close
                      </ConfirmSubmit>
                    </form>
                  ) : null}
                </div>
              </li>
            );
          })}
        </ul>
      ) : null}
    </div>
  );
}
