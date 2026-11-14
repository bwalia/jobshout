import Link from "next/link";
import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { toggleSavedJobAction } from "@/app/actions";
import { JobCard } from "@/components/JobCard";
import { BriefcaseIcon } from "@/components/icons";
import { Badge, EmptyState, ErrorNotice, buttonClass } from "@/components/ui";
import { listSavedJobs, type Job } from "@/lib/api";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Saved jobs",
  robots: { index: false, follow: false },
};

export default async function SavedJobsPage({
  searchParams,
}: {
  searchParams: { saved?: string; error?: string };
}) {
  const viewer = await currentViewer();
  if (!viewer) redirect("/login?callbackUrl=%2Fjobs%2Fsaved");

  let jobs: Job[] = [];
  let error = "";
  try {
    jobs = await listSavedJobs(viewer);
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load saved jobs";
  }

  return (
    <div className="mx-auto max-w-board px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div className="max-w-2xl">
          <h1 className="font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
            Saved jobs
          </h1>
          <p className="mt-4 text-base leading-relaxed text-mute">
            Roles you bookmarked, newest first. Closed roles stay here, marked, so you know what happened.
          </p>
        </div>
        <Link href="/jobs/alerts" className={buttonClass("secondary", "md")}>
          Job alerts
        </Link>
      </header>

      {searchParams.saved === "0" ? (
        <p role="status" className="mt-8 text-sm font-medium text-good">Removed from saved jobs.</p>
      ) : null}
      {searchParams.error || error ? (
        <div className="mt-8">
          <ErrorNotice title={searchParams.error || error} />
        </div>
      ) : null}

      {!error && jobs.length === 0 ? (
        <div className="mt-8">
          <EmptyState
            icon={<BriefcaseIcon className="h-6 w-6" />}
            title="No saved jobs yet"
            body="Use Save on any job to keep it here."
            action={
              <Link href="/jobs" className={buttonClass("primary", "md")}>
                Browse open roles
              </Link>
            }
          />
        </div>
      ) : null}

      {jobs.length > 0 ? (
        <ul className="mt-8 grid gap-4 xl:grid-cols-2">
          {jobs.map((job) => (
            <li key={job.id} className="space-y-2">
              {job.status !== "published" ? <Badge tone="neutral">No longer taking applications</Badge> : null}
              <JobCard job={job} />
              <form action={toggleSavedJobAction}>
                <input type="hidden" name="job_id" value={job.id} />
                <input type="hidden" name="op" value="unsave" />
                <input type="hidden" name="back" value="/jobs/saved" />
                <button type="submit" className={buttonClass("ghost", "sm")}>
                  Remove
                </button>
              </form>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}
