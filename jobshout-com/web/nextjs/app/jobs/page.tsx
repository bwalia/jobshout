import Link from "next/link";
import { formatCompensation, formatLocation, listJobs, type Job } from "@/lib/api";

export const dynamic = "force-dynamic";

export default async function JobsPage() {
  let jobs: Job[] = [];
  let error = "";
  try {
    jobs = await listJobs();
  } catch (e) {
    error = e instanceof Error ? e.message : "Could not load jobs";
  }

  return (
    <div className="mx-auto max-w-board px-6 pb-20 pt-12">
      <div className="flex flex-wrap items-end justify-between gap-6 border-b border-line pb-8">
        <div className="max-w-2xl">
          <h1 className="font-display text-4xl tracking-tight text-ink md:text-5xl">Open roles</h1>
          <p className="mt-3 text-mute">
            Live listings from the marketplace API. Build a profile to see ranked matches.
          </p>
        </div>
        <Link
          href="/profile"
          className="border border-ink/20 bg-white/60 px-4 py-2 text-sm font-semibold text-ink transition hover:border-signal hover:text-signal"
        >
          Match my profile
        </Link>
      </div>

      {error && (
        <p className="mt-8 border border-shout/30 bg-shout/5 px-4 py-3 text-sm text-ink">
          {error}. Start the API on :8088, then refresh.
        </p>
      )}

      <ul className="mt-2">
        {jobs.map((job) => (
          <li key={job.id}>
            <Link href={`/jobs/${job.id}`} className="job-row pl-4">
              <div className="flex flex-wrap items-baseline justify-between gap-x-6 gap-y-2">
                <h2 className="font-display text-2xl tracking-tight text-ink md:text-[1.65rem]">
                  {job.title}
                </h2>
                <p className="text-sm font-medium text-ink">
                  {formatCompensation(job.compensation)}
                </p>
              </div>
              <p className="mt-2 text-sm text-mute">
                {formatLocation(job.location)}
                <span className="mx-2 text-line">/</span>
                {job.employment_type.replaceAll("_", " ")}
              </p>
              <p className="mt-3 max-w-3xl text-[0.95rem] leading-relaxed text-mute line-clamp-2">
                {job.summary || job.description}
              </p>
              {job.requirements.length > 0 && (
                <p className="mt-3 text-sm text-ink/70">
                  {job.requirements.slice(0, 5).join(" · ")}
                </p>
              )}
            </Link>
          </li>
        ))}
        {!error && jobs.length === 0 && (
          <li className="py-16 text-center text-mute">
            No published jobs yet. Seed the API, then refresh this page.
          </li>
        )}
      </ul>
    </div>
  );
}
