import Link from "next/link";
import type { Metadata } from "next";
import { notFound, redirect } from "next/navigation";
import { setApplicationStatusAction } from "@/app/actions";
import { Badge, EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import { InboxIcon } from "@/components/icons";
import {
  RECRUITER_STATUSES,
  getJob,
  listApplicationsForJob,
  type JobApplication,
} from "@/lib/api";
import { APPLICATION_LABELS, relativeTime } from "@/lib/format";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Applicants",
  robots: { index: false },
};

const PAGE_SIZE = 25;

export default async function ApplicantsPage({
  params,
  searchParams,
}: {
  params: { id: string };
  searchParams: { done?: string; error?: string; page?: string };
}) {
  const viewer = await currentViewer();
  if (!viewer) redirect(`/login?callbackUrl=/jobs/${params.id}/applicants`);

  const job = await getJob(params.id, viewer);
  if (!job) notFound();

  const page = Math.max(1, Number(searchParams.page) || 1);
  let applicants: JobApplication[] = [];
  let total = 0;
  let error = "";
  try {
    const res = await listApplicationsForJob(job.id, viewer, {
      limit: PAGE_SIZE,
      offset: (page - 1) * PAGE_SIZE,
    });
    applicants = res.data;
    total = res.total;
  } catch (e) {
    // The API answers 403 to anyone who did not post this job.
    error = e instanceof Error ? e.message : "Could not load applicants";
  }
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const done = searchParams.done as keyof typeof APPLICATION_LABELS | undefined;

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header className="max-w-3xl">
        <Link href="/jobs/mine" className="text-sm font-medium text-mute hover:text-ink">
          ← My posted jobs
        </Link>
        <h1 className="mt-4 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
          Applicants
        </h1>
        <p className="mt-3 text-base text-mute">
          <Link href={`/jobs/${job.id}`} className="font-medium text-ink hover:text-shout">
            {job.title}
          </Link>{" "}
          · {total} {total === 1 ? "application" : "applications"}
        </p>
      </header>

      {done && APPLICATION_LABELS[done] ? (
        <p
          role="status"
          className="mt-8 rounded-xl border border-good/30 bg-good/10 px-4 py-3 text-sm font-medium text-good"
        >
          Moved to “{APPLICATION_LABELS[done]}”. The candidate sees this on their applications page.
        </p>
      ) : null}
      {searchParams.error ? (
        <div className="mt-8">
          <ErrorNotice title={searchParams.error} />
        </div>
      ) : null}
      {error ? (
        <div className="mt-8">
          <ErrorNotice title={error} />
        </div>
      ) : null}

      {!error && applicants.length === 0 ? (
        <div className="mt-8">
          <EmptyState
            icon={<InboxIcon className="h-6 w-6" />}
            title="No applications yet"
            body={
              job.status === "published"
                ? "Applications appear here as soon as candidates apply."
                : "Only published jobs take applications."
            }
          />
        </div>
      ) : null}

      {applicants.length > 0 ? (
        <ul className="mt-8 space-y-4">
          {applicants.map((a) => (
            <li key={a.id} id={a.id} className="surface-card p-5 sm:p-6">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="font-display text-lg font-semibold text-ink">{a.full_name}</p>
                  <p className="mt-0.5 break-all text-sm text-mute">
                    <a href={`mailto:${a.email}`} className="hover:text-ink">
                      {a.email}
                    </a>
                    {a.phone ? ` · ${a.phone}` : ""} · applied {relativeTime(a.created_at)}
                  </p>
                </div>
                <Badge tone={a.status === "rejected" || a.status === "withdrawn" ? "neutral" : "brand"}>
                  {APPLICATION_LABELS[a.status] ?? a.status}
                </Badge>
              </div>

              {a.cover_letter ? (
                <details className="mt-4">
                  <summary className="cursor-pointer text-sm font-medium text-ink">Cover letter</summary>
                  <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-mute">
                    {a.cover_letter}
                  </p>
                </details>
              ) : null}
              {a.cv_text ? (
                <details className="mt-3">
                  <summary className="cursor-pointer text-sm font-medium text-ink">CV</summary>
                  <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-mute">{a.cv_text}</p>
                </details>
              ) : null}
              {/^https?:\/\//i.test(a.cv_url) ? (
                <p className="mt-3 text-sm">
                  <a
                    href={a.cv_url}
                    target="_blank"
                    rel="noopener noreferrer nofollow"
                    className="font-medium text-ink underline underline-offset-4 hover:text-shout"
                  >
                    Open CV link
                  </a>
                </p>
              ) : null}

              {a.status === "withdrawn" ? (
                <p className="mt-4 text-sm text-mute">The candidate withdrew this application.</p>
              ) : (
                <form action={setApplicationStatusAction} className="mt-4 flex flex-wrap items-center gap-2">
                  <input type="hidden" name="job_id" value={job.id} />
                  <input type="hidden" name="application_id" value={a.id} />
                  <label htmlFor={`status-${a.id}`} className="text-sm text-mute">
                    Stage
                  </label>
                  <select
                    id={`status-${a.id}`}
                    name="status"
                    defaultValue={a.status}
                    className="rounded-lg border border-line bg-raised px-3 py-1.5 text-sm text-ink"
                  >
                    {RECRUITER_STATUSES.map((s) => (
                      <option key={s} value={s}>
                        {APPLICATION_LABELS[s]}
                      </option>
                    ))}
                  </select>
                  <button type="submit" className={buttonClass("secondary", "sm")}>
                    Update
                  </button>
                </form>
              )}
            </li>
          ))}
        </ul>
      ) : null}

      {pages > 1 ? (
        <nav className="mt-8 flex items-center gap-3 text-sm" aria-label="Applicant pages">
          {page > 1 ? (
            <Link href={`?page=${page - 1}`} className={buttonClass("ghost", "sm")}>
              Previous
            </Link>
          ) : null}
          <span className="text-mute">
            Page {page} of {pages}
          </span>
          {page < pages ? (
            <Link href={`?page=${page + 1}`} className={buttonClass("ghost", "sm")}>
              Next
            </Link>
          ) : null}
        </nav>
      ) : null}
    </div>
  );
}
